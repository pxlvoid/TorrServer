package torrstor

// homelab: what the disk cache holds and managing it — for web/api/homelab.go (see homelab.go).

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

var (
	ErrHomelabNotReady = errors.New("disk cache is off: enable UseDisk and set TorrentsSavePath")
	ErrHomelabBadHash  = errors.New("bad hash")
	ErrHomelabOpen     = errors.New("torrent is open by upstream cache, close it first")
	ErrHomelabNotFound = errors.New("no cache for this torrent")
)

// HomelabItem — cache of one torrent on disk.
type HomelabItem struct {
	Hash        string `json:"hash"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`        // bytes on disk
	TotalLength int64  `json:"totalLength"` // whole torrent, 0 — unknown
	Pieces      int    `json:"pieces"`      // pieces on disk
	PieceCount  int    `json:"pieceCount"`  // pieces in the torrent, 0 — unknown
	LastAccess  int64  `json:"lastAccess"`  // unix time
	Pinned      bool   `json:"pinned"`      // never evicted by the janitor
	Open        bool   `json:"open"`        // the torrent is loaded
	Playing     bool   `json:"playing"`     // someone reads it right now

	// title and poster remembered from the TorrServer list (HomelabSetInfo), for a torrent no longer in it
	SavedTitle  string `json:"-"`
	SavedPoster string `json:"-"`
}

// HomelabUsage — the disk cache as a whole.
type HomelabUsage struct {
	Enabled   bool   `json:"enabled"` // persistent mode is on
	Ready     bool   `json:"ready"`   // UseDisk and TorrentsSavePath are set
	Path      string `json:"path"`
	Used      int64  `json:"used"`
	Limit     int64  `json:"limit"` // 0 — no limit
	DiskFree  int64  `json:"diskFree"`
	DiskTotal int64  `json:"diskTotal"`
}

func hlReady() bool {
	b := settings.BTsets
	return b != nil && b.UseDisk && b.TorrentsSavePath != ""
}

func hlByHashGet(hash string) *hlCache {
	hlMu.Lock()
	defer hlMu.Unlock()
	return hlByHash[hash]
}

// hlUpstreamOpen — the torrent is loaded but its cache is not persistent (opened before the mode was on).
func hlUpstreamOpen(hash string) *Cache {
	hlMu.Lock()
	s := hlStorage
	hlMu.Unlock()
	if s == nil {
		return nil
	}
	return s.GetCache(metainfo.NewHashFromHex(hash))
}

// HomelabList — torrents in the cache dir, most recently watched first.
func HomelabList() ([]HomelabItem, HomelabUsage) {
	u := HomelabUsage{Enabled: hlEnabled(), Ready: hlReady(), Path: hlRoot()}
	if sets := settings.GetHomelabSets(); sets.LimitGB > 0 {
		u.Limit = sets.LimitGB << 30
	}
	items := make([]HomelabItem, 0)
	if !u.Ready {
		return items, u
	}
	u.DiskFree, u.DiskTotal, _ = hlDiskStat(u.Path)

	open := map[string]*hlCache{}
	for _, h := range hlOpenSnapshot() {
		open[h.meta.Hash] = h
	}

	entries, _ := os.ReadDir(u.Path)
	for _, e := range entries {
		if !e.IsDir() || !hlIsHash(e.Name()) {
			continue
		}
		it := HomelabItem{Hash: e.Name()}
		if h := open[it.Hash]; h != nil {
			for _, p := range h.c.getPieces() {
				if p.Size > 0 {
					it.Size += p.Size
					it.Pieces++
				}
			}
			h.mu.Lock()
			it.Name, it.TotalLength, it.PieceCount = h.meta.Name, h.meta.TotalLength, h.meta.PieceCount
			it.LastAccess, it.Pinned = h.meta.LastAccess, h.meta.Pinned
			it.SavedTitle, it.SavedPoster = h.meta.Title, h.meta.Poster
			h.mu.Unlock()
			it.Open = true
			it.Playing = hlPlayers(h.c) > 0
		} else {
			dir := filepath.Join(u.Path, it.Hash)
			for _, f := range hlPieceFiles(dir) {
				it.Size += f.size
				it.Pieces++
				if f.mtime > it.LastAccess {
					it.LastAccess = f.mtime
				}
			}
			if meta, _ := hlReadMeta(dir); meta != nil {
				it.Name, it.TotalLength, it.PieceCount, it.Pinned = meta.Name, meta.TotalLength, meta.PieceCount, meta.Pinned
				it.SavedTitle, it.SavedPoster = meta.Title, meta.Poster
				if meta.LastAccess > it.LastAccess {
					it.LastAccess = meta.LastAccess
				}
			}
			if c := hlUpstreamOpen(it.Hash); c != nil {
				it.Open = true
				it.Playing = hlPlayers(c) > 0
			}
		}
		u.Used += it.Size
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LastAccess > items[j].LastAccess })
	return items, u
}

// HomelabRemove frees the cache of a torrent. Of a playing torrent only pieces outside the reader window go.
func HomelabRemove(hash string) (freed int64, err error) {
	if !hlReady() {
		return 0, ErrHomelabNotReady
	}
	if !hlIsHash(hash) {
		return 0, ErrHomelabBadHash
	}
	if h := hlByHashGet(hash); h != nil {
		for _, p := range h.c.getRemPieces() {
			freed += p.Size
			h.release(p)
		}
		return freed, nil
	}
	if hlUpstreamOpen(hash) != nil {
		return 0, ErrHomelabOpen
	}

	hlMu.Lock()
	defer hlMu.Unlock()
	if hlIsOpenLocked(hash) {
		return 0, ErrHomelabOpen
	}
	dir := filepath.Join(hlRoot(), hash)
	if _, err := os.Stat(dir); err != nil {
		return 0, ErrHomelabNotFound
	}
	for _, f := range hlPieceFiles(dir) {
		freed += f.size
	}
	return freed, os.RemoveAll(dir)
}

// HomelabSetPinned — pinned torrents are never evicted by the janitor.
func HomelabSetPinned(hash string, pinned bool) error {
	if !hlReady() {
		return ErrHomelabNotReady
	}
	if !hlIsHash(hash) {
		return ErrHomelabBadHash
	}
	if h := hlByHashGet(hash); h != nil {
		h.mu.Lock()
		h.meta.Pinned = pinned
		h.dirty = true
		h.mu.Unlock()
		hlMu.Lock()
		h.flushLocked()
		hlMu.Unlock()
		return nil
	}

	hlMu.Lock()
	defer hlMu.Unlock()
	dir := filepath.Join(hlRoot(), hash)
	if _, err := os.Stat(dir); err != nil {
		return ErrHomelabNotFound
	}
	meta, _ := hlReadMeta(dir)
	if meta == nil {
		meta = &hlMeta{Hash: hash}
	}
	meta.Pinned = pinned
	return hlWriteMeta(dir, meta)
}

// HomelabSetInfo remembers the title and poster of a torrent in its cache meta (only if they changed).
func HomelabSetInfo(hash, title, poster string) {
	if !hlReady() || !hlIsHash(hash) {
		return
	}
	if h := hlByHashGet(hash); h != nil {
		h.mu.Lock()
		if h.meta.Title != title || h.meta.Poster != poster {
			h.meta.Title, h.meta.Poster = title, poster
			h.dirty = true // written by the next flush (janitor, close)
		}
		h.mu.Unlock()
		return
	}
	hlMu.Lock()
	defer hlMu.Unlock()
	if hlIsOpenLocked(hash) {
		return
	}
	dir := filepath.Join(hlRoot(), hash)
	meta, _ := hlReadMeta(dir)
	if meta == nil || meta.Title == title && meta.Poster == poster {
		return
	}
	meta.Title, meta.Poster = title, poster
	_ = hlWriteMeta(dir, meta)
}

// HomelabClear frees everything that is not pinned (a playing torrent keeps its reader window).
func HomelabClear() (freed int64) {
	items, _ := HomelabList()
	for _, it := range items {
		if it.Pinned {
			continue
		}
		if n, err := HomelabRemove(it.Hash); err == nil {
			freed += n
		}
	}
	return freed
}
