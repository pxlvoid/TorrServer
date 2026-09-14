package torrstor

// homelab: tests of the persistent disk cache. They run on every upstream merge (homelab-upstream.yml):
// if an upstream change breaks the hooks, it shows up here and not on the TV.

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"server/settings"
	"server/torr/storage/state"
)

func hlTestSettings(t *testing.T, sets settings.HomelabSets) string {
	t.Helper()
	root := t.TempDir()
	prevBT := settings.BTsets
	prevHL := settings.GetHomelabSets()
	settings.BTsets = &settings.BTSets{UseDisk: true, TorrentsSavePath: root, RemoveCacheOnDrop: true}
	settings.SetHomelabSetsForTest(sets)
	t.Cleanup(func() {
		settings.BTsets = prevBT
		settings.SetHomelabSetsForTest(prevHL)
	})
	return root
}

func hlWriteFile(t *testing.T, path string, size int64, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil { // sparse: "gigabytes" without using the disk
		t.Fatal(err)
	}
	f.Close()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func hlExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestHomelabBitsRoundTrip(t *testing.T) {
	bits := []bool{true, false, false, true, true, false, false, false, true, false, true}
	got := hlDecodeBits(hlEncodeBits(bits), len(bits))
	for i := range bits {
		if got[i] != bits[i] {
			t.Fatalf("bit %d: got %v want %v", i, got[i], bits[i])
		}
	}
	if hlDecodeBits("not base64!", 3)[0] {
		t.Fatal("garbage must decode to all false")
	}
}

func TestHomelabPieceLen(t *testing.T) {
	if n := hlPieceLen(58, 16, 4, 3); n != 10 {
		t.Fatalf("last piece: %d", n)
	}
	if n := hlPieceLen(58, 16, 4, 1); n != 16 {
		t.Fatalf("middle piece: %d", n)
	}
}

// After a restart: verified pieces are trusted (the short last one too), full-size files without
// the mark go to anacrolix hash check, partial ones are downloaded; drop keeps files and the mark.
func TestHomelabInitCompletionAndClose(t *testing.T) {
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: true})
	hash := metainfo.NewHashFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	dir := filepath.Join(root, hash.HexString())
	info := &metainfo.Info{Name: "Season 1", PieceLength: 16, Length: 16*3 + 10, Pieces: make([]byte, 4*20)}

	old := time.Now().Add(-48 * time.Hour)
	hlWriteFile(t, filepath.Join(dir, "0"), 16, old) // verified
	hlWriteFile(t, filepath.Join(dir, "1"), 16, old) // full size, not verified
	hlWriteFile(t, filepath.Join(dir, "2"), 8, old)  // partial
	hlWriteFile(t, filepath.Join(dir, "3"), 10, old) // verified short last piece
	if err := hlWriteMeta(dir, &hlMeta{Hash: hash.HexString(), PieceLength: 16, PieceCount: 4, LastAccess: 12345,
		Pinned: true, Verified: hlEncodeBits([]bool{true, false, false, true})}); err != nil {
		t.Fatal(err)
	}

	stor := NewStorage(64)
	impl, err := stor.OpenTorrent(info, hash)
	if err != nil {
		t.Fatal(err)
	}
	c := impl.(*Cache)
	comp := func(i int) storage.Completion { return c.Piece(info.Piece(i)).Completion() }

	if got := comp(0); !got.Ok || !got.Complete {
		t.Fatalf("piece 0 (verified): %+v", got)
	}
	if got := comp(1); got.Ok {
		t.Fatalf("piece 1 (unverified) must ask for a hash check: %+v", got)
	}
	if got := comp(2); !got.Ok || got.Complete {
		t.Fatalf("piece 2 (partial): %+v", got)
	}
	if got := comp(3); !got.Ok || !got.Complete {
		t.Fatalf("piece 3 (short last piece, verified): %+v", got)
	}

	// anacrolix checked piece 1 and it is fine
	if err := c.Piece(info.Piece(1)).MarkComplete(); err != nil {
		t.Fatal(err)
	}
	if got := comp(1); !got.Ok || !got.Complete {
		t.Fatalf("piece 1 after hash check: %+v", got)
	}
	if !hlOwnsEviction(c) {
		t.Fatal("persistent cache must own eviction")
	}

	c.Close() // RemoveCacheOnDrop is on, but the persistent cache must survive
	for i := 0; i < 4; i++ {
		if !hlExists(filepath.Join(dir, strconv.Itoa(i))) {
			t.Fatalf("piece %d removed on drop", i)
		}
	}
	m, err := hlReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	bits := hlDecodeBits(m.Verified, 4)
	if !bits[0] || !bits[1] || bits[2] || !bits[3] {
		t.Fatalf("verified bits after close: %v", bits)
	}
	if !m.Pinned || m.LastAccess != 12345 || m.Name != "Season 1" {
		t.Fatalf("meta after close: %+v", m)
	}
	if hlByHashGet(hash.HexString()) != nil {
		t.Fatal("closed cache still registered")
	}
}

// Filled of a persistent cache is the reader window only (no readers — nothing), the last piece has its real length.
func TestHomelabAdjustState(t *testing.T) {
	hlTestSettings(t, settings.HomelabSets{PersistentCache: true})
	hash := metainfo.NewHashFromHex("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	info := &metainfo.Info{Name: "x", PieceLength: 16, Length: 16*2 + 5, Pieces: make([]byte, 3*20)}
	impl, _ := NewStorage(64).OpenTorrent(info, hash)
	c := impl.(*Cache)
	defer c.Close()

	st := &state.CacheState{Filled: 37, Pieces: map[int]state.ItemState{
		0: {Id: 0, Size: 16, Length: 16}, 1: {Id: 1, Size: 16, Length: 16}, 2: {Id: 2, Size: 5, Length: 16},
	}}
	hlAdjustState(c, st)
	if st.Filled != 0 {
		t.Fatalf("no readers — Filled must be 0, got %d", st.Filled)
	}
	if st.Pieces[2].Length != 5 {
		t.Fatalf("last piece length: %d", st.Pieces[2].Length)
	}
}

func TestHomelabReaderEnd(t *testing.T) {
	hlTestSettings(t, settings.HomelabSets{PersistentCache: true, BackgroundFill: true})
	info := &metainfo.Info{Name: "x", PieceLength: 16, Length: 64, Pieces: make([]byte, 4*20)}
	impl, _ := NewStorage(64).OpenTorrent(info, metainfo.NewHashFromHex("ffffffffffffffffffffffffffffffffffffffff"))
	c := impl.(*Cache)
	defer c.Close()

	if got := hlReaderEnd(c, 1000, 100); got != 1000 {
		t.Fatalf("background fill: window must reach the end of the file, got %d", got)
	}
	settings.SetHomelabSetsForTest(settings.HomelabSets{PersistentCache: true, BackgroundFill: false})
	if got := hlReaderEnd(c, 1000, 100); got != 100 {
		t.Fatalf("background fill off: upstream window, got %d", got)
	}
	if got := hlReaderEnd(&Cache{}, 1000, 100); got != 100 {
		t.Fatalf("not a persistent cache: upstream window, got %d", got)
	}
}

// Without the persistent mode the hooks must keep upstream behavior.
func TestHomelabOffKeepsUpstream(t *testing.T) {
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: false})
	hash := metainfo.NewHashFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	info := &metainfo.Info{Name: "x", PieceLength: 16, Length: 32, Pieces: make([]byte, 2*20)}
	hlWriteFile(t, filepath.Join(root, hash.HexString(), "0"), 16, time.Now())

	stor := NewStorage(64)
	impl, _ := stor.OpenTorrent(info, hash)
	c := impl.(*Cache)
	if _, wrapped := c.Piece(info.Piece(0)).(*hlPiece); wrapped || hlOwnsEviction(c) {
		t.Fatal("off: cache must not be taken over")
	}
	c.Close()
	if hlExists(filepath.Join(root, hash.HexString(), "0")) {
		t.Fatal("off: RemoveCacheOnDrop must work as upstream")
	}
}

func TestHomelabJanitorBudget(t *testing.T) {
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: true, LimitGB: 4})
	now := time.Now()
	day := 24 * time.Hour
	a := filepath.Join(root, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	b := filepath.Join(root, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	c := filepath.Join(root, "cccccccccccccccccccccccccccccccccccccccc")
	hlWriteFile(t, filepath.Join(a, "0"), 1<<30, now.Add(-10*day))
	hlWriteFile(t, filepath.Join(a, "1"), 1<<30, now.Add(-10*day))
	hlWriteFile(t, filepath.Join(b, "0"), 1<<30, now.Add(-3*day))
	hlWriteFile(t, filepath.Join(b, "1"), 1<<30, now.Add(-1*day))
	hlWriteFile(t, filepath.Join(c, "0"), 1<<30, now.Add(-20*day)) // pinned: oldest, but stays
	hlWriteFile(t, filepath.Join(c, "1"), 1<<30, now.Add(-20*day))
	if err := hlWriteMeta(c, &hlMeta{Hash: filepath.Base(c), Pinned: true}); err != nil {
		t.Fatal(err)
	}

	// 6 GB with a 4 GB limit → down to 95% of it (3.8 GB): both of A, then the older piece of B
	hlJanitorPass(now)

	if hlExists(a) {
		t.Fatal("a: least recently used, must be gone with its dir")
	}
	if hlExists(filepath.Join(b, "0")) || !hlExists(filepath.Join(b, "1")) {
		t.Fatal("b: only the older piece must be evicted")
	}
	if !hlExists(filepath.Join(c, "0")) || !hlExists(filepath.Join(c, "1")) {
		t.Fatal("c: pinned, must stay")
	}
}

func TestHomelabJanitorKeepDays(t *testing.T) {
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: true, KeepDays: 7})
	now := time.Now()
	old := filepath.Join(root, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	fresh := filepath.Join(root, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hlWriteFile(t, filepath.Join(old, "0"), 16, now.Add(-8*24*time.Hour))
	hlWriteFile(t, filepath.Join(fresh, "0"), 16, now.Add(-6*24*time.Hour))
	hlWriteFile(t, filepath.Join(root, "not-a-torrent", "0"), 16, now.Add(-30*24*time.Hour))

	hlJanitorPass(now)

	if hlExists(old) {
		t.Fatal("not accessed for 8 days with KeepDays=7 — must be gone")
	}
	if !hlExists(filepath.Join(fresh, "0")) {
		t.Fatal("6 days old — must stay")
	}
	if !hlExists(filepath.Join(root, "not-a-torrent", "0")) {
		t.Fatal("foreign dirs in the cache path must not be touched")
	}
}

func TestHomelabRemoveAndPin(t *testing.T) {
	root := hlTestSettings(t, settings.HomelabSets{PersistentCache: true})
	hash := "dddddddddddddddddddddddddddddddddddddddd"
	dir := filepath.Join(root, hash)
	hlWriteFile(t, filepath.Join(dir, "0"), 100, time.Now())

	if err := HomelabSetPinned(hash, true); err != nil {
		t.Fatal(err)
	}
	items, usage := HomelabList()
	if len(items) != 1 || !items[0].Pinned || items[0].Size != 100 || usage.Used != 100 {
		t.Fatalf("list: %+v %+v", items, usage)
	}
	if freed := HomelabClear(); freed != 0 || !hlExists(dir) {
		t.Fatal("clear must skip pinned")
	}
	if freed, err := HomelabRemove(hash); err != nil || freed != 100 || hlExists(dir) {
		t.Fatalf("remove: freed=%d err=%v", freed, err)
	}
	if _, err := HomelabRemove("../../etc"); err != ErrHomelabBadHash {
		t.Fatalf("path traversal must be rejected: %v", err)
	}
}
