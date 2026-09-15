package torrstor

// homelab: a preload is a window of the cache for Filled (hlAdjustState). TorrServer's Preload reads the start and
// the end of the file with engine readers of its own, not torrstor readers — no window — so in persistent mode
// preloaded_bytes stayed 0 and Lampa (starts the player at 95% of preload_size, gives up after 30 s without
// progress) never played. The ranges a preload reads count while it runs and for a while after: from disk it ends
// in a moment, before Lampa asks — as in upstream, where what was preloaded stays in the cache.

import (
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

// how long the ranges of a finished preload still count: Lampa polls every second and gives up after 30 s
const hlPreloadKeep = 2 * time.Minute

type hlPreload struct {
	ranges []Range
	until  time.Time // zero — still running
}

var (
	hlPreloadMu     sync.Mutex
	hlPreloadSeq    int
	hlPreloadRanges = map[*Cache]map[int]*hlPreload{} // cache → preload → the piece ranges it reads
)

// HomelabPreloadStart — hook in Torrent.Preload: while it reads size bytes of file (and hlPreloadKeep after), its
// ranges are a window of the cache. The returned func marks the preload finished (defer it).
func HomelabPreloadStart(c *Cache, file *torrent.File, size int64) func() {
	if c == nil || file == nil || c.pieceLength <= 0 {
		return func() {}
	}
	ranges := hlPreloadPieces(file.Offset(), file.Length(), size, c.pieceLength)
	for i := range ranges {
		ranges[i].File = file
	}
	hlPreloadMu.Lock()
	hlPreloadSweepLocked(time.Now())
	hlPreloadSeq++
	id := hlPreloadSeq
	if hlPreloadRanges[c] == nil {
		hlPreloadRanges[c] = map[int]*hlPreload{}
	}
	p := &hlPreload{ranges: ranges}
	hlPreloadRanges[c][id] = p
	hlPreloadMu.Unlock()
	return func() {
		hlPreloadMu.Lock()
		p.until = time.Now().Add(hlPreloadKeep)
		hlPreloadMu.Unlock()
	}
}

// hlPreloadSweepLocked forgets finished preloads past hlPreloadKeep (closed caches included).
func hlPreloadSweepLocked(now time.Time) {
	for c, preloads := range hlPreloadRanges {
		for id, p := range preloads {
			if !p.until.IsZero() && now.After(p.until) {
				delete(preloads, id)
			}
		}
		if len(preloads) == 0 {
			delete(hlPreloadRanges, c)
		}
	}
}

// hlPreloadPieces — the pieces Preload reads, as it does: the start of the file up to size minus the tail, and the
// tail (a piece, at least 8 MB) — so on disk they add up to about size.
func hlPreloadPieces(offset, length, size, pieceLength int64) []Range {
	if length <= 0 || pieceLength <= 0 {
		return nil
	}
	size = min(size, length)
	tail := max(pieceLength, 8<<20)
	startEnd := size - tail
	if startEnd <= 0 {
		startEnd = size
	}
	ranges := []Range{{Start: int(offset / pieceLength), End: int((offset + startEnd - 1) / pieceLength)}}
	if length > tail {
		ranges = append(ranges, Range{Start: int((offset + length - tail) / pieceLength), End: int((offset + length - 1) / pieceLength)})
	}
	return ranges
}

func hlPreloadWindows(c *Cache) []Range {
	hlPreloadMu.Lock()
	defer hlPreloadMu.Unlock()
	hlPreloadSweepLocked(time.Now())
	var out []Range
	for _, p := range hlPreloadRanges[c] {
		out = append(out, p.ranges...)
	}
	return out
}
