package settings

// homelab: settings of the persistent disk cache (pxlvoid/TorrServer fork, see HOMELAB.md).
// Stored separately from BTSets under "Settings"/"Homelab" so the upstream struct stays untouched.

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"server/log"
	"server/version"
)

// HomelabSets — persistent disk cache settings.
type HomelabSets struct {
	// Keep cached pieces on disk instead of evicting them by CacheSize; eviction is done by the
	// global janitor (LimitGB / KeepDays). Requires UseDisk and TorrentsSavePath.
	PersistentCache bool `json:"persistentCache"`
	// Total disk budget for the cache, GB. 0 — no limit (only the free space guard).
	LimitGB int64 `json:"limitGB"`
	// Remove pieces not accessed for N days. 0 — never remove by age.
	KeepDays int `json:"keepDays"`
}

var homelabDefaults = HomelabSets{PersistentCache: false, LimitGB: 100, KeepDays: 7}

var (
	homelabMu   sync.RWMutex
	homelabSets *HomelabSets
)

// HomelabUpstream — upstream release the fork is based on; kept up to date by the upstream sync workflow.
//
//go:embed homelab_upstream.txt
var homelabUpstream string

func HomelabUpstream() string {
	return strings.TrimSpace(homelabUpstream)
}

func init() {
	// Docker build of the fork passes VERSION=homelab; show which upstream release it is built on.
	if version.Version == "homelab" {
		version.Version = HomelabUpstream() + "-homelab"
	}
}

// GetHomelabSets returns the current settings (defaults if nothing is stored yet).
func GetHomelabSets() HomelabSets {
	homelabMu.RLock()
	if homelabSets != nil {
		defer homelabMu.RUnlock()
		return *homelabSets
	}
	homelabMu.RUnlock()

	homelabMu.Lock()
	defer homelabMu.Unlock()
	if homelabSets != nil {
		return *homelabSets
	}
	sets := homelabDefaults
	if tdb != nil {
		if buf := tdb.Get("Settings", "Homelab"); len(buf) > 0 {
			if err := json.Unmarshal(buf, &sets); err != nil {
				log.TLogln("Error unmarshal homelab settings:", err)
				sets = homelabDefaults
			}
		}
		homelabSets = &sets // cache only once the DB is available
	}
	return sets
}

// SetHomelabSets validates and stores the settings.
func SetHomelabSets(sets HomelabSets) error {
	if ReadOnly {
		return errors.New("read-only mode")
	}
	if sets.LimitGB < 0 || sets.KeepDays < 0 {
		return errors.New("limitGB and keepDays must not be negative")
	}
	if sets.PersistentCache && (BTsets == nil || !BTsets.UseDisk || BTsets.TorrentsSavePath == "") {
		return errors.New("persistent cache requires UseDisk and TorrentsSavePath")
	}
	buf, err := json.Marshal(sets)
	if err != nil {
		return err
	}
	homelabMu.Lock()
	defer homelabMu.Unlock()
	if tdb != nil {
		tdb.Set("Settings", "Homelab", buf)
	}
	homelabSets = &sets
	return nil
}

// HomelabPersistentCache — persistent disk cache is on: nothing but the homelab janitor and explicit
// removal in the disk cache dialog may delete cached pieces (hooks in cache.go, server.go, apihelper.go).
func HomelabPersistentCache() bool {
	b := BTsets
	return b != nil && b.UseDisk && b.TorrentsSavePath != "" && GetHomelabSets().PersistentCache
}

// SetHomelabSetsForTest replaces the settings without the DB (tests only).
func SetHomelabSetsForTest(sets HomelabSets) {
	homelabMu.Lock()
	defer homelabMu.Unlock()
	homelabSets = &sets
}
