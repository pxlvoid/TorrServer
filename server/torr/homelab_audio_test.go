package torr

// homelab: tests of one audio track for players (homelab_audio.go) on a synthetic MKV header.

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"server/settings"
)

// ebml element: id bytes as written, size as an 8-byte vint (like mkvmerge for big elements) or 1 byte
func ebml(id []byte, data ...[]byte) []byte {
	body := bytes.Join(data, nil)
	out := append([]byte{}, id...)
	if len(body) < 127 {
		out = append(out, 0x80|byte(len(body)))
	} else {
		size := uint64(len(body))
		out = append(out, 0x01, byte(size>>48), byte(size>>40), byte(size>>32), byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	}
	return append(out, body...)
}

func ebmlUint(id []byte, v byte) []byte { return ebml(id, []byte{v}) }

var (
	idEBML    = []byte{0x1A, 0x45, 0xDF, 0xA3}
	idSegment = []byte{0x18, 0x53, 0x80, 0x67}
	idInfo    = []byte{0x15, 0x49, 0xA9, 0x66}
	idTracks  = []byte{0x16, 0x54, 0xAE, 0x6B}
	idEntry   = []byte{0xAE}
	idNumber  = []byte{0xD7}
	idType    = []byte{0x83}
	idDefault = []byte{0x88}
	idName    = []byte{0x53, 0x6E}
	idLang    = []byte{0x22, 0xB5, 0x9C}
	idCodec   = []byte{0x86}
	idCluster = []byte{0x1F, 0x43, 0xB6, 0x75}
)

// Info with TimecodeScale 1 ms and Duration 6 000 000 ms as a float64: 100 minutes
func testInfo() []byte {
	d := make([]byte, 8)
	binary.BigEndian.PutUint64(d, math.Float64bits(6000000))
	return ebml(idInfo, ebml([]byte{0x2A, 0xD7, 0xB1}, []byte{0x0F, 0x42, 0x40}), ebml([]byte{0x44, 0x89}, d))
}

func testMkv() []byte {
	tracks := ebml(idTracks,
		ebml(idEntry, ebmlUint(idNumber, 1), ebmlUint(idType, 1), ebml(idCodec, []byte("V_MPEG4/ISO/AVC"))),
		ebml(idEntry, ebmlUint(idNumber, 2), ebmlUint(idType, 2), ebml(idCodec, []byte("A_AC3")),
			ebml(idName, []byte("DVO - Кубик в Кубе")), ebml(idLang, []byte("rus"))),
		// no FlagDefault element: default by the spec
		ebml(idEntry, ebmlUint(idNumber, 3), ebmlUint(idType, 2), ebml(idCodec, []byte("A_AC3")),
			ebmlUint(idDefault, 0), ebml(idName, []byte("Original"))),
		ebml(idEntry, ebmlUint(idNumber, 4), ebmlUint(idType, 0x11), ebml(idCodec, []byte("S_TEXT/UTF8"))),
	)
	segment := ebml(idSegment, testInfo(), tracks, ebml(idCluster, make([]byte, 200)))
	return append(ebml(idEBML, []byte{0x42, 0x86, 0x81, 0x01}), segment...)
}

func TestHomelabMkvTracks(t *testing.T) {
	hdr, err := hlParseMkv(bytes.NewReader(testMkv()))
	if err != nil {
		t.Fatal(err)
	}
	if len(hdr.tracks) != 2 {
		t.Fatalf("audio tracks: got %d, want 2", len(hdr.tracks))
	}
	if hdr.duration != 6000 {
		t.Fatalf("duration: %v, want 6000 s", hdr.duration)
	}
	a, b := hdr.tracks[0], hdr.tracks[1]
	if a.Number != 2 || a.Name != "DVO - Кубик в Кубе" || a.Lang != "rus" || a.Codec != "AC3" || !a.Default || a.defLen != 0 {
		t.Fatalf("first track: %+v", a)
	}
	if b.Number != 3 || b.Name != "Original" || b.Lang != "eng" || b.Default || b.defLen != 1 || b.Index != 1 {
		t.Fatalf("second track: %+v", b)
	}
}

// Choosing "Original": the other audio track becomes a control track, FlagDefault of the chosen is set;
// the stream keeps its size and everything else byte for byte.
func TestHomelabMkvPatchedStream(t *testing.T) {
	file := testMkv()
	hdr, err := hlParseMkv(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	patches, chosen := hdr.patches(settings.HomelabAudioChoice{Tracks: []settings.HomelabAudioPref{{Name: "Original", Lang: "eng"}}}, "e1.mkv")
	if chosen != 3 || len(patches) != 2 {
		t.Fatalf("chosen %d, patches %+v", chosen, patches)
	}

	// read in odd chunks after seeks, as http.ServeContent does
	r := &hlPatchReader{ReadSeeker: bytes.NewReader(file), patches: patches}
	if size, _ := r.Seek(0, io.SeekEnd); size != int64(len(file)) {
		t.Fatalf("size: %d", size)
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var out []byte
	buf := make([]byte, 7)
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err == io.EOF {
			break
		}
	}
	if len(out) != len(file) {
		t.Fatalf("patched length %d, file %d", len(out), len(file))
	}
	diff := 0
	for i := range out {
		if out[i] != file[i] {
			diff++
		}
	}
	if diff != 2 {
		t.Fatalf("changed bytes: %d, want 2 (TrackType of one track, FlagDefault of the other)", diff)
	}

	patched, err := hlParseMkv(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(patched.tracks) != 1 || patched.tracks[0].Name != "Original" || !patched.tracks[0].Default {
		t.Fatalf("after the patch only Original must be audio and default: %+v", patched.tracks)
	}
}

// an MKV header with a video track and these audio tracks ({name, language}), in this order
func testMkvAudio(audio ...[2]string) *hlMkvHeader {
	entries := [][]byte{ebml(idEntry, ebmlUint(idNumber, 1), ebmlUint(idType, 1))}
	for i, a := range audio {
		entries = append(entries, ebml(idEntry, ebmlUint(idNumber, byte(i+2)), ebmlUint(idType, 2),
			ebml(idName, []byte(a[0])), ebml(idLang, []byte(a[1]))))
	}
	segment := ebml(idSegment, ebml(idTracks, entries...), ebml(idCluster, make([]byte, 10)))
	hdr, err := hlParseMkv(bytes.NewReader(append(ebml(idEBML, []byte{0x42, 0x86, 0x81, 0x01}), segment...)))
	if err != nil {
		panic(err)
	}
	return hdr
}

func prefs(p ...[2]string) settings.HomelabAudioChoice {
	c := settings.HomelabAudioChoice{}
	for _, x := range p {
		c.Tracks = append(c.Tracks, settings.HomelabAudioPref{Name: x[0], Lang: x[1]})
	}
	return c
}

func TestHomelabMkvPick(t *testing.T) {
	hdr, _ := hlParseMkv(bytes.NewReader(testMkv())) // DVO - Кубик в Кубе (rus), Original (eng)
	cases := []struct {
		choice settings.HomelabAudioChoice
		want   int
		how    string
	}{
		{prefs([2]string{"Original", "eng"}), 1, "1"},
		{prefs([2]string{"LostFilm", "rus"}, [2]string{"Кубик в Кубе", "rus"}), 0, "2"}, // the second preference, by a part of the name
		{prefs([2]string{"LostFilm", "rus"}), 0, "lang"},                                // none by name: the same language
		{prefs([2]string{"LostFilm", "ukr"}), -1, ""},                                   // nothing fits: the file as is
		{settings.HomelabAudioChoice{}, -1, ""},
	}
	for _, c := range cases {
		if got, how := hdr.pick(c.choice, "e1.mkv"); got != c.want || how != c.how {
			t.Fatalf("%+v: got %d %q, want %d %q", c.choice, got, how, c.want, c.how)
		}
	}
	if _, err := hlParseMkv(bytes.NewReader([]byte("not an mkv at all"))); err == nil {
		t.Fatal("garbage must not parse")
	}
}

// A season: LostFilm dubbed episodes 1-3 only, HDRezka the rest. "LostFilm, else HDRezka" — and even with only
// "LostFilm" chosen an episode without it gets the Russian dub, never the original that comes first in the file.
func TestHomelabMkvPickSeries(t *testing.T) {
	early := testMkvAudio([2]string{"LostFilm.TV", "rus"}, [2]string{"HDrezka Studio", "rus"}, [2]string{"Original", "eng"})
	late := testMkvAudio([2]string{"Original", "eng"}, [2]string{"HDrezka Studio", "rus"})

	both := prefs([2]string{"LostFilm", "rus"}, [2]string{"HDrezka Studio", "rus"})
	if got, how := early.pick(both, "e1.mkv"); got != 0 || how != "1" {
		t.Fatalf("early episode: got %d %q", got, how)
	}
	if got, how := late.pick(both, "e4.mkv"); got != 1 || how != "2" {
		t.Fatalf("late episode: got %d %q, want HDrezka by the second preference", got, how)
	}
	if got, how := late.pick(prefs([2]string{"LostFilm", "rus"}), "e4.mkv"); got != 1 || how != "lang" {
		t.Fatalf("late episode, only LostFilm chosen: got %d %q, want the Russian HDrezka", got, how)
	}
}

// A track chosen for one episode wins over the torrent's list there, and only there.
func TestHomelabMkvPickOwnTrack(t *testing.T) {
	ep := testMkvAudio([2]string{"LostFilm.TV", "rus"}, [2]string{"HDrezka Studio", "rus"}, [2]string{"Original", "eng"})
	choice := prefs([2]string{"LostFilm", "rus"})
	choice.Files = map[string]settings.HomelabAudioPref{"s/e2.mkv": {Name: "Original", Lang: "eng", Index: 2}}

	if got, how := ep.pick(choice, "s/e2.mkv"); got != 2 || how != "file" {
		t.Fatalf("own track of the episode: got %d %q", got, how)
	}
	if got, how := ep.pick(choice, "s/e3.mkv"); got != 0 || how != "1" {
		t.Fatalf("other episodes keep the torrent's choice: got %d %q", got, how)
	}
	// the torrent's list empty: only that episode is changed
	choice.Tracks = nil
	if got, _ := ep.pick(choice, "s/e3.mkv"); got != -1 {
		t.Fatalf("no list, no own track: the file as is, got %d", got)
	}
	// the file changed (another release under the same name): the position no longer has that name — by the name
	choice.Files["s/e2.mkv"] = settings.HomelabAudioPref{Name: "HDrezka Studio", Index: 0}
	if got, how := ep.pick(choice, "s/e2.mkv"); got != 1 || how != "file" {
		t.Fatalf("own track by the name: got %d %q", got, how)
	}
}

func TestHomelabSameName(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"LostFilm", "LostFilm.TV", true},
		{"lostfilm tv", "LostFilm.TV 18+", true},
		{"HDrezka Studio", "HDRezka", true},
		{"Original", "Кубик в Кубе", false},
		{"a", "abc", false}, // too short to be a part of a name
		{"", "", false},
	} {
		if got := hlSameName(c.a, c.b); got != c.want {
			t.Fatalf("%q vs %q: got %v", c.a, c.b, got)
		}
	}
}
