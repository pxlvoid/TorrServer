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

// HomelabPinned — the torrent is pinned (never evicted by the janitor).
func HomelabPinned(hash string) bool {
	if !hlIsHash(hash) {
		return false
	}
	if h := hlByHashGet(hash); h != nil {
		return h.pinned()
	}
	meta, _ := hlReadMeta(filepath.Join(hlRoot(), hash))
	return meta != nil && meta.Pinned
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

// HomelabComplete — which pieces of the torrent are complete in the disk cache, nil if nothing is known.
// Loaded torrents answer from memory; closed ones from the verified mark and the piece file sizes.
func HomelabComplete(hash string) []bool {
	if !hlIsHash(hash) {
		return nil
	}
	c := hlUpstreamOpen(hash)
	if h := hlByHashGet(hash); h != nil {
		c = h.c
	}
	if c != nil {
		out := make([]bool, c.pieceCount)
		for id, p := range c.getPieces() {
			if id >= 0 && id < len(out) {
				out[id] = p.Complete
			}
		}
		return out
	}

	dir := filepath.Join(hlRoot(), hash)
	meta, _ := hlReadMeta(dir)
	if meta == nil || meta.PieceCount <= 0 || meta.PieceLength <= 0 {
		return nil
	}
	verified := hlDecodeBits(meta.Verified, meta.PieceCount)
	out := make([]bool, meta.PieceCount)
	for _, f := range hlPieceFiles(dir) {
		id, err := strconv.Atoi(filepath.Base(f.path))
		if err != nil || id < 0 || id >= meta.PieceCount {
			continue
		}
		out[id] = verified[id] && f.size == hlPieceLen(meta.TotalLength, meta.PieceLength, meta.PieceCount, id)
	}
	return out
}
