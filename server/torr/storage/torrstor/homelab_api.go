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
	ErrHomelabNotReady = errors.New("no cache dir: set TorrentsSavePath in the settings")
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
	Open        bool   `json:"open"`        // the torrent is loaded
	Playing     bool   `json:"playing"`     // someone reads it right now

	// title and poster remembered from the TorrServer list (HomelabSetInfo), for a torrent no longer in it
	SavedTitle  string `json:"-"`
	SavedPoster string `json:"-"`
}

// HomelabUsage — the disk cache as a whole.
type HomelabUsage struct {
	Enabled bool `json:"enabled"` // persistent mode is on and working
	Ready   bool `json:"ready"`   // there is a cache dir to look at
	// the upstream "use disk" switch. Off with Ready on — the cache is frozen: nothing is written to it
	// and the janitor does not touch it, but what is left is listed and can be removed from the dialog.
	UseDisk   bool   `json:"useDisk"`
	Path      string `json:"path"`
	Used      int64  `json:"used"`
	Limit     int64  `json:"limit"` // 0 — no limit
	DiskFree  int64  `json:"diskFree"`
	DiskTotal int64  `json:"diskTotal"`
}

// hlReady — there is a cache dir to look at. UseDisk may be off: turning it off used to hide the whole
// homelab UI, and whatever was already cached stayed on the disk with no way to get rid of it.
func hlReady() bool {
	b := settings.BTsets
	return b != nil && b.TorrentsSavePath != "" && b.TorrentsSavePath != "/"
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
	u.UseDisk = settings.BTsets != nil && settings.BTsets.UseDisk
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
			it.LastAccess = h.meta.LastAccess
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
				it.Name, it.TotalLength, it.PieceCount = meta.Name, meta.TotalLength, meta.PieceCount
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
	defer hlInvalidateBudget() // the freed bytes are runway for background fill (homelab_budget.go)

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

	freed, err = func() (int64, error) {
		hlMu.Lock()
		defer hlMu.Unlock()
		if hlIsOpenLocked(hash) {
			return 0, ErrHomelabOpen
		}
		dir := filepath.Join(hlRoot(), hash)
		if _, err := os.Stat(dir); err != nil {
			return 0, ErrHomelabNotFound
		}
		var n int64
		for _, f := range hlPieceFiles(dir) {
			n += f.size
		}
		return n, os.RemoveAll(dir)
	}()
	return freed, err
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

// HomelabClear frees the whole cache (a playing torrent keeps its reader window).
func HomelabClear() (freed int64) {
	items, _ := HomelabList()
	for _, it := range items {
		if n, err := HomelabRemove(it.Hash); err == nil {
			freed += n
		}
	}
	return freed
}
