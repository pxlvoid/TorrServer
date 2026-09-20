package torrstor

// homelab: tests of the disk budget — the regression that let a 40 GB film fill the disk with the
// limit set to 20 GB (homelab_budget.go).

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

// hlTestDiskStat pins what the cache disk reports, so the budget does not depend on the machine.
func hlTestDiskStat(t *testing.T, free, total int64) {
	t.Helper()
	prev := hlDiskStat
	hlDiskStat = func(string) (int64, int64, bool) { return free, total, true }
	t.Cleanup(func() {
		hlDiskStat = prev
		hlInvalidateBudget()
	})
	hlInvalidateBudget()
}

// hlTestNoDiskStat — a platform without statfs: only LimitGB applies.
func hlTestNoDiskStat(t *testing.T) {
	t.Helper()
	prev := hlDiskStat
	hlDiskStat = func(string) (int64, int64, bool) { return 0, 0, false }
	t.Cleanup(func() {
		hlDiskStat = prev
		hlInvalidateBudget()
	})
	hlInvalidateBudget()
}

func TestHomelabAllowed(t *testing.T) {
	gb := int64(1) << 30
	tests := []struct {
		name    string
		limitGB int64
		total   int64
		free    int64
		diskOK  bool
		allowed int64
		byDisk  bool
	}{
		{name: "limit only, no disk stats", limitGB: 20, diskOK: false, allowed: 20 * gb},
		{name: "roomy disk: the limit binds", limitGB: 20, total: 5 * gb, free: 100 * gb, diskOK: true, allowed: 20 * gb},
		// no limit: the cache may grow by the free space beyond the reserve
		{name: "no limit: the disk binds", total: 10 * gb, free: 8 * gb, diskOK: true, allowed: 13 * gb, byDisk: true},
		// a limit larger than the disk must not let the cache fill it up
		{name: "limit over the disk", limitGB: 100, total: 10 * gb, free: 2 * gb, diskOK: true, allowed: 7 * gb, byDisk: true},
		// the reserve is already eaten: the cache has to shrink
		{name: "reserve eaten", limitGB: 100, total: 10 * gb, free: 1 * gb, diskOK: true, allowed: 6 * gb, byDisk: true},
		{name: "nothing limits it", diskOK: false, allowed: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hlTestSettings(t, settings.HomelabSets{PersistentCache: true, LimitGB: tc.limitGB})
			allowed, byDisk := hlAllowed(tc.total, tc.free, tc.diskOK)
			if allowed != tc.allowed || byDisk != tc.byDisk {
				t.Fatalf("got allowed=%d byDisk=%v, want allowed=%d byDisk=%v", allowed, byDisk, tc.allowed, tc.byDisk)
			}
		})
	}
}

// The runway of background fill is what is left of the budget, and the cache dir of torrents that are
// not open counts towards it.
func TestHomelabFillAhead(t *testing.T) {
	gb := int64(1) << 30
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: true, BackgroundFill: true, LimitGB: 4})
	hlTestDiskStat(t, 100*gb, 200*gb)

	if got := hlFillAhead(); got != 4*gb {
		t.Fatalf("empty cache: the whole limit is runway, got %d", got)
	}

	other := filepath.Join(root, "cccccccccccccccccccccccccccccccccccccccc")
	hlWriteFile(t, filepath.Join(other, "0"), 3*gb, time.Now())
	hlInvalidateBudget()
	if got := hlFillAhead(); got != gb {
		t.Fatalf("3 GB of 4 GB used: 1 GB of runway, got %d", got)
	}

	hlWriteFile(t, filepath.Join(other, "1"), 2*gb, time.Now())
	hlInvalidateBudget()
	if got := hlFillAhead(); got != 0 {
		t.Fatalf("over the limit: no runway, got %d", got)
	}
}

// Without a limit and without disk stats nothing bounds the cache — fill to the end of the file, as before.
func TestHomelabFillAheadUnlimited(t *testing.T) {
	hlTestSettings(t, settings.HomelabSets{PersistentCache: true, BackgroundFill: true})
	hlTestNoDiskStat(t)
	if got := hlFillAhead(); got != -1 {
		t.Fatalf("no limit and no disk stats: unbounded, got %d", got)
	}
}

// The free space guard bounds the fill even with a limit that does not fit on the disk.
func TestHomelabFillAheadDiskGuard(t *testing.T) {
	gb := int64(1) << 30
	hlTestSettings(t, settings.HomelabSets{PersistentCache: true, BackgroundFill: true, LimitGB: 500})
	hlTestDiskStat(t, 7*gb, 100*gb) // 7 GB free, hlMinFree (5 GB) must stay
	if got := hlFillAhead(); got != 2*gb {
		t.Fatalf("only the free space beyond the reserve is runway, got %d", got)
	}
}

// The regression: the reader window of background fill must stop at the budget. With the window
// stretched to the end of the file everything in it was protected from eviction, so the janitor could
// drop nothing and a 40 GB film filled the disk with the limit set to 20 GB.
func TestHomelabReaderEndStopsAtBudget(t *testing.T) {
	gb := int64(1) << 30
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: true, BackgroundFill: true, LimitGB: 4})
	hlTestDiskStat(t, 100*gb, 200*gb)

	// 3 of the 4 GB are held by a torrent that is not open: 1 GB of runway is left
	other := filepath.Join(root, "cccccccccccccccccccccccccccccccccccccccc")
	hlWriteFile(t, filepath.Join(other, "0"), 3*gb, time.Now())

	info := &metainfo.Info{Name: "x", PieceLength: 1 << 20, Length: 10 * gb, Pieces: make([]byte, 10*1024*20)}
	impl, err := NewStorage(64).OpenTorrent(info, metainfo.NewHashFromHex("ffffffffffffffffffffffffffffffffffffffff"))
	if err != nil {
		t.Fatal(err)
	}
	c := impl.(*Cache)
	defer c.Close()

	hlInvalidateBudget()
	// The upstream window ends 1 GB into the file and the runway adds at most the 1 GB left of the budget —
	// never the whole 10 GB, which is the bug this guards. Bounds rather than an exact value: the runway is
	// shared between the readers in use, and in this package other tests may still hold some.
	got := hlReaderEnd(c, 10*gb, gb)
	if got <= gb {
		t.Fatalf("background fill must reach past the upstream window, got %d", got)
	}
	if got > 2*gb {
		t.Fatalf("window must stop at the budget (%d max), got %d", 2*gb, got)
	}

	// a file whose tail is within the runway is filled to its very end, so the last piece is not left out
	hlInvalidateBudget()
	if got := hlReaderEnd(c, gb+1024, gb); got != gb+1024 {
		t.Fatalf("a file within reach must be filled to the end, got %d", got)
	}

	// the budget is spent: the upstream window and nothing more
	hlWriteFile(t, filepath.Join(other, "1"), 2*gb, time.Now())
	hlInvalidateBudget()
	if got := hlReaderEnd(c, 10*gb, gb); got != gb {
		t.Fatalf("no runway: upstream window only, got %d", got)
	}
}
