package torrstor

// homelab: persistent disk cache (pxlvoid/TorrServer fork, see HOMELAB.md).
//
// Upstream evicts pieces of a torrent as soon as it exceeds CacheSize. With a season pack in one
// torrent the episodes watched earlier are gone by the time you come back to them. In persistent
// mode (settings.HomelabSets.PersistentCache, requires UseDisk):
//   - per-torrent eviction is off (hook in Cache.cleanPieces); CacheSize only sizes the reader window;
//   - a global janitor (homelab_janitor.go) keeps the cache directory within LimitGB, evicting the
//     least recently accessed pieces across all torrents, and drops pieces not accessed for KeepDays.
//     Pieces inside active reader windows and pinned torrents are never evicted;
//   - pieces that passed the hash check are remembered in <hash>/.homelab.json and trusted after a
//     restart. A piece file without that mark is re-checked by anacrolix (Completion.Ok=false)
//     instead of being trusted by its size: chunks arrive out of order, so a full-size file can have holes.
//
// Hooks into upstream code are marked "// homelab" (list in HOMELAB.md); the rest is in homelab*.go.

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"server/log"
	"server/settings"
	"server/torr/storage/state"
)

const (
	hlMetaFile = ".homelab.json"
	// how often a read refreshes the piece file mtime — the access time the janitor sees after a restart
	hlTouchInterval = int64(time.Hour / time.Second)
)

// hlMeta — <hash>/.homelab.json
type hlMeta struct {
	Hash        string `json:"hash"`
	Name        string `json:"name"`
	TotalLength int64  `json:"totalLength"`
	PieceLength int64  `json:"pieceLength"`
	PieceCount  int    `json:"pieceCount"`
	LastAccess  int64  `json:"lastAccess"`
	Pinned      bool   `json:"pinned,omitempty"`
	// title and poster from the TorrServer list: the cache card keeps them after the torrent is removed from it
	Title  string `json:"title,omitempty"`
	Poster string `json:"poster,omitempty"`
	// bitmap of pieces that passed the hash check, base64
	Verified string `json:"verified,omitempty"`
}

// hlCache — persistent state of an open Cache.
type hlCache struct {
	c     *Cache
	dir   string
	total int64

	mu         sync.Mutex
	meta       hlMeta
	verified   []bool
	unverified []bool  // on disk, but not known to be valid: anacrolix re-checks the hash
	touched    []int64 // last mtime refresh per piece
	dirty      bool

	pieces []*hlPiece
}

var (
	hlCaches  sync.Map // *Cache → *hlCache (lock-free lookup in Cache.Piece, the hot path)
	hlStorage *Storage // storage of the running server: open torrents by hash

	// hlMu guards hlByHash and serializes removing files of closed torrents with opening them:
	// hlOnInit registers the cache under it, the janitor re-checks "not open" under it.
	hlMu     sync.Mutex
	hlByHash = map[string]*hlCache{}
)

func hlEnabled() bool {
	return settings.HomelabPersistentCache()
}

func hlRoot() string {
	if b := settings.BTsets; b != nil {
		return b.TorrentsSavePath
	}
	return ""
}

func hlGet(c *Cache) *hlCache {
	if v, ok := hlCaches.Load(c); ok {
		return v.(*hlCache)
	}
	return nil
}

// hlPieceLen — length of piece i (the last one is usually shorter).
func hlPieceLen(total, pieceLength int64, count, i int) int64 {
	if i == count-1 {
		return total - pieceLength*int64(count-1)
	}
	return pieceLength
}

// hlOnInit — hook at the end of Cache.Init: takes over the cache in persistent mode.
func hlOnInit(c *Cache, info *metainfo.Info) {
	hlMu.Lock()
	hlStorage = c.storage
	hlMu.Unlock()

	if !hlEnabled() || c.pieceCount == 0 || c.pieces[0].dPiece == nil {
		return
	}
	HomelabStartJanitor()

	hash := c.hash.HexString()
	h := &hlCache{
		c:          c,
		dir:        filepath.Join(hlRoot(), hash),
		total:      info.TotalLength(),
		verified:   make([]bool, c.pieceCount),
		unverified: make([]bool, c.pieceCount),
		touched:    make([]int64, c.pieceCount),
		pieces:     make([]*hlPiece, c.pieceCount),
	}
	for i := range h.pieces {
		h.pieces[i] = &hlPiece{Piece: c.pieces[i], h: h}
	}

	hlMu.Lock()
	defer hlMu.Unlock()

	old, _ := hlReadMeta(h.dir)
	var verified []bool
	if old != nil && old.PieceLength == c.pieceLength && old.PieceCount == c.pieceCount {
		verified = hlDecodeBits(old.Verified, c.pieceCount)
	} else {
		verified = make([]bool, c.pieceCount)
	}

	now := time.Now().Unix()
	for i := 0; i < c.pieceCount; i++ {
		p := c.pieces[i]
		want := hlPieceLen(h.total, c.pieceLength, c.pieceCount, i)
		st, err := os.Stat(p.dPiece.name)
		if err != nil {
			p.Size, p.Complete = 0, false
			continue
		}
		size := st.Size()
		if size > want {
			size = want
		}
		p.Size = size
		p.Accessed = st.ModTime().Unix()
		h.touched[i] = p.Accessed
		switch {
		case size == want && verified[i]:
			p.Complete = true
			h.verified[i] = true
		case size == want:
			p.Complete = false
			h.unverified[i] = true
		default:
			p.Complete = false
		}
	}

	h.meta = hlMeta{
		Hash:        hash,
		Name:        info.Name,
		TotalLength: h.total,
		PieceLength: c.pieceLength,
		PieceCount:  c.pieceCount,
		LastAccess:  now,
	}
	if old != nil {
		h.meta.Pinned, h.meta.Title, h.meta.Poster = old.Pinned, old.Title, old.Poster
		if old.LastAccess > 0 {
			h.meta.LastAccess = old.LastAccess // opening a torrent to list its files is not watching it
		}
	}
	h.dirty = true
	hlByHash[hash] = h
	hlCaches.Store(c, h)
	h.flushLocked()
	hlInvalidateBudget() // its pieces move out of the cache dir scan and into this open cache
}

// hlOnClose — hook at the start of Cache.Close. true — the cache is persistent, RemoveCacheOnDrop does not apply.
func hlOnClose(c *Cache) bool {
	v, ok := hlCaches.LoadAndDelete(c)
	if !ok {
		return false
	}
	h := v.(*hlCache)
	hlMu.Lock()
	if hlByHash[h.meta.Hash] == h {
		delete(hlByHash, h.meta.Hash)
	}
	h.flushLocked()
	hlMu.Unlock()
	hlInvalidateBudget() // its pieces are part of the cache dir scan again
	return true
}

// hlOwnsEviction — hook in Cache.cleanPieces: in persistent mode pieces are evicted only by the janitor.
func hlOwnsEviction(c *Cache) bool {
	return hlGet(c) != nil && hlEnabled()
}

// hlPieceImpl — hook in Cache.Piece: anacrolix talks to the wrapper, which keeps the verified bitmap.
func hlPieceImpl(c *Cache, p *Piece) storage.PieceImpl {
	if h := hlGet(c); h != nil && p.Id < len(h.pieces) {
		return h.pieces[p.Id]
	}
	return p
}

// hlReaderEnd — hook in Reader.getOffsetRange: with BackgroundFill the reader window reaches past the
// upstream window of CacheSize, so while the file is open (playing or paused) upstream priorities keep
// downloading it — the piece under the player first, the rest behind it, all into the disk cache.
// Otherwise the window is CacheSize ahead and a paused player gets a couple of minutes of buffer.
//
// It reaches only as far as the disk budget has room for (homelab_budget.go). Everything inside a reader
// window is protected from eviction, so a window stretched to the end of the file left the janitor
// nothing to drop and the cache grew to the size of the file whatever LimitGB said.
func hlReaderEnd(c *Cache, fileLength, end int64) int64 {
	if hlGet(c) == nil || !hlEnabled() || !settings.GetHomelabSets().BackgroundFill {
		return end
	}
	ahead := hlFillAhead()
	if ahead < 0 {
		return fileLength // nothing limits the cache: the whole file, as before
	}
	if end >= fileLength-ahead {
		return fileLength
	}
	return end + ahead
}

// hlAdjustState — hook at the end of Cache.GetState. Outside the state is what upstream reports: the window of
// CacheSize around each player. Background fill (the window to the end of the file) and downloads are internal:
//   - Readers: the upstream window of each reader, download readers left out. Lampa draws its buffer dots from
//     Readers[0] (Reader → End) and the Completed pieces in a row after it; with End at the file end the dots
//     were red until the whole file was on disk, and a download reader could be taken for the player;
//   - Filled (preloaded_bytes of the torrent — Lampa's preload progress, the buffer bar of the web UI): what is
//     on disk in those windows and in the ranges of a running preload, not the whole disk cache — otherwise
//     preload is "ready" at once even if the start of the file is not on disk;
//   - Pieces: only those windows. The web UI polls /cache ten times a second, and every piece on disk made it
//     hundreds of KB (a whole cached film) — the torrent details choked on it. What is on disk as a whole is
//     shown by the homelab UI (HomelabList, HomelabPieces).
//
// Also the last piece is reported with its real length, otherwise the UI never shows it as complete.
func hlAdjustState(c *Cache, st *state.CacheState) {
	h := hlGet(c)
	if h == nil || st == nil {
		return
	}
	window := make([]Range, 0)
	readers := make([]*state.ReaderState, 0, len(st.Readers))
	for _, r := range c.readersSnapshot() {
		if hlIsDownloadReader(r) {
			continue
		}
		rng := r.hlUpstreamRange()
		readers = append(readers, &state.ReaderState{Start: rng.Start, End: rng.End, Reader: r.getReaderPiece()})
		if r.isUse {
			window = append(window, rng)
		}
	}
	st.Readers = readers
	window = mergeRange(append(window, hlPreloadWindows(c)...)) // a running preload counts too (homelab_preload.go)
	last := c.pieceCount - 1
	if p, ok := st.Pieces[last]; ok {
		p.Length = hlPieceLen(h.total, c.pieceLength, c.pieceCount, last)
		st.Pieces[last] = p
	}
	var fill int64
	for id, p := range st.Pieces {
		if inRanges(window, id) {
			fill += p.Size
		} else {
			delete(st.Pieces, id)
		}
	}
	st.Filled = fill
}

// hlUpstreamRange — the reader window as upstream computes it (getOffsetRange without background fill):
// CacheSize around the reader.
func (r *Reader) hlUpstreamRange() Range {
	prc := int64(settings.BTsets.ReaderReadAHead)
	readers := int64(r.getUseReaders())
	if readers == 0 {
		readers = 1
	}
	begin := max(r.offset-(r.cache.capacity/readers)*(100-prc)/100, 0)
	end := min(r.offset+(r.cache.capacity/readers)*prc/100, r.file.Length())
	return Range{r.getPieceNum(begin), r.getPieceNum(end), r.file}
}

// hlPiece wraps Piece for anacrolix: records hash check results and access times.
type hlPiece struct {
	*Piece
	h *hlCache
}

func (w *hlPiece) ReadAt(b []byte, off int64) (int, error) {
	n, err := w.Piece.ReadAt(b, off)
	if n > 0 {
		w.h.touch(w.Piece)
	}
	return n, err
}

func (w *hlPiece) MarkComplete() error {
	err := w.Piece.MarkComplete()
	w.h.setVerified(w.Id, true)
	return err
}

func (w *hlPiece) MarkNotComplete() error {
	err := w.Piece.MarkNotComplete()
	w.h.setVerified(w.Id, false)
	return err
}

func (w *hlPiece) Completion() storage.Completion {
	w.h.mu.Lock()
	unverified := w.h.unverified[w.Id]
	w.h.mu.Unlock()
	if unverified {
		return storage.Completion{Ok: false} // anacrolix queues a hash check
	}
	return w.Piece.Completion()
}

func (h *hlCache) touch(p *Piece) {
	now := time.Now().Unix()
	h.mu.Lock()
	h.meta.LastAccess = now
	h.dirty = true
	refresh := now-h.touched[p.Id] >= hlTouchInterval
	if refresh {
		h.touched[p.Id] = now
	}
	h.mu.Unlock()
	if refresh && p.dPiece != nil {
		t := time.Unix(now, 0)
		_ = os.Chtimes(p.dPiece.name, t, t)
	}
}

func (h *hlCache) setVerified(id int, ok bool) {
	h.mu.Lock()
	h.verified[id] = ok
	h.unverified[id] = false
	h.dirty = true
	h.mu.Unlock()
}

func (h *hlCache) pinned() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.meta.Pinned
}

// release evicts a piece of an open cache.
func (h *hlCache) release(p *Piece) {
	h.mu.Lock()
	h.verified[p.Id] = false
	h.unverified[p.Id] = false
	h.dirty = true
	h.mu.Unlock()
	h.c.removePiece(p)
}

// flushLocked writes .homelab.json if something changed. Caller holds hlMu.
func (h *hlCache) flushLocked() {
	h.mu.Lock()
	if !h.dirty {
		h.mu.Unlock()
		return
	}
	m := h.meta
	m.Verified = hlEncodeBits(h.verified)
	h.dirty = false
	h.mu.Unlock()
	if err := hlWriteMeta(h.dir, &m); err != nil {
		log.TLogln("homelab: error write cache meta:", err)
	}
}

func hlReadMeta(dir string) (*hlMeta, error) {
	buf, err := os.ReadFile(filepath.Join(dir, hlMetaFile))
	if err != nil {
		return nil, err
	}
	m := new(hlMeta)
	if err := json.Unmarshal(buf, m); err != nil {
		return nil, err
	}
	return m, nil
}

func hlWriteMeta(dir string, m *hlMeta) error {
	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	tmp := filepath.Join(dir, hlMetaFile+".tmp")
	if err := os.WriteFile(tmp, buf, 0o666); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, hlMetaFile))
}

func hlEncodeBits(bits []bool) string {
	buf := make([]byte, (len(bits)+7)/8)
	for i, b := range bits {
		if b {
			buf[i/8] |= 1 << (i % 8)
		}
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func hlDecodeBits(s string, n int) []bool {
	bits := make([]bool, n)
	buf, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return bits
	}
	for i := 0; i < n && i/8 < len(buf); i++ {
		bits[i] = buf[i/8]&(1<<(i%8)) != 0
	}
	return bits
}
