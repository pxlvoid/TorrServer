package torr

// homelab: preload that stops as soon as the buffer is enough (pxlvoid/TorrServer fork, see HOMELAB.md).
//
// Upstream buffers a fixed share of CacheSize before playback — the same 215 MB for a 1080p episode with
// fifty seeders and for a 4K remux with two. For the well seeded file that is half a minute of waiting for
// nothing: the swarm gives tens of times the bitrate, so the download runs away from the player anyway and
// a few seconds of video is all the head start it needs.
//
// So the preload keeps running as upstream wrote it, but is cut short once the file is clearly fast
// enough: we know the bitrate (ffprobe has run by then) and what the swarm gives (DownloadSpeed), and
// while the swarm stays comfortably above the bitrate the buffer only grows during playback.
//
// It never buffers more than upstream would: when the swarm can not beat the bitrate the configured size
// stands, because a thin swarm is exactly where the buffer earns its keep.

import (
	"strconv"

	"github.com/anacrolix/torrent"

	"server/settings"
)

const (
	// the swarm must be this much faster than the file before the buffer is called enough; a margin,
	// not a guess: the speed of a swarm swings, and below the bitrate the buffer drains instead of growing
	hlPreloadMargin = 2.0
	// seconds of video to have in hand when the swarm is that fast
	hlPreloadSeconds = 20.0
	// never stop below this, however small the bitrate: the player needs a header and room to probe
	hlPreloadFloor = int64(16 << 20)
)

// hlBitrate — bytes per second the file needs to play, 0 if unknown.
func hlBitrate(t *Torrent, file *torrent.File) float64 {
	if t == nil {
		return 0
	}
	if t.DurationSeconds > 0 && file != nil && file.Length() > 0 {
		return float64(file.Length()) / t.DurationSeconds
	}
	if bits, err := strconv.ParseFloat(t.BitRate, 64); err == nil && bits > 0 {
		return bits / 8
	}
	return 0
}

// hlPreloadEnough — the buffer got is enough to start playing: at least a floor, at least a few seconds of
// video, and the swarm is comfortably faster than the file. floor also takes whole pieces into account.
func hlPreloadEnough(bitrate, speed float64, got, floor int64) bool {
	if bitrate <= 0 || speed <= 0 {
		return false // nothing to reason from: buffer as configured
	}
	if got < floor || got < int64(bitrate*hlPreloadSeconds) {
		return false
	}
	return speed >= bitrate*hlPreloadMargin
}

// hlPreloadStop — hook in Torrent.Preload: stop buffering, what is there is enough.
func hlPreloadStop(t *Torrent, file *torrent.File, got, pieceLength int64) bool {
	if !settings.GetHomelabSets().AdaptivePreload {
		return false
	}
	floor := hlPreloadFloor
	if n := pieceLength * 2; n > floor {
		floor = n
	}
	return hlPreloadEnough(hlBitrate(t, file), t.DownloadSpeed, got, floor)
}
