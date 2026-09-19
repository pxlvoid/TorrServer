package torrstor

// homelab: the disk budget of the persistent cache — how much the cache may hold and how far ahead
// background fill may reach (see homelab.go).
//
// Background fill used to stretch the reader window to the end of the file, and everything inside a
// reader window is protected from eviction (Cache.getRemPieces). So while a film played the janitor was
// allowed to drop almost nothing and the cache grew to the size of the file whatever LimitGB said: a
// 40 GB remux filled the disk with the limit set to 20 GB. Now the window reaches only as far as the
// budget has room for.
//
// The fill spends free budget and never evicts to get more. When the budget is full it stops and
// playback goes on with the upstream window of CacheSize; as the player moves, that window pulls new
// pieces, the cache goes over the limit, the janitor trims the least recently used ones from behind and
// the fill gets its runway back. The result is a sliding window of exactly LimitGB — every piece is
// still downloaded once, and nothing a player is about to need is thrown away, because everything
// inside a reader window stays protected.
//
// The budget is the smaller of LimitGB and what the disk can spare (hlMinFree always stays free), so a
// limit larger than the free space can no longer fill the disk up.

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"server/settings"
)

const (
	// how long a scan of the cache dir (bytes held by torrents that are not open) is reused
	hlClosedTTL = 30 * time.Second
	// how long a computed fill runway is reused: hlReaderEnd sits on the piece read/write path
	hlFillTTL = time.Second
)

// hlDiskStat — free and total bytes of the cache disk (homelab_statfs*.go), replaceable in tests.
var hlDiskStat = hlDiskStatSys

var (
	hlClosedMu    sync.Mutex
	hlClosedBytes int64
	hlClosedAt    time.Time

	hlFillMu    sync.Mutex
	hlFillValue int64
	hlFillAt    time.Time

	// Bumped whenever what the cache holds changes under us (a torrent opened or closed, something was
	// removed). A scan started before the bump may not publish its result afterwards: a torrent that
	// closed mid-scan is in neither half of the count, and the fill would think it has that much more
	// room — the one direction that overfills the disk.
	hlBudgetEpoch atomic.Uint64
)

// hlAllowed — how many bytes the cache may hold: LimitGB, and never so much that less than hlMinFree is
// left free on the disk. total is what it holds now, free/diskOK come from hlDiskStat. -1 — no limit at
// all (no LimitGB and no disk stats). byDisk — the free space guard is what binds, not LimitGB.
func hlAllowed(total, free int64, diskOK bool) (allowed int64, byDisk bool) {
	allowed = -1
	if sets := settings.GetHomelabSets(); sets.LimitGB > 0 {
		allowed = sets.LimitGB << 30
	}
	if diskOK {
		// the cache may grow by the free space beyond the reserve — and must shrink if it is already eaten
		byFree := total + free - hlMinFree
		if byFree < 0 {
			byFree = 0
		}
		if allowed < 0 || byFree < allowed {
			allowed, byDisk = byFree, true
		}
	}
	return allowed, byDisk
}

// hlOpenBytes — bytes held by the caches of open torrents.
func hlOpenBytes(open []*hlCache) int64 {
	var total int64
	for _, h := range open {
		if h.c.isClosed.Load() {
			continue
		}
		for _, p := range h.c.getPieces() {
			if p.Size > 0 {
				total += p.Size
			}
		}
	}
	return total
}

// hlScanClosed — bytes held by the piece files of torrents that are not open.
func hlScanClosed(openHash map[string]bool) int64 {
	root := hlRoot()
	if root == "" {
		return 0
	}
	var total int64
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() || !hlIsHash(e.Name()) || openHash[e.Name()] {
			continue
		}
		for _, f := range hlPieceFiles(filepath.Join(root, e.Name())) {
			total += f.size
		}
	}
	return total
}

// hlClosedTotal — hlScanClosed, rescanned at most every hlClosedTTL: only the janitor and an explicit
// removal change what closed torrents hold, and both publish the new value (hlPublishClosed).
func hlClosedTotal(openHash map[string]bool) int64 {
	hlClosedMu.Lock()
	fresh, v := !hlClosedAt.IsZero() && time.Since(hlClosedAt) < hlClosedTTL, hlClosedBytes
	hlClosedMu.Unlock()
	if fresh {
		return v
	}
	epoch := hlBudgetEpoch.Load()
	v = hlScanClosed(openHash)
	hlPublishClosed(epoch, v)
	return v
}

// hlPublishClosed stores a freshly measured size of the closed part of the cache — unless the cache
// changed while it was being measured (see hlBudgetEpoch), in which case the next caller measures again.
func hlPublishClosed(epoch uint64, v int64) {
	if v < 0 {
		v = 0
	}
	hlClosedMu.Lock()
	if hlBudgetEpoch.Load() == epoch {
		hlClosedBytes, hlClosedAt = v, time.Now()
	}
	hlClosedMu.Unlock()
}

// hlInvalidateBudget forces the next reader window to measure the cache again: a torrent has just been
// opened or closed, or the disk cache dialog has removed something.
func hlInvalidateBudget() {
	hlBudgetEpoch.Add(1)
	hlClosedMu.Lock()
	hlClosedAt = time.Time{}
	hlClosedMu.Unlock()
	hlFillMu.Lock()
	hlFillAt = time.Time{}
	hlFillMu.Unlock()
}

// hlFillAhead — how far past the upstream window background fill may reach, per reader in use.
// -1 — nothing limits the cache, fill to the end of the file; 0 — the budget is spent, upstream window only.
func hlFillAhead() int64 {
	hlFillMu.Lock()
	if !hlFillAt.IsZero() && time.Since(hlFillAt) < hlFillTTL {
		v := hlFillValue
		hlFillMu.Unlock()
		return v
	}
	hlFillMu.Unlock()

	open := hlOpenSnapshot()
	openHash := make(map[string]bool, len(open))
	readers := 0
	for _, h := range open {
		openHash[h.meta.Hash] = true
		if !h.c.isClosed.Load() {
			readers += h.c.GetUseReaders()
		}
	}
	total := hlOpenBytes(open) + hlClosedTotal(openHash)
	free, _, diskOK := hlDiskStat(hlRoot())
	allowed, _ := hlAllowed(total, free, diskOK)

	v := int64(-1)
	if allowed >= 0 {
		v = max(allowed-total, 0)
		if readers > 1 {
			v /= int64(readers) // the readers in use share the runway, they all fill into the same budget
		}
	}

	hlFillMu.Lock()
	hlFillValue, hlFillAt = v, time.Now()
	hlFillMu.Unlock()
	return v
}
