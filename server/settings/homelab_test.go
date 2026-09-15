package settings

// homelab: the download queue must survive a restart with JsonDB (StoreSettingsInJson) — it keeps only
// objects under a key, a bare array was silently not written.

import (
	"io/fs"
	"reflect"
	"testing"
)

func TestHomelabDownloadsJsonDBRoundTrip(t *testing.T) {
	oldDB, oldReadOnly := tdb, ReadOnly
	tdb = &JsonDB{
		Path:              t.TempDir(),
		filenameDelimiter: ".",
		filenameExtension: ".json",
		fileMode:          fs.FileMode(0o666),
		xPathDelimeter:    "/",
	}
	ReadOnly = false
	t.Cleanup(func() { tdb, ReadOnly = oldDB, oldReadOnly })

	jobs := []HomelabDownloadJob{
		{Hash: "0123456789abcdef0123456789abcdef01234567", Files: []int{1, 3}, Added: 100, WasPinned: true},
		{Hash: "fedcba9876543210fedcba9876543210fedcba98", Added: 200},
	}
	SetHomelabDownloads(jobs)
	if got := GetHomelabDownloads(); !reflect.DeepEqual(got, jobs) {
		t.Fatalf("round trip: got %+v, want %+v", got, jobs)
	}
	SetHomelabDownloads(nil)
	if got := GetHomelabDownloads(); len(got) != 0 {
		t.Fatalf("emptied queue: got %+v", got)
	}
}

func TestHomelabAudioJsonDBRoundTrip(t *testing.T) {
	oldDB, oldReadOnly, oldCached := tdb, ReadOnly, homelabAudioCached
	dir := t.TempDir()
	newDB := func() TorrServerDB {
		return &JsonDB{Path: dir, filenameDelimiter: ".", filenameExtension: ".json", fileMode: fs.FileMode(0o666), xPathDelimeter: "/"}
	}
	tdb, ReadOnly, homelabAudioCached = newDB(), false, nil
	t.Cleanup(func() { tdb, ReadOnly, homelabAudioCached = oldDB, oldReadOnly, oldCached })

	hash := "0123456789ABCDEF0123456789ABCDEF01234567"
	choice := HomelabAudioChoice{Tracks: []HomelabAudioPref{{Name: "LostFilm", Lang: "rus"}, {Name: "HDrezka Studio", Lang: "rus"}}}
	if err := SetHomelabAudio(hash, &choice); err != nil {
		t.Fatal(err)
	}
	homelabAudioCached = nil // as after a restart: read from the file
	if got, ok := GetHomelabAudio(hash); !ok || !reflect.DeepEqual(got, choice) {
		t.Fatalf("round trip: got %+v %v", got, ok)
	}
	if err := SetHomelabAudio(hash, nil); err != nil {
		t.Fatal(err)
	}
	homelabAudioCached = nil
	if _, ok := GetHomelabAudio(hash); ok {
		t.Fatal("cleared choice must be gone")
	}
}
