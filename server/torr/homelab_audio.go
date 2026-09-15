package torr

// homelab: one audio track for players (pxlvoid/TorrServer fork, see HOMELAB.md).
//
// The Lampa player (HTML5 video: Chromium / FFmpeg on Android TV) plays the first or the default audio track
// of an MKV and can not switch. For a torrent tracks can be chosen in order of preference (a series may have
// LostFilm in the first episodes and HDRezka in the rest): each episode serves the first of them it has, or a
// track in the same language, and the stream hides the other audio tracks:
// in the MKV header their TrackType is rewritten to "control" (0x20), which demuxers skip — FFmpeg ignores
// the blocks of such tracks — and FlagDefault is moved to the chosen track where the element exists.
// Only bytes of existing values change, never the size of anything: offsets, Cues and range requests stay as
// they are, so seeking works as before. Hooked into Torrent.Stream (the file is served through hlStreamReader,
// homelab_streams.go).

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/anacrolix/missinggo/v2/httptoo"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/log"
	"server/settings"
)

const (
	mkvEBML         = 0x1A45DFA3
	mkvSegment      = 0x18538067
	mkvSeekHead     = 0x114D9B74
	mkvSeek         = 0x4DBB
	mkvSeekID       = 0x53AB
	mkvSeekPosition = 0x53AC
	mkvTracks       = 0x1654AE6B
	mkvTrackEntry   = 0xAE
	mkvTrackNumber  = 0xD7
	mkvTrackType    = 0x83
	mkvFlagDefault  = 0x88
	mkvName         = 0x536E
	mkvLanguage     = 0x22B59C
	mkvLanguageBCP  = 0x22B59D
	mkvCodecID      = 0x86
	mkvCluster      = 0x1F43B675
	mkvInfo         = 0x1549A966
	mkvTimecodeScal = 0x2AD7B1
	mkvDuration     = 0x4489

	mkvTypeAudio   = 2
	mkvTypeControl = 0x20 // not audio, video or subtitles: players skip the track

	hlHeadWindow  = 256 << 10 // the header is read in windows of this size…
	hlHeadLimit   = 16 << 20  // …and Tracks is looked for no further than this
	hlHeadTimeout = 30 * time.Second
)

func metainfoHash(hash string) metainfo.Hash {
	if !hlDlValidHash(hash) {
		return metainfo.Hash{}
	}
	return metainfo.NewHashFromHex(hash)
}

// HomelabAudioTrack — an audio track of an MKV file.
type HomelabAudioTrack struct {
	Index   int    `json:"index"` // among the audio tracks, from 0
	Number  int    `json:"number"`
	Name    string `json:"name"`
	Lang    string `json:"lang"`
	Codec   string `json:"codec"`
	Default bool   `json:"default"`

	typeOff, defOff int64 // value offsets in the file
	typeLen, defLen int   // defLen 0 — the file has no FlagDefault element (default is 1)
}

type hlMkvHeader struct {
	tracks   []HomelabAudioTrack
	duration float64 // seconds, 0 — unknown (who is watching shows the time with it, homelab_streams.go)
}

// hlPatch — bytes replacing the file content at off.
type hlPatch struct {
	off  int64
	data []byte
}

var hlHeaders sync.Map // hash/path → *hlMkvHeader (nil header: not an MKV or no Tracks found)

func hlIsMkv(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".mkv") || strings.HasSuffix(p, ".webm")
}

// hlAudioPatches — with an audio track chosen for the torrent, the patches of this MKV that hide the other audio
// tracks (the ETag of the response changes with them); nil otherwise. Leaves r at the start of the file.
func hlAudioPatches(t *Torrent, file *torrent.File, r io.ReadSeeker, resp http.ResponseWriter) []hlPatch {
	choice, ok := settings.GetHomelabAudio(t.Hash().HexString())
	if !ok || !hlIsMkv(file.Path()) {
		return nil
	}
	hdr := hlHeaderOf(t.Hash().HexString()+"/"+file.Path(), r)
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	patches, chosen := hdr.patches(choice, file.Path())
	if len(patches) == 0 {
		return nil
	}
	// another content than the plain file: its own ETag, so nothing mixes them up in a cache
	if etag := resp.Header().Get("ETag"); etag != "" {
		resp.Header().Set("ETag", httptoo.EncodeQuotedString(fmt.Sprintf("%s-a%d", strings.Trim(etag, `"`), chosen)))
	}
	return patches
}

// HomelabAudioFile — a video MKV of the torrent: its audio tracks and which of them the choice serves.
type HomelabAudioFile struct {
	Id     int                 `json:"id"`
	Path   string              `json:"path"`
	Tracks []HomelabAudioTrack `json:"tracks"`
	Picked int                 `json:"picked"`        // index in Tracks, -1 — the file as is
	How    string              `json:"how,omitempty"` // "file" — its own track, preference N ("1", "2"…), "lang"
	Ready  bool                `json:"ready"`         // false — the start of the file is not downloaded yet
}

// HomelabAudioTracks — the audio tracks of every video MKV of a loaded torrent and what the choice serves in each.
// Headers are read from the start of the files (a few hundred KB each), a few at a time.
func HomelabAudioTracks(hash string) ([]HomelabAudioFile, error) {
	hash = strings.ToLower(hash)
	tt := hlDlLoaded(hash)
	t := bts.GetTorrent(metainfoHash(hash))
	if tt == nil || t == nil {
		return nil, errors.New("torrent is not loaded")
	}
	choice, _ := settings.GetHomelabAudio(hash)
	var sel []hlDlFile
	for _, f := range hlDlSelect(tt, nil) {
		if hlIsMkv(f.Path()) {
			sel = append(sel, f)
		}
	}
	out := make([]HomelabAudioFile, len(sel))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 3)
	for i, f := range sel {
		out[i] = HomelabAudioFile{Id: f.id, Path: f.Path(), Tracks: []HomelabAudioTrack{}, Picked: -1}
		wg.Add(1)
		go func(i int, f hlDlFile) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			hdr, ok := hlHeaderWait(t, f.File, hash+"/"+f.Path())
			if !ok {
				return
			}
			out[i].Ready = true
			out[i].Tracks = hdr.list()
			out[i].Picked, out[i].How = hdr.pick(choice, f.Path())
		}(i, f)
	}
	wg.Wait()
	return out, nil
}

// hlHeaderWait — the header of a file, reading it through a torrent reader if it is not known yet;
// ok=false if the torrent can not deliver the start of the file in time.
func hlHeaderWait(t *Torrent, file *torrent.File, key string) (*hlMkvHeader, bool) {
	if v, ok := hlHeaders.Load(key); ok {
		return v.(*hlMkvHeader), true
	}
	r := t.NewReader(file)
	if r == nil {
		return nil, false
	}
	defer t.CloseReader(r) // also ends a read still waiting for data
	done := make(chan struct{})
	go func() {
		hlHeaderOf(key, r)
		close(done)
	}()
	select {
	case <-done:
		v, ok := hlHeaders.Load(key) // not stored — the read failed, try again later
		if !ok {
			return nil, false
		}
		return v.(*hlMkvHeader), true
	case <-time.After(hlHeadTimeout):
		return nil, false
	}
}

func hlHeaderOf(key string, r io.ReadSeeker) *hlMkvHeader {
	if v, ok := hlHeaders.Load(key); ok {
		return v.(*hlMkvHeader)
	}
	hdr, err := hlParseMkv(r)
	if err != nil {
		log.TLogln("homelab: audio: mkv header of", key, err)
		hdr = &hlMkvHeader{}
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errHlMkvRead) {
			return hdr // the torrent could not deliver the start yet: try again next time
		}
	}
	hlHeaders.Store(key, hdr)
	return hdr
}

func (h *hlMkvHeader) list() []HomelabAudioTrack {
	if h == nil {
		return []HomelabAudioTrack{}
	}
	return append([]HomelabAudioTrack{}, h.tracks...)
}

// hlNorm — a track name for comparing: "LostFilm.TV" and "lostfilm tv" are the same dub.
func hlNorm(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hlSameName(a, b string) bool {
	na, nb := hlNorm(a), hlNorm(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	// one contains the other: "LostFilm" in "LostFilm.TV 18+", but not a stray "a" in everything
	return len(na) >= 4 && len(nb) >= 4 && (strings.Contains(na, nb) || strings.Contains(nb, na))
}

// pick — the track the choice serves in this file (path in the torrent): the track chosen for this episode,
// otherwise the first preference the file has (by name, the language breaks ties), otherwise a track in the
// language of the first preference (the file's default among them first); -1 — none: the file is served as is.
// how — "file" for the episode's own track, "1", "2"… for a preference, "lang" for the language.
func (h *hlMkvHeader) pick(c settings.HomelabAudioChoice, path string) (int, string) {
	if h == nil || len(h.tracks) == 0 {
		return -1, ""
	}
	if own, ok := c.Files[path]; ok {
		// the position it was chosen at, if the name still agrees; otherwise by the name
		if own.Index >= 0 && own.Index < len(h.tracks) && (own.Name == "" || h.tracks[own.Index].Name == own.Name) {
			return own.Index, "file"
		}
		for i, a := range h.tracks {
			if own.Name != "" && a.Name == own.Name {
				return i, "file"
			}
		}
	}
	if len(c.Tracks) == 0 {
		return -1, ""
	}
	for n, pref := range c.Tracks {
		match := -1
		for i, a := range h.tracks {
			if !hlSameName(a.Name, pref.Name) {
				continue
			}
			if match < 0 || strings.EqualFold(a.Lang, pref.Lang) && !strings.EqualFold(h.tracks[match].Lang, pref.Lang) {
				match = i
			}
		}
		if match >= 0 {
			return match, strconv.Itoa(n + 1)
		}
	}
	lang := c.Tracks[0].Lang
	match := -1
	for i, a := range h.tracks {
		if lang != "" && strings.EqualFold(a.Lang, lang) && (match < 0 || a.Default && !h.tracks[match].Default) {
			match = i
		}
	}
	if match >= 0 {
		return match, "lang"
	}
	return -1, ""
}

// patches hiding every audio track but the chosen one; chosen — its track number.
func (h *hlMkvHeader) patches(c settings.HomelabAudioChoice, path string) (patches []hlPatch, chosen int) {
	keep, _ := h.pick(c, path)
	if keep < 0 || len(h.tracks) < 2 {
		return nil, 0
	}
	for i, a := range h.tracks {
		if i == keep {
			if a.defLen > 0 {
				patches = append(patches, hlPatch{a.defOff, hlUint(1, a.defLen)})
			}
			continue
		}
		patches = append(patches, hlPatch{a.typeOff, hlUint(mkvTypeControl, a.typeLen)})
		if a.defLen > 0 {
			patches = append(patches, hlPatch{a.defOff, hlUint(0, a.defLen)})
		}
	}
	return patches, h.tracks[keep].Number
}

func hlUint(v uint64, n int) []byte {
	buf := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		buf[i] = byte(v)
		v >>= 8
	}
	return buf
}

// hlPatchReader serves the file with the patched bytes and counts what the client gets (homelab_streams.go).
type hlPatchReader struct {
	io.ReadSeeker
	off     int64
	patches []hlPatch
	stream  *hlStream // nil — not counted
}

func (p *hlPatchReader) Seek(offset int64, whence int) (int64, error) {
	n, err := p.ReadSeeker.Seek(offset, whence)
	if err == nil {
		p.off = n
	}
	return n, err
}

func (p *hlPatchReader) Read(b []byte) (int, error) {
	n, err := p.ReadSeeker.Read(b)
	if n > 0 {
		hlApply(b[:n], p.off, p.patches)
		if p.stream != nil {
			p.stream.read(p.off, n)
		}
		p.off += int64(n)
	}
	return n, err
}

// hlApply writes the patches that fall into buf, which holds the file from off.
func hlApply(buf []byte, off int64, patches []hlPatch) {
	end := off + int64(len(buf))
	for _, pt := range patches {
		for i, v := range pt.data {
			if at := pt.off + int64(i); at >= off && at < end {
				buf[at-off] = v
			}
		}
	}
}

// ---------- MKV (EBML) header ----------

var errHlMkvRead = errors.New("mkv: read")

// hlSource reads the file in windows: the header and Tracks are near the start, but SeekHead may point further.
type hlSource struct {
	r        io.ReadSeeker
	base     int64
	buf      []byte
	fileSize int64
}

func (s *hlSource) at(off int64, n int) ([]byte, error) {
	if off < 0 || off+int64(n) > hlHeadLimit {
		return nil, errors.New("mkv: header too far")
	}
	if off >= s.base && off+int64(n) <= s.base+int64(len(s.buf)) {
		return s.buf[off-s.base : off-s.base+int64(n)], nil
	}
	size := max(n, hlHeadWindow)
	if _, err := s.r.Seek(off, io.SeekStart); err != nil {
		return nil, errHlMkvRead
	}
	buf := make([]byte, size)
	got, err := io.ReadFull(s.r, buf)
	if got < n {
		if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	s.base, s.buf = off, buf[:got]
	return s.buf[:n], nil
}

// vint — an EBML variable size integer at off; keepMarker for element IDs.
func (s *hlSource) vint(off int64, keepMarker bool) (v uint64, n int, err error) {
	first, err := s.at(off, 1)
	if err != nil {
		return 0, 0, err
	}
	n = 1
	for mask := byte(0x80); n <= 8 && first[0]&mask == 0; mask >>= 1 {
		n++
	}
	if n > 8 {
		return 0, 0, errors.New("mkv: bad vint")
	}
	b, err := s.at(off, n)
	if err != nil {
		return 0, 0, err
	}
	v = uint64(b[0])
	if !keepMarker {
		v &= uint64(0xFF >> n)
	}
	for _, c := range b[1:] {
		v = v<<8 | uint64(c)
	}
	return v, n, nil
}

type hlElem struct {
	id      uint64
	data    int64 // offset of the value
	size    int64 // -1 — unknown size
	next    int64 // offset after the element
	unknown bool
}

func (s *hlSource) elem(off int64) (hlElem, error) {
	id, idLen, err := s.vint(off, true)
	if err != nil {
		return hlElem{}, err
	}
	size, sizeLen, err := s.vint(off+int64(idLen), false)
	if err != nil {
		return hlElem{}, err
	}
	e := hlElem{id: id, data: off + int64(idLen+sizeLen), size: int64(size)}
	if size == 1<<(7*uint(sizeLen))-1 { // all ones: unknown size
		e.size, e.unknown = -1, true
	}
	e.next = e.data + e.size
	return e, nil
}

// duration — seconds from an Info element: Duration (a float in TimecodeScale units, 1 ms by default).
func (s *hlSource) duration(info hlElem) float64 {
	scale, dur := uint64(1000000), 0.0
	for off := info.data; off < info.next; {
		e, err := s.elem(off)
		if err != nil || e.unknown {
			break
		}
		switch e.id {
		case mkvTimecodeScal:
			if v, err := s.uintAt(e); err == nil && v > 0 {
				scale = v
			}
		case mkvDuration:
			if b, err := s.at(e.data, int(e.size)); err == nil {
				switch e.size {
				case 4:
					dur = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
				case 8:
					dur = math.Float64frombits(binary.BigEndian.Uint64(b))
				}
			}
		}
		off = e.next
	}
	if dur <= 0 || math.IsNaN(dur) || math.IsInf(dur, 0) {
		return 0
	}
	return dur * float64(scale) / 1e9
}

func (s *hlSource) uintAt(e hlElem) (uint64, error) {
	if e.size < 1 || e.size > 8 {
		return 0, errors.New("mkv: bad uint")
	}
	b, err := s.at(e.data, int(e.size))
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, nil
}

func (s *hlSource) strAt(e hlElem) string {
	if e.size <= 0 || e.size > 4096 {
		return ""
	}
	b, err := s.at(e.data, int(e.size))
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\x00")
}

func hlParseMkv(r io.ReadSeeker) (*hlMkvHeader, error) {
	s := &hlSource{r: r}
	ebml, err := s.elem(0)
	if err != nil {
		return nil, err
	}
	if ebml.id != mkvEBML || ebml.unknown {
		return nil, errors.New("mkv: no EBML header")
	}
	seg, err := s.elem(ebml.next)
	if err != nil {
		return nil, err
	}
	if seg.id != mkvSegment {
		return nil, errors.New("mkv: no Segment")
	}

	// top level: Tracks itself, or where SeekHead says it is; stop at the first Cluster. Info (the duration) on the way.
	tracks, info := int64(-1), int64(-1)
	duration := 0.0
	for off := seg.data; off < hlHeadLimit; {
		e, err := s.elem(off)
		if err != nil {
			return nil, err
		}
		if e.id == mkvInfo && !e.unknown {
			duration = s.duration(e)
		}
		if e.id == mkvTracks {
			tracks = off
			break
		}
		if e.id == mkvSeekHead && !e.unknown {
			if pos, ok := s.seekTo(e, mkvTracks); ok && tracks < 0 {
				tracks = seg.data + pos
			}
			if pos, ok := s.seekTo(e, mkvInfo); ok && info < 0 {
				info = seg.data + pos
			}
		}
		if e.id == mkvCluster || e.unknown {
			break
		}
		off = e.next
	}
	if tracks < 0 {
		return nil, errors.New("mkv: no Tracks")
	}
	te, err := s.elem(tracks)
	if err != nil {
		return nil, err
	}
	if te.id != mkvTracks || te.unknown {
		return nil, errors.New("mkv: bad Tracks")
	}

	if duration == 0 && info >= 0 { // Info after Tracks: where SeekHead says
		if e, err := s.elem(info); err == nil && e.id == mkvInfo && !e.unknown {
			duration = s.duration(e)
		}
	}

	hdr := &hlMkvHeader{duration: duration}
	for off := te.data; off < te.next; {
		entry, err := s.elem(off)
		if err != nil {
			return nil, err
		}
		if entry.unknown {
			break
		}
		if entry.id == mkvTrackEntry {
			if a, ok, err := s.track(entry); err != nil {
				return nil, err
			} else if ok {
				a.Index = len(hdr.tracks)
				hdr.tracks = append(hdr.tracks, a)
			}
		}
		off = entry.next
	}
	return hdr, nil
}

// seekTo — SeekPosition of the element id in a SeekHead (relative to the Segment data).
func (s *hlSource) seekTo(head hlElem, id uint64) (int64, bool) {
	for off := head.data; off < head.next; {
		seek, err := s.elem(off)
		if err != nil || seek.unknown {
			return 0, false
		}
		if seek.id == mkvSeek {
			var sid uint64
			pos := int64(-1)
			for c := seek.data; c < seek.next; {
				e, err := s.elem(c)
				if err != nil || e.unknown {
					return 0, false
				}
				switch e.id {
				case mkvSeekID:
					if b, err := s.at(e.data, int(min(e.size, 8))); err == nil {
						for _, x := range b {
							sid = sid<<8 | uint64(x)
						}
					}
				case mkvSeekPosition:
					if v, err := s.uintAt(e); err == nil {
						pos = int64(v)
					}
				}
				c = e.next
			}
			if sid == id && pos >= 0 {
				return pos, true
			}
		}
		off = seek.next
	}
	return 0, false
}

// track — an audio TrackEntry; ok=false for other tracks.
func (s *hlSource) track(entry hlElem) (a HomelabAudioTrack, ok bool, err error) {
	a.Default = true
	isAudio := false
	lang, bcp := "eng", "" // eng — the Matroska default
	for off := entry.data; off < entry.next; {
		e, err := s.elem(off)
		if err != nil {
			return a, false, err
		}
		if e.unknown {
			break
		}
		switch e.id {
		case mkvTrackNumber:
			v, _ := s.uintAt(e)
			a.Number = int(v)
		case mkvTrackType:
			v, err := s.uintAt(e)
			if err != nil {
				return a, false, nil
			}
			isAudio = v == mkvTypeAudio
			a.typeOff, a.typeLen = e.data, int(e.size)
		case mkvFlagDefault:
			v, err := s.uintAt(e)
			if err == nil {
				a.Default = v != 0
				a.defOff, a.defLen = e.data, int(e.size)
			}
		case mkvName:
			a.Name = s.strAt(e)
		case mkvLanguage:
			lang = s.strAt(e)
		case mkvLanguageBCP:
			bcp = s.strAt(e) // wins over the old Language element
		case mkvCodecID:
			a.Codec = strings.TrimPrefix(s.strAt(e), "A_")
		}
		off = e.next
	}
	a.Lang = lang
	if bcp != "" {
		a.Lang = bcp
	}
	return a, isAudio && a.typeLen > 0, nil
}
