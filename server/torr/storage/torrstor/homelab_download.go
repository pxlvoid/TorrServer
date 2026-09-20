package torrstor

// homelab: what the cache needs to know about "download to disk" (torr/homelab_download.go, see homelab.go).
// A download holds an ordinary reader on the file it fetches. Such readers are marked here: they are not
// someone watching (Playing, the "Filled" buffer of the player), they are just a way to use upstream
// piece priorities for the whole file.

import (
	"path/filepath"
	"strconv"
	"sync"
)

var hlDlReaders sync.Map // *Reader → struct{}

// HomelabMarkDownloadReader / HomelabUnmarkDownloadReader — the reader belongs to a download, not to a player.
func HomelabMarkDownloadReader(r *Reader) {
	if r != nil {
		hlDlReaders.Store(r, struct{}{})
	}
}

func HomelabUnmarkDownloadReader(r *Reader) {
	if r != nil {
		hlDlReaders.Delete(r)
	}
}

func hlIsDownloadReader(r *Reader) bool {
	_, ok := hlDlReaders.Load(r)
	return ok
}

// hlPlayers — readers in use that are not downloads: someone is watching the torrent.
func hlPlayers(c *Cache) int {
	if c == nil {
		return 0
	}
	n := 0
	for _, r := range c.readersSnapshot() {
		if r.isUse && !hlIsDownloadReader(r) {
			n++
		}
	}
	return n
}

// HomelabIsOpen — the torrent's cache is loaded (persistent or upstream).
func HomelabIsOpen(hash string) bool {
	if !hlIsHash(hash) {
		return false
	}
	return hlByHashGet(hash) != nil || hlUpstreamOpen(hash) != nil
}

// A download in progress holds its torrent: the janitor leaves its cache alone until the job ends.
// Without it the janitor could evict, under the limit, the very pieces the job has just fetched, and the
// job would fetch them again — forever. The hold is automatic and lasts only while the job runs; it is
// what the manual pin used to be needed for.
var hlHeld sync.Map // hash → struct{}

// HomelabHoldDownload keeps the cache of a torrent while its download runs; the returned func releases it.
func HomelabHoldDownload(hash string) func() {
	if !hlIsHash(hash) {
		return func() {}
	}
	hlHeld.Store(hash, struct{}{})
	return func() { hlHeld.Delete(hash) }
}

// hlIsHeld — a download is running for this torrent.
func hlIsHeld(hash string) bool {
	_, ok := hlHeld.Load(hash)
	return ok
}

// HomelabDiskFree — free space on the cache disk; ok=false if unknown.
func HomelabDiskFree() (free int64, ok bool) {
	root := hlRoot()
	if root == "" {
		return 0, false
	}
	free, _, ok = hlDiskStat(root)
	return free, ok
}

// HomelabMinFree — what the janitor always keeps free on the cache disk.
const HomelabMinFree = hlMinFree

// hlPieceStates — the pieces of a torrent in the disk cache: complete ones and bytes of each; ok=false if nothing
// is known. Loaded torrents answer from memory; closed ones from the verified mark and the piece file sizes.
func hlPieceStates(hash string) (complete []bool, size []int64, pieceLength, total int64, ok bool) {
	if !hlIsHash(hash) {
		return nil, nil, 0, 0, false
	}
	c := hlUpstreamOpen(hash)
	h := hlByHashGet(hash)
	if h != nil {
		c = h.c
	}
	if c != nil {
		complete, size = make([]bool, c.pieceCount), make([]int64, c.pieceCount)
		for id, p := range c.getPieces() {
			if id >= 0 && id < c.pieceCount {
				complete[id], size[id] = p.Complete, p.Size
			}
		}
		total = c.pieceLength * int64(c.pieceCount)
		if h != nil {
			total = h.total
		}
		return complete, size, c.pieceLength, total, true
	}

	dir := filepath.Join(hlRoot(), hash)
	meta, _ := hlReadMeta(dir)
	if meta == nil || meta.PieceCount <= 0 || meta.PieceLength <= 0 {
		return nil, nil, 0, 0, false
	}
	verified := hlDecodeBits(meta.Verified, meta.PieceCount)
	complete, size = make([]bool, meta.PieceCount), make([]int64, meta.PieceCount)
	for _, f := range hlPieceFiles(dir) {
		id, err := strconv.Atoi(filepath.Base(f.path))
		if err != nil || id < 0 || id >= meta.PieceCount {
			continue
		}
		want := hlPieceLen(meta.TotalLength, meta.PieceLength, meta.PieceCount, id)
		size[id] = min(f.size, want)
		complete[id] = verified[id] && f.size == want
	}
	return complete, size, meta.PieceLength, meta.TotalLength, true
}

// HomelabComplete — which pieces of the torrent are complete in the disk cache, nil if nothing is known.
func HomelabComplete(hash string) []bool {
	complete, _, _, _, _ := hlPieceStates(hash)
	return complete
}

// HomelabPieceMap — what of a torrent is in the disk cache, compact: bitsets (base64, bit i — piece i) of complete
// and of partly downloaded pieces. For the disk timeline in the torrent details: the /cache state no longer carries
// every piece on disk (hlAdjustState).
type HomelabPieceMap struct {
	PieceCount  int    `json:"pieceCount"`
	PieceLength int64  `json:"pieceLength"`
	TotalLength int64  `json:"totalLength"`
	Bytes       int64  `json:"bytes"` // on disk
	Complete    string `json:"complete"`
	Partial     string `json:"partial"`
}

// HomelabPieces — the disk map of a torrent; ok=false if nothing is known.
func HomelabPieces(hash string) (HomelabPieceMap, bool) {
	complete, size, pieceLength, total, ok := hlPieceStates(hash)
	if !ok {
		return HomelabPieceMap{}, false
	}
	partial := make([]bool, len(size))
	m := HomelabPieceMap{PieceCount: len(size), PieceLength: pieceLength, TotalLength: total}
	for i, n := range size {
		m.Bytes += n
		partial[i] = n > 0 && !complete[i]
	}
	m.Complete, m.Partial = hlEncodeBits(complete), hlEncodeBits(partial)
	return m, true
}
