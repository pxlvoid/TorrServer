package torr

// homelab: "download to disk" — the whole torrent or chosen files into the persistent disk cache
// (pxlvoid/TorrServer fork, see HOMELAB.md).
//
// Preload fetches only a buffer before playback. A download fetches files completely: it holds a torrstor
// reader on the file and moves it to the first piece that is not complete yet, so upstream piece priorities
// — the same as for a player — fetch what is ahead of it (with background fill the window reaches the end
// of the file). The reader is marked as a download, so the torrent is not shown as "playing".
// A job pins the torrent: the janitor keeps what is done. Stopping a job leaves the pin as it was before.
// Jobs run one at a time in the order added; the queue is stored in the DB and survives a restart.

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/log"
	"server/settings"
	"server/torr/state"
	"server/torr/storage/torrstor"
	utils2 "server/utils"
	"server/version"
)

const (
	hlDlTick      = time.Second
	hlDlReseek    = 20 * time.Second // keep the reader fresh: an idle reader is switched off after 60 s
	hlDlStall     = 10 * time.Minute // no progress this long — let the next job in the queue run
	hlDlRetry     = 5 * time.Minute  // a failed job is tried again
	hlDlDiskFloor = torrstor.HomelabMinFree / 2
)

// Error codes of a job, translated by the web UI.
const (
	HomelabDlOff      = "off"       // the persistent cache is off
	HomelabDlNotFound = "not_found" // the torrent is not in the TorrServer list
	HomelabDlNoInfo   = "no_info"   // no metadata: nobody seeds it
	HomelabDlNoFiles  = "no_files"  // the chosen files are not in the torrent
	HomelabDlLimit    = "limit"     // does not fit into the cache limit next to what is pinned
	HomelabDlDisk     = "disk"      // does not fit on the disk
	HomelabDlDiskFull = "disk_full" // the disk filled up while downloading
	HomelabDlFailed   = "failed"
)

// HomelabDlError — why a job can not run; Need / Avail are set for limit and disk.
type HomelabDlError struct {
	Code  string
	Need  int64
	Avail int64
}

func (e *HomelabDlError) Error() string { return "download: " + e.Code }

var (
	errHlReopen = errors.New("torrent closed, open it again")
	errHlRotate = errors.New("no progress, next job")
	errHlPaused = errors.New("persistent cache is off")
)

// HomelabDownload — a job for the API.
type HomelabDownload struct {
	Hash  string  `json:"hash"`
	Files []int   `json:"files,omitempty"` // empty — the whole torrent
	Added int64   `json:"added"`
	State string  `json:"state"` // queued | active | error
	Error string  `json:"error,omitempty"`
	Need  int64   `json:"need,omitempty"`
	Avail int64   `json:"avail,omitempty"`
	Done  int64   `json:"done"`  // bytes of the chosen files on disk; known while the torrent is loaded
	Total int64   `json:"total"` // 0 — not known yet
	Speed float64 `json:"speed"`
}

// HomelabFileState — a file of the torrent and how much of it is on disk.
type HomelabFileState struct {
	Id     int    `json:"id"`
	Path   string `json:"path"`
	Length int64  `json:"length"`
	Done   int64  `json:"done"`
}

type hlDlJob struct {
	settings.HomelabDownloadJob
	state   string
	err     *HomelabDlError
	retryAt time.Time
	done    int64
	total   int64
	speed   float64
	cancel  context.CancelFunc
	exited  chan struct{} // closed when the run of an active job returns
	stopped bool

	title, label string // for notifications: the torrent and what of it ("весь торрент", a file, "N файлов")
	notified     string // error code already notified: one notification per reason, not every retry
}

var (
	hlDlMu   sync.Mutex
	hlDlJobs []*hlDlJob
	hlDlOnce sync.Once
	hlDlKick = make(chan struct{}, 1)
)

// HomelabDownloadsStart loads the stored queue and starts the worker (once).
func HomelabDownloadsStart() {
	hlDlOnce.Do(func() {
		hlDlMu.Lock()
		for _, j := range settings.GetHomelabDownloads() {
			if hlDlValidHash(j.Hash) {
				hlDlJobs = append(hlDlJobs, &hlDlJob{HomelabDownloadJob: j, state: "queued"})
			}
		}
		hlDlMu.Unlock()
		go hlDlLoop()
		torrstor.HomelabNotify(torrstor.HomelabMessage{
			Event: torrstor.HomelabEvStarted, Title: "TorrServer запущен", Message: version.Version, Priority: 2,
			Tags: []string{"rocket"},
		})
	})
}

func hlDlWake() {
	select {
	case hlDlKick <- struct{}{}:
	default:
	}
}

func hlDlSaveLocked() {
	jobs := make([]settings.HomelabDownloadJob, 0, len(hlDlJobs))
	for _, j := range hlDlJobs {
		jobs = append(jobs, j.HomelabDownloadJob)
	}
	settings.SetHomelabDownloads(jobs)
}

func hlDlFindLocked(hash string) (int, *hlDlJob) {
	for i, j := range hlDlJobs {
		if j.Hash == hash {
			return i, j
		}
	}
	return -1, nil
}

func hlDlRemoveLocked(job *hlDlJob) {
	for i, j := range hlDlJobs {
		if j == job {
			hlDlJobs = append(hlDlJobs[:i], hlDlJobs[i+1:]...)
			return
		}
	}
}

// HomelabDownloadStart queues files of a torrent (none — the whole torrent); adds to a job already queued.
func HomelabDownloadStart(hash string, files []int) error {
	hash = strings.ToLower(hash)
	if !settings.HomelabPersistentCache() {
		return &HomelabDlError{Code: HomelabDlOff}
	}
	if !hlDlValidHash(hash) || bts == nil {
		return &HomelabDlError{Code: HomelabDlNotFound}
	}
	h := metainfo.NewHashFromHex(hash)
	t := bts.GetTorrent(h)
	if t == nil && GetTorrentDB(h) == nil {
		return &HomelabDlError{Code: HomelabDlNotFound}
	}
	HomelabDownloadsStart()

	// whether it fits is known only with the file list; otherwise the job checks it when it starts
	if tt := t.anacrolix(); tt != nil {
		sel := hlDlSelect(tt, files)
		if len(sel) == 0 {
			return &HomelabDlError{Code: HomelabDlNoFiles}
		}
		done, total := hlDlProgress(tt, sel, hlPieceComplete(tt))
		if err := hlDlCheckSpace(hash, total-done); err != nil {
			return err
		}
	}

	hlDlMu.Lock()
	defer hlDlMu.Unlock()
	_, job := hlDlFindLocked(hash)
	if job == nil {
		job = &hlDlJob{HomelabDownloadJob: settings.HomelabDownloadJob{
			Hash:      hash,
			Files:     append([]int(nil), files...),
			Added:     time.Now().Unix(),
			WasPinned: torrstor.HomelabPinned(hash),
		}, state: "queued"}
		hlDlJobs = append(hlDlJobs, job)
	} else {
		if len(files) == 0 || len(job.Files) == 0 {
			job.Files = nil
		} else {
			job.Files = hlUnion(job.Files, files)
		}
		if job.state == "error" {
			job.state, job.err, job.retryAt = "queued", nil, time.Time{}
		}
	}
	hlDlSaveLocked()
	hlDlWake()
	return nil
}

// HomelabDownloadStop removes files from a job (none — the whole job) and waits for its run to end.
func HomelabDownloadStop(hash string, files []int) {
	hash = strings.ToLower(hash)
	hlDlMu.Lock()
	_, job := hlDlFindLocked(hash)
	if job == nil {
		hlDlMu.Unlock()
		return
	}
	if len(files) > 0 {
		ids := job.Files
		if len(ids) == 0 { // the whole torrent: all its files but these
			if tt := hlDlLoaded(hash); tt != nil {
				for _, f := range hlDlSelect(tt, nil) {
					ids = append(ids, f.id)
				}
			}
		}
		if rest := hlMinus(ids, files); len(rest) > 0 {
			job.Files = rest
			hlDlSaveLocked()
			hlDlMu.Unlock()
			return
		}
	}
	hlDlRemoveLocked(job)
	job.stopped = true
	if job.cancel != nil {
		job.cancel()
	}
	exited, wasPinned := job.exited, job.WasPinned
	hlDlSaveLocked()
	hlDlMu.Unlock()

	if exited != nil {
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
		}
	}
	if !wasPinned {
		_ = torrstor.HomelabSetPinned(hash, false)
	}
}

// hlOnRemove — hook at the start of RemTorrent (one torrent or "Remove all"): its download stops before the torrent
// is closed (otherwise the job could open it again between the close and the removal from the DB), its audio
// choice is forgotten. The disk cache stays, as for every removal in persistent mode.
func hlOnRemove(hash string) {
	hash = strings.ToLower(hash)
	HomelabDownloadStop(hash, nil)
	if _, ok := settings.GetHomelabAudio(hash); ok {
		_ = settings.SetHomelabAudio(hash, nil)
	}
}

// HomelabDownloads — the queue, in order.
func HomelabDownloads() []HomelabDownload {
	hlDlMu.Lock()
	jobs := make([]HomelabDownload, 0, len(hlDlJobs))
	idle := map[int]*hlDlJob{}
	for i, j := range hlDlJobs {
		d := HomelabDownload{
			Hash:  j.Hash,
			Files: append([]int(nil), j.Files...),
			Added: j.Added,
			State: j.state,
			Done:  j.done,
			Total: j.total,
			Speed: j.speed,
		}
		if j.err != nil {
			d.Error, d.Need, d.Avail = j.err.Code, j.err.Need, j.err.Avail
		}
		if j.state != "active" {
			idle[i] = j
		}
		jobs = append(jobs, d)
	}
	hlDlMu.Unlock()

	// progress of a waiting job is known if its torrent is loaded
	for i, j := range idle {
		if tt := hlDlLoaded(j.Hash); tt != nil {
			jobs[i].Done, jobs[i].Total = hlDlProgress(tt, hlDlSelect(tt, jobs[i].Files), hlPieceComplete(tt))
		}
	}
	return jobs
}

// HomelabDownloadStatus — the job of a torrent (nil if none) and its files with what is on disk (for a torrent that
// is not loaded — from its record in the TorrServer list; empty if unknown).
func HomelabDownloadStatus(hash string) (*HomelabDownload, []HomelabFileState) {
	hash = strings.ToLower(hash)
	var job *HomelabDownload
	for _, d := range HomelabDownloads() {
		if d.Hash == hash {
			d := d
			job = &d
			break
		}
	}
	files := make([]HomelabFileState, 0)
	if tt := hlDlLoaded(hash); tt != nil {
		complete := hlPieceComplete(tt)
		pl := tt.Info().PieceLength
		for _, f := range hlDlSelect(tt, nil) {
			files = append(files, HomelabFileState{Id: f.id, Path: f.Path(), Length: f.Length(), Done: hlFileDone(f.File, complete, pl)})
		}
	}
	if len(files) == 0 { // not loaded: from its record in the TorrServer list and the pieces on disk
		if disk := HomelabDiskFiles(hash); disk != nil {
			files = disk
		}
	}
	return job, files
}

// HomelabCloseTorrent closes a loaded torrent (stops whoever reads it) and waits until its cache is closed.
func HomelabCloseTorrent(hash string) {
	if bts == nil || !hlDlValidHash(hash) {
		return
	}
	bts.RemoveTorrent(metainfo.NewHashFromHex(hash))
	for deadline := time.Now().Add(5 * time.Second); torrstor.HomelabIsOpen(hash) && time.Now().Before(deadline); {
		time.Sleep(100 * time.Millisecond)
	}
}

func hlDlValidHash(hash string) bool {
	if len(hash) != 40 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

// hlDlLoaded — the torrent if it is loaded with metadata.
func hlDlLoaded(hash string) *torrent.Torrent {
	if bts == nil || !hlDlValidHash(hash) {
		return nil
	}
	return bts.GetTorrent(metainfo.NewHashFromHex(hash)).anacrolix()
}

// anacrolix — the loaded torrent with metadata, nil otherwise (safe on a nil *Torrent).
func (t *Torrent) anacrolix() *torrent.Torrent {
	if t == nil || t.Stat == state.TorrentClosed || t.GetCache() == nil {
		return nil
	}
	tt := t.Torrent
	if tt == nil || tt.Info() == nil {
		return nil
	}
	return tt
}

// the worker

func hlDlNext() *hlDlJob {
	if bts == nil || !settings.HomelabPersistentCache() {
		return nil
	}
	hlDlMu.Lock()
	defer hlDlMu.Unlock()
	now := time.Now()
	for _, j := range hlDlJobs {
		if j.state == "queued" || j.state == "error" && now.After(j.retryAt) {
			return j
		}
	}
	return nil
}

func hlDlHasOtherLocked(job *hlDlJob) bool {
	now := time.Now()
	for _, j := range hlDlJobs {
		if j != job && (j.state == "queued" || j.state == "error" && now.After(j.retryAt)) {
			return true
		}
	}
	return false
}

func hlDlLoop() {
	for {
		job := hlDlNext()
		if job == nil {
			select {
			case <-hlDlKick:
			case <-time.After(30 * time.Second):
			}
			continue
		}

		ctx, cancel := context.WithCancel(context.Background())
		exited := make(chan struct{})
		hlDlMu.Lock()
		job.state, job.err, job.cancel, job.exited = "active", nil, cancel, exited
		hlDlMu.Unlock()

		err := hlDlRun(ctx, job)
		cancel()

		hlDlMu.Lock()
		job.cancel, job.speed = nil, 0
		var de *HomelabDlError
		switch {
		case job.stopped:
		case err == nil:
			hlDlRemoveLocked(job)
			hlDlSaveLocked()
			log.TLogln("homelab: download complete", job.Hash)
			torrstor.HomelabNotify(torrstor.HomelabMessage{
				Event:   torrstor.HomelabEvDownloadDone,
				Title:   "Скачано на диск: " + job.title,
				Message: fmt.Sprintf("%s, %s. Закреплено — уборщик не тронет.", job.label, hlGB(job.total)),
				Tags:    []string{"arrow_down"},
			})
		case errors.Is(err, errHlRotate):
			hlDlRemoveLocked(job)
			hlDlJobs = append(hlDlJobs, job)
			job.state = "queued"
		case errors.Is(err, errHlReopen), errors.Is(err, errHlPaused), errors.Is(err, context.Canceled):
			job.state = "queued"
		case errors.As(err, &de) && de.Code == HomelabDlNotFound:
			// the torrent is gone from the list: nothing to retry
			hlDlRemoveLocked(job)
			hlDlSaveLocked()
			log.TLogln("homelab: download dropped, torrent is not in the list", job.Hash)
		case errors.As(err, &de):
			job.state, job.err, job.retryAt = "error", de, time.Now().Add(hlDlRetry)
			log.TLogln("homelab: download", job.Hash, err)
			if job.notified != de.Code {
				job.notified = de.Code
				torrstor.HomelabNotify(torrstor.HomelabMessage{
					Event:    torrstor.HomelabEvDownloadError,
					Title:    "TorrServer: загрузка стоит — " + hlDlTitle(job),
					Message:  hlDlErrorText(de),
					Priority: 4,
					Tags:     []string{"warning"},
				})
			}
		default:
			job.state, job.err, job.retryAt = "error", &HomelabDlError{Code: HomelabDlFailed}, time.Now().Add(hlDlRetry)
			log.TLogln("homelab: download", job.Hash, err)
		}
		hlDlMu.Unlock()
		close(exited)

		if errors.Is(err, errHlReopen) {
			time.Sleep(3 * time.Second) // settings change or drop: the BT client needs a moment
		}
	}
}

// hlDlOpen loads the torrent (from the TorrServer list if it is not loaded) and waits for its metadata.
func hlDlOpen(hash string) (*Torrent, *torrent.Torrent, error) {
	h := metainfo.NewHashFromHex(hash)
	t := bts.GetTorrent(h)
	if t == nil {
		db := GetTorrentDB(h)
		if db == nil || db.TorrentSpec == nil {
			return nil, nil, &HomelabDlError{Code: HomelabDlNotFound}
		}
		nt, err := NewTorrent(db.TorrentSpec, bts)
		if err != nil {
			return nil, nil, errHlReopen
		}
		if nt.Title == "" {
			nt.Title, nt.Poster, nt.Data = db.Title, db.Poster, db.Data
			nt.Size, nt.Timestamp, nt.Category = db.Size, db.Timestamp, db.Category
		}
		t = nt
	}
	if tt := t.anacrolix(); tt != nil {
		return t, tt, nil
	}
	if !t.GotInfo() {
		return nil, nil, &HomelabDlError{Code: HomelabDlNoInfo}
	}
	tt := t.anacrolix()
	if tt == nil {
		return nil, nil, errHlReopen
	}
	return t, tt, nil
}

func hlDlRun(ctx context.Context, job *hlDlJob) error {
	t, tt, err := hlDlOpen(job.Hash)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	hlDlMu.Lock()
	ids := append([]int(nil), job.Files...)
	if !job.stopped {
		_ = torrstor.HomelabSetPinned(job.Hash, true) // under hlDlMu: Stop restores the pin after us, never before
	}
	hlDlMu.Unlock()

	pl := tt.Info().PieceLength
	sel := hlDlSelect(tt, ids)
	if len(sel) == 0 {
		return &HomelabDlError{Code: HomelabDlNoFiles}
	}
	hlDlMu.Lock()
	job.title, job.label = t.Title, hlDlLabel(ids, sel)
	if job.title == "" {
		job.title = tt.Name()
	}
	hlDlMu.Unlock()
	done, total := hlDlProgress(tt, sel, hlPieceComplete(tt))
	if err := hlDlCheckSpace(job.Hash, total-done); err != nil {
		return err
	}
	log.TLogln("homelab: download", job.Hash, tt.Name(), "files:", len(sel), "left:", (total-done)>>20, "MB")

	var (
		reader       *torrstor.Reader
		readerFile   *torrent.File
		piece        = -1
		lastSeek     time.Time
		lastDone     = done
		lastProgress = time.Now()
	)
	closeReader := func() {
		if reader != nil {
			torrstor.HomelabUnmarkDownloadReader(reader)
			t.CloseReader(reader)
			reader, readerFile = nil, nil
		}
	}
	defer closeReader()

	for tick := 0; ; tick++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tt.Closed():
			return errHlReopen
		default:
		}
		if t.Stat == state.TorrentClosed {
			return errHlReopen
		}
		if !settings.HomelabPersistentCache() {
			return errHlPaused
		}

		hlDlMu.Lock()
		ids = append(ids[:0], job.Files...)
		hlDlMu.Unlock()
		if sel = hlDlSelect(tt, ids); len(sel) == 0 {
			return &HomelabDlError{Code: HomelabDlNoFiles}
		}
		complete := hlPieceComplete(tt)
		done, total = hlDlProgress(tt, sel, complete)
		if done > lastDone {
			lastDone, lastProgress = done, time.Now()
		}
		hlDlMu.Lock()
		job.done, job.total, job.speed = done, total, t.DownloadSpeed
		stalled := time.Since(lastProgress) > hlDlStall && hlDlHasOtherLocked(job)
		hlDlMu.Unlock()

		f, next := hlDlNextPiece(sel, complete, pl)
		if f == nil {
			return nil
		}
		if stalled {
			return errHlRotate
		}
		if f != readerFile {
			closeReader()
			if reader = t.NewReader(f); reader == nil {
				return errHlReopen
			}
			torrstor.HomelabMarkDownloadReader(reader)
			readerFile, piece = f, -1
		}
		if next != piece || time.Since(lastSeek) > hlDlReseek {
			off := int64(next)*pl - f.Offset()
			if off < 0 {
				off = 0
			}
			if _, err := reader.Seek(off, io.SeekStart); err != nil {
				return errHlReopen
			}
			piece, lastSeek = next, time.Now()
		}
		t.AddExpiredTime(time.Minute)

		if tick%10 == 0 {
			if free, ok := torrstor.HomelabDiskFree(); ok && free < hlDlDiskFloor {
				torrstor.HomelabKick()
				return &HomelabDlError{Code: HomelabDlDiskFull}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(hlDlTick):
		}
	}
}

// helpers

// hlDlLabel — what a job downloads, for a notification.
func hlDlLabel(ids []int, sel []hlDlFile) string {
	switch {
	case len(ids) == 0:
		return "весь торрент"
	case len(sel) == 1:
		return path.Base(sel[0].Path())
	default:
		return fmt.Sprintf("файлов: %d", len(sel))
	}
}

// hlDlTitle — the torrent of a job for a notification (the job may not have run yet).
func hlDlTitle(job *hlDlJob) string {
	if job.title != "" {
		return job.title
	}
	if db := GetTorrentDB(metainfo.NewHashFromHex(job.Hash)); db != nil && db.Title != "" {
		return db.Title
	}
	return job.Hash
}

// hlDlErrorText — why a job can not run, in words (the web UI has its own translations of the codes).
func hlDlErrorText(e *HomelabDlError) string {
	switch e.Code {
	case HomelabDlLimit:
		return fmt.Sprintf("Не влезает в лимит кэша: нужно ещё %s, рядом с закреплённым осталось %s. "+
			"Увеличьте лимит или открепите что-нибудь.", hlGB(e.Need), hlGB(e.Avail))
	case HomelabDlDisk:
		return fmt.Sprintf("Не хватает места на диске: нужно ещё %s, доступно %s.", hlGB(e.Need), hlGB(e.Avail))
	case HomelabDlDiskFull:
		return "На диске кончилось место — загрузка остановлена. Повторю через 5 минут."
	case HomelabDlNoInfo:
		return "Нет данных о торренте — похоже, его никто не раздаёт. Повторю через 5 минут."
	default:
		return fmt.Sprintf("Не удалось скачать (%s). Повторю через 5 минут.", e.Code)
	}
}

func hlGB(n int64) string {
	return fmt.Sprintf("%.1f ГБ", float64(n)/(1<<30))
}

type hlDlFile struct {
	*torrent.File
	id int
}

// hlDlSelect — files with the ids of file_stats (sorted by path, from 1); ids empty — all of them.
func hlDlSelect(tt *torrent.Torrent, ids []int) []hlDlFile {
	files := append([]*torrent.File(nil), tt.Files()...)
	sort.Slice(files, func(i, j int) bool { return utils2.CompareStrings(files[i].Path(), files[j].Path()) })
	want := map[int]bool{}
	for _, id := range ids {
		want[id] = true
	}
	sel := make([]hlDlFile, 0, len(files))
	for i, f := range files {
		if len(ids) == 0 || want[i+1] {
			sel = append(sel, hlDlFile{File: f, id: i + 1})
		}
	}
	return sel
}

func hlPieceComplete(tt *torrent.Torrent) []bool {
	complete := make([]bool, 0, tt.NumPieces())
	for _, run := range tt.PieceStateRuns() {
		for i := 0; i < run.Length; i++ {
			complete = append(complete, run.Complete)
		}
	}
	return complete
}

// hlFileDone — bytes of the file in complete pieces.
func hlFileDone(f *torrent.File, complete []bool, pl int64) int64 {
	return hlSpanDone(f.Offset(), f.Length(), complete, pl)
}

// hlSpanDone — bytes of [start, start+length) of the torrent in complete pieces.
func hlSpanDone(start, length int64, complete []bool, pl int64) (done int64) {
	if length <= 0 || pl <= 0 {
		return 0
	}
	end := start + length
	for p := start / pl; p <= (end-1)/pl && int(p) < len(complete); p++ {
		if complete[p] {
			done += min(p*pl+pl, end) - max(p*pl, start)
		}
	}
	return done
}

func hlDlProgress(tt *torrent.Torrent, sel []hlDlFile, complete []bool) (done, total int64) {
	pl := tt.Info().PieceLength
	for _, f := range sel {
		total += f.Length()
		done += hlFileDone(f.File, complete, pl)
	}
	return done, total
}

// hlDlNextPiece — the first piece that is not complete, in the order of the files.
func hlDlNextPiece(sel []hlDlFile, complete []bool, pl int64) (*torrent.File, int) {
	for _, f := range sel {
		if p := hlSpanNext(f.Offset(), f.Length(), complete, pl); p >= 0 {
			return f.File, p
		}
	}
	return nil, -1
}

// hlSpanNext — the first piece of [start, start+length) that is not complete, -1 if all are.
func hlSpanNext(start, length int64, complete []bool, pl int64) int {
	if length <= 0 || pl <= 0 {
		return -1
	}
	for p := int(start / pl); p <= int((start+length-1)/pl) && p < len(complete); p++ {
		if !complete[p] {
			return p
		}
	}
	return -1
}

// hlDlCheckSpace — need more bytes fit into the limit next to what is pinned, and onto the disk
// (unpinned cache of other torrents counts as free: the janitor evicts it).
func hlDlCheckSpace(hash string, need int64) error {
	if need <= 0 {
		return nil
	}
	items, u := torrstor.HomelabList()
	var pinnedOther, evictable, self int64
	for _, it := range items {
		switch {
		case it.Hash == hash:
			self += it.Size
		case it.Pinned:
			pinnedOther += it.Size
		default:
			evictable += it.Size
		}
	}
	if u.Limit > 0 {
		if avail := u.Limit - pinnedOther - self; need > avail {
			return &HomelabDlError{Code: HomelabDlLimit, Need: need, Avail: max(avail, 0)}
		}
	}
	if u.DiskTotal > 0 {
		if avail := u.DiskFree + evictable - torrstor.HomelabMinFree; need > avail {
			return &HomelabDlError{Code: HomelabDlDisk, Need: need, Avail: max(avail, 0)}
		}
	}
	return nil
}

func hlUnion(a, b []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(a)+len(b))
	for _, v := range append(append([]int(nil), a...), b...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func hlMinus(a, b []int) []int {
	drop := map[int]bool{}
	for _, v := range b {
		drop[v] = true
	}
	out := make([]int, 0, len(a))
	for _, v := range a {
		if !drop[v] {
			out = append(out, v)
		}
	}
	return out
}
