package torr

// homelab: tests of who is watching (homelab_streams.go).

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestHomelabDevice(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Linux; Android 11; MiTV-MOOQ1 Build/RTM4.220307.164; wv) AppleWebKit/537.36": "Xiaomi TV",
		"Mozilla/5.0 (SMART-TV; LINUX; Tizen 9.0) AppleWebKit/537.36":                              "Samsung TV",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15":              "iPhone",
		"AppleCoreMedia/1.0.0.22H20 (iPhone; U; CPU OS 18_7 like Mac OS X; ru_ru)":                 "iPhone",
		"VLC/3.0.21 LibVLC/3.0.21":             "VLC",
		"Lavf/61.7.100":                        "ffmpeg",
		"Mozilla/5.0 (Windows NT 10.0; Win64)": "Windows",
		"":                                     "?",
		"SomePlayer/2.1 (custom)":              "SomePlayer",
	} {
		if got := hlDevice(ua); got != want {
			t.Fatalf("%q: got %q, want %q", ua, got, want)
		}
	}
}

func hlTestStream(t *testing.T, ip, ua, path string, started time.Time) *hlStream {
	s := &hlStream{hash: "h", title: "Призраки в Венеции", path: path, length: 1000, ip: ip, ua: ua, started: started}
	hlStreamsMu.Lock()
	hlStreams[s] = struct{}{}
	hlStreamsMu.Unlock()
	t.Cleanup(func() {
		hlStreamsMu.Lock()
		delete(hlStreams, s)
		hlStreamsMu.Unlock()
	})
	return s
}

// Requests of one device and file are one client; the speed is over the last seconds; an ended request lingers.
func TestHomelabStreamClients(t *testing.T) {
	t0 := time.Now().Add(-time.Minute)
	tv := "Mozilla/5.0 (Linux; Android 11; MiTV-MOOQ1 Build/RTM4; wv)"
	a := hlTestStream(t, "91.77.161.136", tv, "film.mkv", t0)
	b := hlTestStream(t, "91.77.161.136", tv, "film.mkv", t0.Add(10*time.Second)) // a seek: a second request
	phone := hlTestStream(t, "92.36.45.102", "AppleCoreMedia/1.0 (iPhone)", "film.mkv", t0)

	now := t0.Add(50 * time.Second)
	// the TV: 2 MB/s over the last 4 s on request b, request a idle for long
	a.bytes, a.offset, a.lastRead = 300, 300, t0.Add(20*time.Second)
	b.samples = []hlSample{{now.Add(-4 * time.Second), 100}}
	b.bytes, b.offset, b.lastRead = 100+8<<20, 500, now.Add(-time.Second)
	// the phone: nothing read for a while — paused (or its buffer is full)
	phone.bytes, phone.offset, phone.lastRead = 50, 900, now.Add(-30*time.Second)
	phone.ended = now.Add(-10 * time.Second) // its request ended 10 s ago: still listed

	clients := hlStreamClients(now)
	if len(clients) != 2 {
		t.Fatalf("clients: %+v", clients)
	}
	c := clients[0]
	if c.Device != "Xiaomi TV" || c.Connections != 2 || !c.Active || c.Ended || c.Position != 0.5 {
		t.Fatalf("TV: %+v", c)
	}
	if c.Speed < 1.9*(1<<20) || c.Speed > 2.1*(1<<20) {
		t.Fatalf("TV speed: %.0f, want ~2 MB/s", c.Speed)
	}
	p := clients[1]
	if p.Device != "iPhone" || p.Active || !p.Ended || p.Speed != 0 || p.Position != 0.9 {
		t.Fatalf("phone: %+v", p)
	}

	// a closed request: no speed even if it read within the last seconds
	phone.lastRead = now.Add(-time.Second)
	phone.samples = []hlSample{{now.Add(-3 * time.Second), 0}}
	if got := hlStreamClients(now); got[1].Speed != 0 {
		t.Fatalf("closed request with speed: %+v", got[1])
	}

	// 20 s after its request ended the phone is gone
	if got := hlStreamClients(now.Add(10 * time.Second)); len(got) != 1 {
		t.Fatalf("after the linger: %+v", got)
	}
}

// Requests of TorrServer to itself (ffprobe of a preload) are not clients; behind Caddy the address is X-Forwarded-For.
func TestHomelabStreamOpenAddress(t *testing.T) {
	local := httptest.NewRequest("GET", "/play/h/1", nil)
	local.RemoteAddr = "127.0.0.1:50000"
	if s := hlStreamOpen(local, &Torrent{}, nil); s != nil {
		t.Fatal("no file: no stream")
	}
	if ip, fwd := hlClientIP(local); ip != "127.0.0.1" || fwd {
		t.Fatalf("local: %s %v", ip, fwd)
	}
	proxied := httptest.NewRequest("GET", "/stream/x", nil)
	proxied.RemoteAddr = "172.18.0.5:40000"
	proxied.Header.Set("X-Forwarded-For", "91.77.161.136, 172.18.0.5")
	if ip, fwd := hlClientIP(proxied); ip != "91.77.161.136" || !fwd {
		t.Fatalf("proxied: %s %v", ip, fwd)
	}
}
