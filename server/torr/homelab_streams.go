package torr

// homelab: who is watching (pxlvoid/TorrServer fork, see HOMELAB.md) — the clients of Torrent.Stream (Lampa, VLC,
// "Open link"): the device, the torrent and file, where the player is and how fast it takes the data.
// Every request of a player is a stream (a seek is a new request, some players keep several); a client is a device
// playing a file: streams of the same address, User-Agent and file. A client stays listed a little after its last
// request ends, so a seek does not blink it away. Not counted: the GStreamer HLS player, DLNA, WebDAV/FUSE — they
// read the torrent their own way — and requests TorrServer makes to itself (ffprobe of a preload).

import (
	"io"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"

	"server/torr/storage/torrstor"
)

const (
	hlStreamLinger  = 15 * time.Second // a client stays listed after its last request
	hlStreamWindow  = 5 * time.Second  // speed — over this much of recent data
	hlStreamSamples = 8
)

// hlStreamReader — hook in Torrent.Stream: the reader the file is served through. It counts what the client gets
// and, with an audio track chosen (homelab_audio.go), hides the other audio tracks.
func hlStreamReader(t *Torrent, file *torrent.File, r io.ReadSeeker, req *http.Request, resp http.ResponseWriter) io.ReadSeeker {
	patches := hlAudioPatches(t, file, r, resp)
	s := hlStreamOpen(req, t, file)
	if s == nil && len(patches) == 0 {
		return r
	}
	return &hlPatchReader{ReadSeeker: r, patches: patches, stream: s}
}

type hlSample struct {
	at    time.Time
	bytes int64
}

type hlStream struct {
	hash, title, path, ip, ua string
	length, fileOffset, pl    int64 // the file in the torrent and its piece length: how much of it is on disk
	started                   time.Time

	mu       sync.Mutex
	bytes    int64
	offset   int64
	lastRead time.Time
	samples  []hlSample // one per second at most, the last hlStreamSamples
	ended    time.Time  // zero — the request is still going
}

var (
	hlStreamsMu sync.Mutex
	hlStreams   = map[*hlStream]struct{}{}
)

// hlClientIP — the address of the client: behind Caddy the first X-Forwarded-For.
func hlClientIP(req *http.Request) (ip string, forwarded bool) {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0]), true
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	return host, false
}

// hlStreamOpen registers a request of a player; it is ended when the request is (its context is done).
// nil for requests of TorrServer to itself.
func hlStreamOpen(req *http.Request, t *Torrent, file *torrent.File) *hlStream {
	if req == nil || t == nil || file == nil {
		return nil
	}
	ip, forwarded := hlClientIP(req)
	if parsed := net.ParseIP(ip); !forwarded && parsed != nil && parsed.IsLoopback() {
		return nil // ffprobe of a preload and the like
	}
	title := t.Title
	if title == "" && t.Torrent != nil {
		title = t.Torrent.Name()
	}
	s := &hlStream{
		hash: t.Hash().HexString(), title: title, path: file.Path(), length: file.Length(), fileOffset: file.Offset(),
		ip: ip, ua: req.UserAgent(), started: time.Now(),
	}
	if info := file.Torrent().Info(); info != nil {
		s.pl = info.PieceLength
	}
	hlStreamsMu.Lock()
	hlStreams[s] = struct{}{}
	hlStreamsMu.Unlock()
	go func() {
		<-req.Context().Done()
		s.mu.Lock()
		s.ended = time.Now()
		s.mu.Unlock()
	}()
	return s
}

// read — n bytes of the file from off went to the client.
func (s *hlStream) read(off int64, n int) {
	now := time.Now()
	s.mu.Lock()
	s.bytes += int64(n)
	s.offset = off + int64(n)
	s.lastRead = now
	if len(s.samples) == 0 || now.Sub(s.samples[len(s.samples)-1].at) >= time.Second {
		s.samples = append(s.samples, hlSample{now, s.bytes})
		if len(s.samples) > hlStreamSamples {
			s.samples = s.samples[1:]
		}
	}
	s.mu.Unlock()
}

// speedLocked — bytes per second over the last hlStreamWindow; 0 if nothing was read in it.
func (s *hlStream) speedLocked(now time.Time) float64 {
	if now.Sub(s.lastRead) > hlStreamWindow {
		return 0
	}
	for _, sm := range s.samples {
		if now.Sub(sm.at) <= hlStreamWindow {
			if dt := now.Sub(sm.at).Seconds(); dt >= 0.5 {
				return float64(s.bytes-sm.bytes) / dt
			}
			break
		}
	}
	// a burst within the last half a second: the whole of it over the time since the first sample
	if len(s.samples) > 0 {
		if dt := now.Sub(s.samples[0].at).Seconds(); dt >= 0.5 {
			return float64(s.bytes-s.samples[0].bytes) / dt
		}
	}
	return 0
}

// HomelabStreamClient — a device playing a file.
type HomelabStreamClient struct {
	Device      string  `json:"device"` // "Xiaomi TV", "iPhone", "VLC"…
	IP          string  `json:"ip"`
	UA          string  `json:"ua"`
	Hash        string  `json:"hash"`
	Title       string  `json:"title"`
	Path        string  `json:"path"`
	Position    float64 `json:"position"` // 0..1 of the file, where the player reads
	Speed       float64 `json:"speed"`    // bytes per second, now
	Bytes       int64   `json:"bytes"`    // sent since Since
	Since       int64   `json:"since"`    // unix
	Active      bool    `json:"active"`   // data flows now (otherwise paused or its buffer is full)
	Connections int     `json:"connections"`
	Ended       bool    `json:"ended"` // no request any more, listed for hlStreamLinger

	FileLength int64   `json:"fileLength"`
	Offset     int64   `json:"offset"`     // where the player reads, bytes of the file
	OnDisk     float64 `json:"onDisk"`     // share of the file in the disk cache, -1 — unknown
	NetSpeed   float64 `json:"netSpeed"`   // the torrent downloads from peers now, bytes per second
	Peers      int     `json:"peers"`      // active peers of the torrent
	TotalPeers int     `json:"totalPeers"` // known peers
}

// HomelabStreams — the clients now, the ones with most data flowing first.
func HomelabStreams() []HomelabStreamClient {
	return hlStreamClients(time.Now())
}

func hlStreamClients(now time.Time) []HomelabStreamClient {
	hlStreamsMu.Lock()
	streams := make([]*hlStream, 0, len(hlStreams))
	for s := range hlStreams {
		s.mu.Lock()
		gone := !s.ended.IsZero() && now.Sub(s.ended) > hlStreamLinger
		s.mu.Unlock()
		if gone {
			delete(hlStreams, s)
			continue
		}
		streams = append(streams, s)
	}
	hlStreamsMu.Unlock()

	type agg struct {
		c              HomelabStreamClient
		lastRead       time.Time
		fileOffset, pl int64
	}
	clients := map[string]*agg{}
	for _, s := range streams {
		s.mu.Lock()
		key := s.ip + "|" + s.ua + "|" + s.hash + "|" + s.path
		a := clients[key]
		if a == nil {
			a = &agg{c: HomelabStreamClient{
				Device: hlDevice(s.ua), IP: s.ip, UA: s.ua, Hash: s.hash, Title: s.title, Path: s.path,
				Since: s.started.Unix(), Ended: true, FileLength: s.length, OnDisk: -1,
			}, fileOffset: s.fileOffset, pl: s.pl}
			clients[key] = a
		}
		speed := 0.0
		if s.ended.IsZero() { // a closed request sends nothing, whatever it read in its last seconds
			speed = s.speedLocked(now)
		}
		a.c.Speed += speed
		a.c.Bytes += s.bytes
		if s.started.Unix() < a.c.Since {
			a.c.Since = s.started.Unix()
		}
		if s.ended.IsZero() {
			a.c.Ended = false
			a.c.Connections++
		}
		if speed > 0 {
			a.c.Active = true
		}
		if s.length > 0 && (a.lastRead.IsZero() || s.lastRead.After(a.lastRead)) && !s.lastRead.IsZero() {
			a.lastRead = s.lastRead
			a.c.Position = min(1, float64(s.offset)/float64(s.length))
			a.c.Offset = s.offset
		}
		s.mu.Unlock()
	}
	out := make([]HomelabStreamClient, 0, len(clients))
	complete := map[string][]bool{} // per torrent, for every client of it
	for _, a := range clients {
		c := &a.c
		if _, ok := complete[c.Hash]; !ok {
			complete[c.Hash] = torrstor.HomelabComplete(c.Hash)
		}
		if pieces := complete[c.Hash]; pieces != nil && a.pl > 0 && c.FileLength > 0 {
			c.OnDisk = float64(hlSpanDone(a.fileOffset, c.FileLength, pieces, a.pl)) / float64(c.FileLength)
		}
		if tt := hlDlLoaded(c.Hash); tt != nil {
			st := tt.Stats()
			c.Peers, c.TotalPeers = st.ActivePeers, st.TotalPeers
			if t := bts.GetTorrent(tt.InfoHash()); t != nil {
				c.NetSpeed = t.DownloadSpeed
			}
		}
		out = append(out, a.c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Speed != out[j].Speed {
			return out[i].Speed > out[j].Speed
		}
		return out[i].Since < out[j].Since
	})
	return out
}

// hlDevice — a short name of the device from the User-Agent.
func hlDevice(ua string) string {
	l := strings.ToLower(ua)
	for _, d := range []struct{ key, name string }{
		{"mitv", "Xiaomi TV"},
		{"tizen", "Samsung TV"},
		{"web0s", "LG TV"},
		{"webos", "LG TV"},
		{"bravia", "Sony TV"},
		{"; aft", "Fire TV"},
		{"android tv", "Android TV"},
		{"vlc", "VLC"},
		{"kodi", "Kodi"},
		{"infuse", "Infuse"},
		{"mpv", "mpv"},
		{"exoplayer", "Android"},
		{"iphone", "iPhone"},
		{"ipad", "iPad"},
		{"applecoremedia", "Apple"},
		{"apple tv", "Apple TV"},
		{"macintosh", "Mac"},
		{"windows", "Windows"},
		{"android", "Android"},
		{"linux", "Linux"},
		{"lavf", "ffmpeg"},
	} {
		if strings.Contains(l, d.key) {
			return d.name
		}
	}
	if ua == "" {
		return "?"
	}
	name := strings.Fields(ua)[0]
	return path.Base(strings.Split(name, "/")[0])
}
