package torr

// homelab: the next episode of a series, fetched to disk while the current one is being watched
// (pxlvoid/TorrServer fork, see HOMELAB.md).
//
// A season is one torrent, and autoplay moves to the next file the moment an episode ends. Without this
// the next episode starts from nothing: a fresh preload, and with a thin swarm a wait or a stall.
//
// It only fetches once the episode being watched is already on disk. A download of the same torrent
// holds a reader of its own, and Cache.setLoadPriority splits the connection limit between the readers —
// prefetching while the player still needs the network would take half of it away from the player, which
// is exactly when it can least afford it. Once the current episode is complete, background fill has
// nothing left to do and the bandwidth is free, so the prefetch costs the viewer nothing.
//
// The job is queued as "auto" (homelab_download.go): it yields to downloads the user asked for and gives
// up quietly when it does not fit the budget, instead of complaining in ntfy.

import (
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/log"
	"server/settings"
	"server/torr/storage/torrstor"
)

const (
	hlNextTick = 15 * time.Second
	// the episode being watched counts as "on disk" at this share of it: pieces on the boundary with the
	// neighbouring file may never be counted as this file's own
	hlNextReady = 0.99
	// ignore a file that was opened and left: somebody has to be actually watching
	hlNextMinWatched = 0.05
)

// episodes already queued this run, so a job the user stopped is not queued again behind their back
var hlNextQueued sync.Map // "hash|id" → struct{}

var hlNextOnce sync.Once

// HomelabNextEpisodeStart starts the background prefetch (once).
func HomelabNextEpisodeStart() {
	hlNextOnce.Do(func() {
		go func() {
			for {
				time.Sleep(hlNextTick)
				hlNextPass()
			}
		}()
	})
}

func hlNextPass() {
	defer func() {
		if r := recover(); r != nil {
			log.TLogln("homelab: next episode panic:", r)
		}
	}()
	if !settings.HomelabPersistentCache() || !settings.GetHomelabSets().NextEpisode {
		return
	}
	for _, c := range HomelabStreams() {
		if c.Ended || c.Position < hlNextMinWatched {
			continue
		}
		hlNextFor(c.Hash, c.Path)
	}
}

// hlNextVideos — the video files of a torrent, in the order and with the ids of file_stats.
func hlNextVideos(hash string) ([]hlFileSpan, int64) {
	info := hlInfo(hash)
	if info == nil || info.PieceLength <= 0 {
		return nil, 0
	}
	var videos []hlFileSpan
	for _, f := range hlFileSpans(info) {
		if hlIsVideo(f.path) && f.length > 0 {
			videos = append(videos, f)
		}
	}
	return videos, info.PieceLength
}

// hlNextPick — the episode that should be fetched next, or nil. videos are the video files of the torrent
// in playing order, complete says which pieces are on disk. Nothing is picked unless the episode being
// watched is on disk: until then the network belongs to the player.
func hlNextPick(videos []hlFileSpan, pieceLength int64, complete []bool, filePath string) *hlFileSpan {
	if len(videos) < 2 || complete == nil || pieceLength <= 0 {
		return nil // a film: nothing follows it
	}
	cur := -1
	for i, v := range videos {
		if v.path == filePath {
			cur = i
			break
		}
	}
	if cur < 0 || cur+1 >= len(videos) {
		return nil // unknown file, or the last episode
	}
	watched := videos[cur]
	if float64(hlSpanDone(watched.offset, watched.length, complete, pieceLength)) < float64(watched.length)*hlNextReady {
		return nil // the player still needs the network itself
	}
	next := videos[cur+1]
	if hlSpanNext(next.offset, next.length, complete, pieceLength) < 0 {
		return nil // already on disk
	}
	return &next
}

// hlNextFor queues the episode after filePath, if the one being watched is already on disk.
func hlNextFor(hash, filePath string) {
	hash = strings.ToLower(hash)
	videos, pieceLength := hlNextVideos(hash)
	next := hlNextPick(videos, pieceLength, torrstor.HomelabComplete(hash), filePath)
	if next == nil {
		return
	}
	key := hash + "|" + strconv.Itoa(next.id)
	if _, seen := hlNextQueued.Load(key); seen {
		return
	}
	// do not touch a torrent that already has a job: the user's own download knows better
	for _, d := range HomelabDownloads() {
		if d.Hash == hash {
			return
		}
	}
	if err := hlDlStart(hash, []int{next.id}, true); err != nil {
		log.TLogln("homelab: next episode not queued:", hash, err)
		return
	}
	hlNextQueued.Store(key, struct{}{})
	log.TLogln("homelab: next episode queued to disk:", path.Base(next.path))
}
