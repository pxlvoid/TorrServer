package torr

// homelab: tests of the preload that stops early (homelab_preload.go).

import "testing"

func TestHomelabPreloadEnough(t *testing.T) {
	const mb = 1 << 20
	floor := int64(16 * mb)

	// a 1080p episode at 6 Mbit/s with a swarm giving 8 MB/s: 20 s of video is 15 MB, so the floor decides
	bitrate := 6.0 * mb / 8 // 0.75 MB/s
	if hlPreloadEnough(bitrate, 8*mb, 16*mb, floor) != true {
		t.Fatal("a swarm ten times the bitrate: 16 MB must be enough")
	}
	if hlPreloadEnough(bitrate, 8*mb, 8*mb, floor) {
		t.Fatal("below the floor is never enough, however fast the swarm")
	}

	// a 4K remux at 85 Mbit/s: 20 s of video is 212 MB, and the swarm has to beat the bitrate twice over
	uhd := 85.0 * mb / 8 // 10.6 MB/s
	if hlPreloadEnough(uhd, 30*mb, 100*mb, floor) {
		t.Fatal("100 MB is under 20 s of a 4K remux")
	}
	if !hlPreloadEnough(uhd, 30*mb, 250*mb, floor) {
		t.Fatal("250 MB with a swarm three times the bitrate must be enough")
	}
	// the swarm barely keeps up: buffer as configured, this is where it earns its keep
	if hlPreloadEnough(uhd, 12*mb, 500*mb, floor) {
		t.Fatal("a swarm barely above the bitrate must not cut the preload short")
	}
	if hlPreloadEnough(uhd, 5*mb, 900*mb, floor) {
		t.Fatal("a swarm below the bitrate must not cut the preload short")
	}

	// nothing to reason from: keep upstream behaviour
	if hlPreloadEnough(0, 50*mb, 900*mb, floor) {
		t.Fatal("unknown bitrate must keep the configured preload")
	}
	if hlPreloadEnough(bitrate, 0, 900*mb, floor) {
		t.Fatal("unknown speed must keep the configured preload")
	}
}

func TestHomelabBitrate(t *testing.T) {
	// duration wins when it is known: bytes of the file over its seconds
	if got := hlBitrate(&Torrent{DurationSeconds: 100}, nil); got != 0 {
		t.Fatalf("no file, no bitrate: %v", got)
	}
	// ffprobe reports bits per second as a string
	if got := hlBitrate(&Torrent{BitRate: "8000000"}, nil); got != 1e6 {
		t.Fatalf("8 Mbit/s must be 1 MB/s, got %v", got)
	}
	if got := hlBitrate(&Torrent{BitRate: "N/A"}, nil); got != 0 {
		t.Fatalf("unparsable bitrate must be 0, got %v", got)
	}
	if got := hlBitrate(nil, nil); got != 0 {
		t.Fatalf("no torrent: %v", got)
	}
}
