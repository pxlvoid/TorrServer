package api

// homelab: API of the persistent disk cache (pxlvoid/TorrServer fork, see HOMELAB.md).
// Registered by the "// homelab" hook in SetupRoute.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	sets "server/settings"
	"server/torr/storage/torrstor"
)

func homelabRoutes(authorized gin.IRouter) {
	authorized.POST("/homelab/cache", homelabCache)
	authorized.GET("/homelab/settings", homelabGetSettings)
	authorized.POST("/homelab/settings", homelabSetSettings)
	torrstor.HomelabStartJanitor()
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
}

type homelabCacheResp struct {
	Settings sets.HomelabSets      `json:"settings"`
	Usage    torrstor.HomelabUsage `json:"usage"`
	Upstream string                `json:"upstream"`
	Items    []homelabCacheItem    `json:"items"`
	Freed    int64                 `json:"freed"` // bytes freed by remove / clear
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
		Settings: sets.GetHomelabSets(),
		Usage:    usage,
		Upstream: sets.HomelabUpstream(),
		Items:    make([]homelabCacheItem, 0, len(items)),
	}
	for _, it := range items {
		item := homelabCacheItem{HomelabItem: it}
		if t := known[it.Hash]; t != nil {
			item.Title, item.Poster = t.Title, t.Poster
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
		freed, err = torrstor.HomelabRemove(req.Hash)
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
