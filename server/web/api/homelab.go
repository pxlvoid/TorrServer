package api

// homelab: API of the persistent disk cache (pxlvoid/TorrServer fork, see HOMELAB.md).
// Registered by the "// homelab" hook in SetupRoute.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	sets "server/settings"
	"server/torr"
	"server/torr/storage/torrstor"
)

func homelabRoutes(authorized gin.IRouter) {
	authorized.POST("/homelab/cache", homelabCache)
	authorized.GET("/homelab/settings", homelabGetSettings)
	authorized.POST("/homelab/settings", homelabSetSettings)
	authorized.POST("/homelab/download", homelabDownload)
	authorized.POST("/homelab/audio", homelabAudio)
	authorized.POST("/homelab/pieces", homelabPieces)
	authorized.POST("/homelab/ntfy", homelabNtfy)
	authorized.GET("/homelab/streams", homelabStreams)
	torrstor.HomelabStartJanitor()
	torr.HomelabDownloadsStart()
}

type homelabCacheReq struct {
	Action string `json:"action"` // list | remove | pin | clear
	Hash   string `json:"hash,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

type homelabCacheItem struct {
	torrstor.HomelabItem
	Title  string `json:"title,omitempty"`
	Poster string `json:"poster,omitempty"`
	// a series (several video files): how many episodes are completely on disk
	Episodes     int `json:"episodes,omitempty"`
	EpisodesDone int `json:"episodesDone"`
}

type homelabCacheResp struct {
	Settings  sets.HomelabSets       `json:"settings"`
	Usage     torrstor.HomelabUsage  `json:"usage"`
	Upstream  string                 `json:"upstream"`
	Items     []homelabCacheItem     `json:"items"`
	Downloads []torr.HomelabDownload `json:"downloads"` // the download queue, in order
	Freed     int64                  `json:"freed"`     // bytes freed by remove / clear
}

func homelabList() homelabCacheResp {
	items, usage := torrstor.HomelabList()
	// titles and posters of torrents saved in TorrServer (added from Lampa, the web UI...)
	known := map[string]*sets.TorrentDB{}
	for _, t := range sets.ListTorrent() {
		if t != nil && t.TorrentSpec != nil {
			known[t.InfoHash.HexString()] = t
		}
	}
	resp := homelabCacheResp{
		Settings:  sets.GetHomelabSets(),
		Usage:     usage,
		Upstream:  sets.HomelabUpstream(),
		Items:     make([]homelabCacheItem, 0, len(items)),
		Downloads: torr.HomelabDownloads(),
	}
	listed := map[string]bool{}
	for _, it := range items {
		listed[it.Hash] = true
		item := homelabCacheItem{HomelabItem: it, Title: it.SavedTitle, Poster: it.SavedPoster}
		if t := known[it.Hash]; t != nil {
			item.Title, item.Poster = t.Title, t.Poster
			if t.Title != it.SavedTitle || t.Poster != it.SavedPoster {
				torrstor.HomelabSetInfo(it.Hash, t.Title, t.Poster) // the card keeps them if the torrent is removed
			}
		}
		if done, total, ok := torr.HomelabEpisodes(it.Hash); ok && total > 1 {
			item.Episodes, item.EpisodesDone = total, done
		}
		resp.Items = append(resp.Items, item)
	}
	// queued downloads of torrents that have nothing on disk yet: a card for them too
	for _, d := range resp.Downloads {
		if listed[d.Hash] {
			continue
		}
		item := homelabCacheItem{HomelabItem: torrstor.HomelabItem{Hash: d.Hash, TotalLength: d.Total, LastAccess: d.Added}}
		if t := known[d.Hash]; t != nil {
			item.Title, item.Poster, item.TotalLength = t.Title, t.Poster, t.Size
			if d.Total > 0 {
				item.TotalLength = d.Total
			}
		}
		resp.Items = append(resp.Items, item)
	}
	return resp
}

func homelabCache(c *gin.Context) {
	var req homelabCacheReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	var err error
	var freed int64
	switch req.Action {
	case "list":
	case "remove":
		// the whole cache: stop its download, close the torrent (even if someone watches — the UI asks
		// first), otherwise only pieces outside the player window would go, and with background fill
		// that window is the rest of the file
		torr.HomelabDownloadStop(req.Hash, nil)
		if torrstor.HomelabIsOpen(req.Hash) {
			torr.HomelabCloseTorrent(req.Hash)
		}
		freed, err = torrstor.HomelabRemove(req.Hash)
		if errors.Is(err, torrstor.ErrHomelabNotFound) {
			err = nil // only a queued download, nothing on disk yet
		}
	case "pin":
		err = torrstor.HomelabSetPinned(req.Hash, req.Pinned)
	case "clear":
		freed = torrstor.HomelabClear()
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, torrstor.ErrHomelabOpen) {
			status = http.StatusConflict
		} else if errors.Is(err, torrstor.ErrHomelabNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	resp := homelabList()
	resp.Freed = freed
	c.JSON(http.StatusOK, resp)
}

type homelabDownloadReq struct {
	Action string `json:"action"` // start | stop | status
	Hash   string `json:"hash"`
	Files  []int  `json:"files,omitempty"` // file ids as in file_stats; empty — the whole torrent
}

type homelabDownloadResp struct {
	Enabled bool                    `json:"enabled"`
	Job     *torr.HomelabDownload   `json:"job"`
	Files   []torr.HomelabFileState `json:"files"` // empty if the torrent is not loaded
}

// homelabDownload — "download to disk": the whole torrent or chosen files (torr/homelab_download.go).
func homelabDownload(c *gin.Context) {
	var req homelabDownloadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	switch req.Action {
	case "start":
		if err := torr.HomelabDownloadStart(req.Hash, req.Files); err != nil {
			var de *torr.HomelabDlError
			if errors.As(err, &de) {
				c.JSON(http.StatusConflict, gin.H{"error": de.Code, "need": de.Need, "avail": de.Avail})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	case "stop":
		torr.HomelabDownloadStop(req.Hash, req.Files)
	case "status":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
		return
	}
	job, files := torr.HomelabDownloadStatus(req.Hash)
	c.JSON(http.StatusOK, homelabDownloadResp{Enabled: sets.HomelabPersistentCache(), Job: job, Files: files})
}

type homelabAudioReq struct {
	// tracks | set (the list for the torrent, own tracks of episodes stay) | file (a track for one episode,
	// reset — back to the torrent's) | files_clear (every episode back to the torrent's) | clear (everything)
	Action string                  `json:"action"`
	Hash   string                  `json:"hash"`
	Tracks []sets.HomelabAudioPref `json:"tracks,omitempty"`
	Path   string                  `json:"path,omitempty"`
	Track  sets.HomelabAudioPref   `json:"track"`
	Reset  bool                    `json:"reset,omitempty"`
}

type homelabAudioResp struct {
	Files  []torr.HomelabAudioFile  `json:"files"`  // video MKVs: their audio tracks and what is served in each
	Choice *sets.HomelabAudioChoice `json:"choice"` // null — all tracks, as in the file
}

// homelabAudio — the audio track served to players (torr/homelab_audio.go). Every action answers with the
// files and what the (new) choice serves in each of them.
func homelabAudio(c *gin.Context) {
	var req homelabAudioReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	choice, _ := sets.GetHomelabAudio(req.Hash)
	files := map[string]sets.HomelabAudioPref{}
	for path, pref := range choice.Files {
		files[path] = pref
	}
	var err error
	switch req.Action {
	case "tracks":
	case "set":
		err = sets.SetHomelabAudio(req.Hash, &sets.HomelabAudioChoice{Tracks: req.Tracks, Files: files})
	case "file":
		if req.Path == "" {
			err = errors.New("no path")
			break
		}
		if req.Reset {
			delete(files, req.Path)
		} else {
			files[req.Path] = req.Track
		}
		err = sets.SetHomelabAudio(req.Hash, &sets.HomelabAudioChoice{Tracks: choice.Tracks, Files: files})
	case "files_clear":
		err = sets.SetHomelabAudio(req.Hash, &sets.HomelabAudioChoice{Tracks: choice.Tracks})
	case "clear":
		err = sets.SetHomelabAudio(req.Hash, nil)
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp := homelabAudioResp{}
	if resp.Files, err = torr.HomelabAudioTracks(req.Hash); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if saved, ok := sets.GetHomelabAudio(req.Hash); ok {
		resp.Choice = &saved
	}
	c.JSON(http.StatusOK, resp)
}

// homelabPieces — the disk map of a torrent for the timeline in the torrent details (torrstor.HomelabPieces).
func homelabPieces(c *gin.Context) {
	var req struct {
		Hash string `json:"hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	m, ok := torrstor.HomelabPieces(strings.ToLower(req.Hash))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no cache for this torrent"})
		return
	}
	c.JSON(http.StatusOK, m)
}

type homelabNtfyReq struct {
	Action     string          `json:"action"` // get | save | test
	URL        string          `json:"url"`
	Topic      string          `json:"topic"`
	Token      string          `json:"token,omitempty"`      // empty — keep the stored one
	TokenClear bool            `json:"tokenClear,omitempty"` // forget the stored token
	Events     map[string]bool `json:"events,omitempty"`
}

// the settings as the web UI sees them: the token itself never leaves the server
type homelabNtfyResp struct {
	URL      string          `json:"url"`
	Topic    string          `json:"topic"`
	TokenSet bool            `json:"tokenSet"`
	Events   map[string]bool `json:"events"`
}

// homelabNtfy — ntfy notifications (torrstor/homelab_ntfy.go). test sends a notification with the form's settings
// without saving them (the stored token if none is given).
func homelabNtfy(c *gin.Context) {
	var req homelabNtfyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	cur := sets.GetHomelabNtfy()
	form := sets.HomelabNtfy{URL: req.URL, Topic: req.Topic, Token: cur.Token, Events: req.Events}
	if req.Token != "" {
		form.Token = req.Token
	}
	if req.TokenClear {
		form.Token = ""
	}
	var err error
	switch req.Action {
	case "get":
	case "save":
		err = sets.SetHomelabNtfy(form)
	case "test": // the form as it is, not saved
		var n sets.HomelabNtfy
		if n, err = sets.NormalizeHomelabNtfy(form); err == nil {
			err = torrstor.HomelabNotifyTest(n)
		}
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n := sets.GetHomelabNtfy()
	c.JSON(http.StatusOK, homelabNtfyResp{URL: n.URL, Topic: n.Topic, TokenSet: n.Token != "", Events: n.Events})
}

// homelabStreams — who is watching now (torr/homelab_streams.go).
func homelabStreams(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"clients": torr.HomelabStreams()})
}

func homelabGetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, sets.GetHomelabSets())
}

func homelabSetSettings(c *gin.Context) {
	var req sets.HomelabSets
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	if err := sets.SetHomelabSets(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	torrstor.HomelabKick()
	c.JSON(http.StatusOK, sets.GetHomelabSets())
}
