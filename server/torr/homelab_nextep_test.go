package torr

// homelab: tests of the next-episode prefetch (homelab_nextep.go).

import (
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

// three episodes of 32 bytes, pieces of 16: episode i owns pieces 2i and 2i+1
func hlNextTestVideos(t *testing.T) ([]hlFileSpan, int64) {
	t.Helper()
	info := &metainfo.Info{Name: "Show.S01", PieceLength: 16, Files: []metainfo.FileInfo{
		{Path: []string{"Show.S01E01.mkv"}, Length: 32},
		{Path: []string{"Show.S01E02.mkv"}, Length: 32},
		{Path: []string{"Show.S01E03.mkv"}, Length: 32},
		{Path: []string{"subs.srt"}, Length: 8}, // not a video: must not be picked
	}}
	var videos []hlFileSpan
	for _, f := range hlFileSpans(info) {
		if hlIsVideo(f.path) && f.length > 0 {
			videos = append(videos, f)
		}
	}
	if len(videos) != 3 {
		t.Fatalf("videos: %+v", videos)
	}
	return videos, info.PieceLength
}

func TestHomelabNextPick(t *testing.T) {
	videos, pl := hlNextTestVideos(t)
	e1, e2, e3 := videos[0].path, videos[1].path, videos[2].path

	// nothing on disk: the player still needs the network, do not compete with it
	if got := hlNextPick(videos, pl, []bool{false, false, false, false, false, false, false}, e1); got != nil {
		t.Fatalf("nothing on disk: %+v", got)
	}

	// the episode being watched is on disk — fetch the next one
	done1 := []bool{true, true, false, false, false, false, false}
	got := hlNextPick(videos, pl, done1, e1)
	if got == nil || got.path != e2 {
		t.Fatalf("after E01 must come E02, got %+v", got)
	}

	// halfway through the current one is not enough
	if got := hlNextPick(videos, pl, []bool{true, false, false, false, false, false, false}, e1); got != nil {
		t.Fatalf("half of the current episode: %+v", got)
	}

	// the next one is already there: nothing to do
	done12 := []bool{true, true, true, true, false, false, false}
	if got := hlNextPick(videos, pl, done12, e1); got != nil {
		t.Fatalf("E02 already on disk: %+v", got)
	}
	// ...but from E02 the one after it is now due
	if got := hlNextPick(videos, pl, done12, e2); got == nil || got.path != e3 {
		t.Fatalf("after E02 must come E03, got %+v", got)
	}

	// the last episode has no successor
	all := []bool{true, true, true, true, true, true, true}
	if got := hlNextPick(videos, pl, all, e3); got != nil {
		t.Fatalf("nothing follows the last episode: %+v", got)
	}
	// an unknown file
	if got := hlNextPick(videos, pl, all, "Show.S01/nope.mkv"); got != nil {
		t.Fatalf("unknown file: %+v", got)
	}
	// a film: a single video file, nothing to prefetch
	if got := hlNextPick(videos[:1], pl, all, e1); got != nil {
		t.Fatalf("a single file is not a series: %+v", got)
	}
	// no disk map yet
	if got := hlNextPick(videos, pl, nil, e1); got != nil {
		t.Fatalf("no disk map: %+v", got)
	}
}
