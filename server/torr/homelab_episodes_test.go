package torr

// homelab: tests of the file list of a torrent that is not loaded (homelab_episodes.go).

import (
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

// Ids and paths as file_stats has them: files sorted by path from 1, the path is name/path, offsets in torrent order.
func TestHomelabFileSpans(t *testing.T) {
	info := &metainfo.Info{Name: "Show.S01", PieceLength: 16, Files: []metainfo.FileInfo{
		{Path: []string{"Show.S01E02.mkv"}, Length: 10},
		{Path: []string{"Show.S01E01.mkv"}, Length: 20},
		{Path: []string{"subs", "ru.srt"}, Length: 5},
	}}
	spans := hlFileSpans(info)
	if len(spans) != 3 {
		t.Fatalf("spans: %+v", spans)
	}
	want := []hlFileSpan{
		{1, "Show.S01/Show.S01E01.mkv", 10, 20},
		{2, "Show.S01/Show.S01E02.mkv", 0, 10},
		{3, "Show.S01/subs/ru.srt", 30, 5},
	}
	for i, w := range want {
		if spans[i] != w {
			t.Fatalf("span %d: %+v, want %+v", i, spans[i], w)
		}
	}
	single := hlFileSpans(&metainfo.Info{Name: "Film.mkv", PieceLength: 16, Length: 100})
	if len(single) != 1 || single[0].path != "Film.mkv" || single[0].id != 1 {
		t.Fatalf("single file: %+v", single)
	}
}
