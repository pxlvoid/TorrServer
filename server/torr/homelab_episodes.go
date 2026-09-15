package torr

// homelab: how many episodes of a series are completely in the disk cache — "3/8" on the torrent card and
// in the disk cache dialog (see HOMELAB.md). Episodes are the video files of the torrent: subtitles and
// external audio tracks do not count.

import (
	"mime"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	_ "server/mimetype" // registers video extensions missing in the system mime table (mkv, ts...)
	"server/torr/storage/torrstor"
	utils2 "server/utils"
)

type hlSpan struct{ start, length int64 }

type hlVideoLayout struct {
	pieceLength int64
	videos      []hlSpan
}

// hlFileSpan — a file of the torrent as file_stats has it: the id (from 1, files sorted by path), the path as the
// engine builds it (name/path...), where it is in the torrent.
type hlFileSpan struct {
	id             int
	path           string
	offset, length int64
}

var (
	hlLayouts sync.Map // hash → *hlVideoLayout; the file list of a torrent never changes
	hlInfos   sync.Map // hash → *metainfo.Info of torrents not loaded (from the TorrServer list)
)

// hlInfo — the metainfo of a torrent: loaded, or from its record in the TorrServer list.
func hlInfo(hash string) *metainfo.Info {
	if tt := hlDlLoaded(hash); tt != nil {
		return tt.Info()
	}
	if v, ok := hlInfos.Load(hash); ok {
		return v.(*metainfo.Info)
	}
	if !hlDlValidHash(hash) {
		return nil
	}
	db := GetTorrentDB(metainfo.NewHashFromHex(hash))
	if db == nil || db.TorrentSpec == nil || len(db.TorrentSpec.InfoBytes) == 0 {
		return nil
	}
	info := new(metainfo.Info)
	if err := bencode.Unmarshal(db.TorrentSpec.InfoBytes, info); err != nil {
		return nil
	}
	hlInfos.Store(hash, info)
	return info
}

// hlFileSpans — the files of the torrent in the order and with the ids of file_stats (as hlDlSelect for a loaded one).
func hlFileSpans(info *metainfo.Info) []hlFileSpan {
	var spans []hlFileSpan
	var offset int64
	for _, fi := range info.UpvertedFiles() {
		p := strings.Join(append([]string{info.BestName()}, fi.BestPath()...), "/")
		spans = append(spans, hlFileSpan{path: p, offset: offset, length: fi.Length})
		offset += fi.Length
	}
	sort.SliceStable(spans, func(i, j int) bool { return utils2.CompareStrings(spans[i].path, spans[j].path) })
	for i := range spans {
		spans[i].id = i + 1
	}
	return spans
}

// hlLayout — video files of the torrent, from the loaded torrent or from its record in the TorrServer list.
func hlLayout(hash string) *hlVideoLayout {
	if v, ok := hlLayouts.Load(hash); ok {
		return v.(*hlVideoLayout)
	}
	info := hlInfo(hash)
	if info == nil || info.PieceLength <= 0 {
		return nil
	}
	layout := &hlVideoLayout{pieceLength: info.PieceLength}
	for _, f := range hlFileSpans(info) {
		if hlIsVideo(f.path) && f.length > 0 {
			layout.videos = append(layout.videos, hlSpan{f.offset, f.length})
		}
	}
	hlLayouts.Store(hash, layout)
	return layout
}

// HomelabDiskFiles — the files of a torrent that is not loaded and how much of each is in the disk cache
// (from its record in the TorrServer list and the pieces on disk); nil if unknown.
func HomelabDiskFiles(hash string) []HomelabFileState {
	info := hlInfo(strings.ToLower(hash))
	if info == nil || info.PieceLength <= 0 {
		return nil
	}
	complete := torrstor.HomelabComplete(strings.ToLower(hash))
	files := make([]HomelabFileState, 0)
	for _, f := range hlFileSpans(info) {
		done := int64(0)
		if complete != nil {
			done = hlSpanDone(f.offset, f.length, complete, info.PieceLength)
		}
		files = append(files, HomelabFileState{Id: f.id, Path: f.path, Length: f.length, Done: done})
	}
	return files
}

// hlIsVideo — by the extension only: the file is not on disk to sniff.
func hlIsVideo(name string) bool {
	return strings.HasPrefix(mime.TypeByExtension(strings.ToLower(path.Ext(name))), "video/") ||
		strings.EqualFold(path.Ext(name), ".rmvb")
}

// HomelabEpisodes — video files of the torrent and how many of them are completely on disk.
// ok=false if the file list is unknown (the torrent is neither loaded nor in the TorrServer list).
func HomelabEpisodes(hash string) (done, total int, ok bool) {
	layout := hlLayout(strings.ToLower(hash))
	if layout == nil {
		return 0, 0, false
	}
	complete := torrstor.HomelabComplete(strings.ToLower(hash))
	for _, v := range layout.videos {
		if complete != nil && hlSpanNext(v.start, v.length, complete, layout.pieceLength) < 0 {
			done++
		}
	}
	return done, len(layout.videos), true
}
