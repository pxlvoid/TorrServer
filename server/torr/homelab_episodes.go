package torr

// homelab: how many episodes of a series are completely in the disk cache — "3/8" on the torrent card and
// in the disk cache dialog (see HOMELAB.md). Episodes are the video files of the torrent: subtitles and
// external audio tracks do not count.

import (
	"mime"
	"path"
	"strings"
	"sync"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	_ "server/mimetype" // registers video extensions missing in the system mime table (mkv, ts...)
	"server/torr/storage/torrstor"
)

type hlSpan struct{ start, length int64 }

type hlVideoLayout struct {
	pieceLength int64
	videos      []hlSpan
}

var hlLayouts sync.Map // hash → *hlVideoLayout; the file list of a torrent never changes

// hlLayout — video files of the torrent, from the loaded torrent or from its record in the TorrServer list.
func hlLayout(hash string) *hlVideoLayout {
	if v, ok := hlLayouts.Load(hash); ok {
		return v.(*hlVideoLayout)
	}
	var info *metainfo.Info
	if tt := hlDlLoaded(hash); tt != nil {
		info = tt.Info()
	} else if hlDlValidHash(hash) {
		db := GetTorrentDB(metainfo.NewHashFromHex(hash))
		if db == nil || db.TorrentSpec == nil || len(db.TorrentSpec.InfoBytes) == 0 {
			return nil
		}
		info = new(metainfo.Info)
		if err := bencode.Unmarshal(db.TorrentSpec.InfoBytes, info); err != nil {
			return nil
		}
	}
	if info == nil || info.PieceLength <= 0 {
		return nil
	}
	layout := &hlVideoLayout{pieceLength: info.PieceLength}
	var offset int64
	for _, fi := range info.UpvertedFiles() {
		name := info.Name
		if len(fi.Path) > 0 {
			name = fi.Path[len(fi.Path)-1]
		}
		if hlIsVideo(name) && fi.Length > 0 {
			layout.videos = append(layout.videos, hlSpan{offset, fi.Length})
		}
		offset += fi.Length
	}
	hlLayouts.Store(hash, layout)
	return layout
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
