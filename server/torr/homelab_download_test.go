package torr

// homelab: tests of "download to disk" (homelab_download.go).

import (
	"reflect"
	"strings"
	"testing"

	"server/settings"
)

// Pieces of 10 bytes; a file of 25 bytes from offset 5 covers pieces 0..2 (5 + 10 + 10 bytes).
func TestHomelabSpanDone(t *testing.T) {
	complete := []bool{true, false, true, true}
	if got := hlSpanDone(5, 25, complete, 10); got != 5+10 {
		t.Fatalf("done: got %d, want 15", got)
	}
	if got := hlSpanDone(0, 40, []bool{true, true, true, true}, 10); got != 40 {
		t.Fatalf("all complete: got %d", got)
	}
	// the last piece of the torrent is short: a file ending in it counts only its own bytes
	if got := hlSpanDone(30, 3, []bool{false, false, false, true}, 10); got != 3 {
		t.Fatalf("short last piece: got %d", got)
	}
	if got := hlSpanDone(5, 0, complete, 10); got != 0 {
		t.Fatalf("empty file: got %d", got)
	}
}

func TestHomelabSpanNext(t *testing.T) {
	complete := []bool{true, false, true, false}
	if got := hlSpanNext(5, 25, complete, 10); got != 1 {
		t.Fatalf("first missing piece: got %d, want 1", got)
	}
	if got := hlSpanNext(20, 10, complete, 10); got != -1 {
		t.Fatalf("file in a complete piece: got %d, want -1", got)
	}
	if got := hlSpanNext(20, 15, complete, 10); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}

func TestHomelabFileSets(t *testing.T) {
	if got := hlUnion([]int{3, 1}, []int{2, 3}); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("union: %v", got)
	}
	if got := hlMinus([]int{1, 2, 3}, []int{2}); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("minus: %v", got)
	}
	if !hlDlValidHash("0123456789abcdef0123456789abcdef01234567") || hlDlValidHash("zz") ||
		hlDlValidHash("0123456789abcdef0123456789abcdef0123456z") {
		t.Fatal("hash validation")
	}
}

func TestHomelabIsVideo(t *testing.T) {
	for _, name := range []string{"The.Boys.S05E01.mkv", "a.MP4", "b.avi", "c.ts", "d.m2ts"} {
		if !hlIsVideo(name) {
			t.Fatalf("%s must be a video", name)
		}
	}
	for _, name := range []string{"s01e01.srt", "track.ac3", "cover.jpg", "info.nfo", "noext"} {
		if hlIsVideo(name) {
			t.Fatalf("%s must not be a video", name)
		}
	}
}

// Removing a torrent (one or "Remove all") takes its download off the queue.
func TestHomelabOnRemoveDropsDownload(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	hlDlMu.Lock()
	saved := hlDlJobs
	hlDlJobs = []*hlDlJob{{HomelabDownloadJob: settings.HomelabDownloadJob{Hash: hash}, state: "queued"}}
	hlDlMu.Unlock()
	t.Cleanup(func() {
		hlDlMu.Lock()
		hlDlJobs = saved
		hlDlMu.Unlock()
	})

	hlOnRemove(strings.ToUpper(hash))
	if jobs := HomelabDownloads(); len(jobs) != 0 {
		t.Fatalf("the job of a removed torrent must be gone: %+v", jobs)
	}
}
