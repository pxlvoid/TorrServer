package settings

// homelab: settings of the persistent disk cache (pxlvoid/TorrServer fork, see HOMELAB.md).
// Stored separately from BTSets under "Settings"/"Homelab" so the upstream struct stays untouched.

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/url"
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
	// While a file is open (playing or paused), download it to the end instead of only the CacheSize
	// window ahead of the player. Only in persistent mode: the rest lands in the disk cache.
	BackgroundFill bool `json:"backgroundFill"`
	// Halfway through an episode of a series, quietly fetch the next one to disk, so autoplay starts it
	// from the disk instead of buffering from peers (torr/homelab_nextep.go).
	NextEpisode bool `json:"nextEpisode"`
	// Stop the preload once the swarm is clearly faster than the file needs, instead of always buffering
	// the configured share of CacheSize (torr/homelab_preload.go). Never buffers more than upstream would.
	AdaptivePreload bool `json:"adaptivePreload"`
}

// fields missing in the stored JSON keep these values (BackgroundFill appeared later — stays on)
var homelabDefaults = HomelabSets{
	PersistentCache: false, LimitGB: 100, KeepDays: 7,
	BackgroundFill: true, NextEpisode: true, AdaptivePreload: true,
}

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

// HomelabDownloadJob — a queued "download to disk" (torr/homelab_download.go), stored so the queue survives a restart.
type HomelabDownloadJob struct {
	Hash      string `json:"hash"`
	Files     []int  `json:"files,omitempty"` // file ids as in file_stats; empty — the whole torrent
	Added int64 `json:"added"`
	// queued by the server itself — the next episode of what is being watched (torr/homelab_nextep.go).
	// Such a job yields to the ones the user asked for and gives up quietly when it does not fit.
	Auto bool `json:"auto,omitempty"`
}

// stored as an object: JsonDB (StoreSettingsInJson) keeps only objects under a key, not arrays
type homelabDownloads struct {
	Jobs []HomelabDownloadJob `json:"jobs"`
}

// GetHomelabDownloads — the stored download queue.
func GetHomelabDownloads() []HomelabDownloadJob {
	var stored homelabDownloads
	if tdb == nil {
		return nil
	}
	if buf := tdb.Get("Settings", "HomelabDownloads"); len(buf) > 0 {
		if err := json.Unmarshal(buf, &stored); err != nil {
			log.TLogln("Error unmarshal homelab downloads:", err)
			return nil
		}
	}
	return stored.Jobs
}

// SetHomelabDownloads stores the download queue.
func SetHomelabDownloads(jobs []HomelabDownloadJob) {
	if ReadOnly || tdb == nil {
		return
	}
	if jobs == nil {
		jobs = []HomelabDownloadJob{}
	}
	if buf, err := json.Marshal(homelabDownloads{Jobs: jobs}); err == nil {
		tdb.Set("Settings", "HomelabDownloads", buf)
	}
}

// SetHomelabSetsForTest replaces the settings without the DB (tests only).
func SetHomelabSetsForTest(sets HomelabSets) {
	homelabMu.Lock()
	defer homelabMu.Unlock()
	homelabSets = &sets
}

// HomelabAudioChoice — the audio track served to players for a torrent (torr/homelab_audio.go): the others are
// hidden in the stream. A series may have different dubs in different episodes, so it is a list in order of
// preference: an episode gets the first of them it has; with none of them — a track in the language of the first.
// An episode may have its own track (Files, by the file path in the torrent): it wins over the list.
type HomelabAudioChoice struct {
	Tracks []HomelabAudioPref          `json:"tracks"`
	Files  map[string]HomelabAudioPref `json:"files,omitempty"`
}

// HomelabAudioPref — a track as the user chose it: matched by name, language breaks ties. Index — the position
// among the audio tracks of the file, used for a track chosen for one episode.
type HomelabAudioPref struct {
	Name  string `json:"name"`
	Lang  string `json:"lang"`
	Index int    `json:"index,omitempty"`
}

type homelabAudio struct {
	Torrents map[string]HomelabAudioChoice `json:"torrents"`
}

var (
	homelabAudioMu     sync.Mutex
	homelabAudioCached *homelabAudio
)

func homelabAudioLoadLocked() *homelabAudio {
	if homelabAudioCached != nil {
		return homelabAudioCached
	}
	stored := &homelabAudio{}
	if tdb != nil {
		if buf := tdb.Get("Settings", "HomelabAudio"); len(buf) > 0 {
			if err := json.Unmarshal(buf, stored); err != nil {
				log.TLogln("Error unmarshal homelab audio:", err)
			}
		}
		homelabAudioCached = stored // cache only once the DB is available
	}
	if stored.Torrents == nil {
		stored.Torrents = map[string]HomelabAudioChoice{}
	}
	return stored
}

// GetHomelabAudio — the chosen audio track of a torrent; ok=false — all tracks, as in the file.
func GetHomelabAudio(hash string) (HomelabAudioChoice, bool) {
	homelabAudioMu.Lock()
	defer homelabAudioMu.Unlock()
	choice, ok := homelabAudioLoadLocked().Torrents[strings.ToLower(hash)]
	return choice, ok
}

// SetHomelabAudio stores the choice; nil or no tracks — serve all tracks again.
func SetHomelabAudio(hash string, choice *HomelabAudioChoice) error {
	if ReadOnly {
		return errors.New("read-only mode")
	}
	if choice != nil && len(choice.Tracks) == 0 && len(choice.Files) == 0 {
		choice = nil
	}
	homelabAudioMu.Lock()
	defer homelabAudioMu.Unlock()
	stored := homelabAudioLoadLocked()
	if choice == nil {
		delete(stored.Torrents, strings.ToLower(hash))
	} else {
		stored.Torrents[strings.ToLower(hash)] = *choice
	}
	buf, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if tdb != nil {
		tdb.Set("Settings", "HomelabAudio", buf)
	}
	return nil
}

// HomelabNtfy — ntfy notifications of the fork (torrstor/homelab_ntfy.go). Stored apart from HomelabSets: the disk
// cache dialog saves those as a whole and would drop these fields.
type HomelabNtfy struct {
	URL    string          `json:"url"`   // ntfy server, e.g. https://ntfy.example.org
	Topic  string          `json:"topic"` // the channel, "ts" by default
	Token  string          `json:"token,omitempty"`
	Events map[string]bool `json:"events"` // event → on
}

// events of the notifications and whether they are on by default
var HomelabNtfyEvents = map[string]bool{
	"download_done":  true,  // a download to disk is complete
	"download_error": true,  // a download can not run: no space, the limit, nobody seeds
	"disk":           true,  // the cache does not fit into the limit / little free space on the disk
	"started":        false, // the server has started (a deploy)
}

var (
	homelabNtfyMu     sync.Mutex
	homelabNtfyCached *HomelabNtfy
)

// GetHomelabNtfy — the ntfy settings, defaults filled in.
func GetHomelabNtfy() HomelabNtfy {
	homelabNtfyMu.Lock()
	defer homelabNtfyMu.Unlock()
	if homelabNtfyCached == nil {
		n := HomelabNtfy{}
		if tdb != nil {
			if buf := tdb.Get("Settings", "HomelabNtfy"); len(buf) > 0 {
				if err := json.Unmarshal(buf, &n); err != nil {
					log.TLogln("Error unmarshal homelab ntfy:", err)
				}
			}
		}
		if n.Topic == "" {
			n.Topic = "ts"
		}
		if tdb == nil {
			return homelabNtfyWithDefaults(n)
		}
		homelabNtfyCached = &n
	}
	return homelabNtfyWithDefaults(*homelabNtfyCached)
}

func homelabNtfyWithDefaults(n HomelabNtfy) HomelabNtfy {
	events := make(map[string]bool, len(HomelabNtfyEvents))
	for ev, on := range HomelabNtfyEvents {
		events[ev] = on
		if v, ok := n.Events[ev]; ok {
			events[ev] = v
		}
	}
	n.Events = events
	return n
}

// NormalizeHomelabNtfy validates the ntfy settings. A URL with a path ("https://ntfy.example.org/torrserver") is
// split into the server and the topic; events get their defaults.
func NormalizeHomelabNtfy(n HomelabNtfy) (HomelabNtfy, error) {
	n.URL, n.Topic = strings.TrimSpace(n.URL), strings.Trim(strings.TrimSpace(n.Topic), "/")
	if n.URL != "" {
		u, err := url.Parse(n.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return n, errors.New("ntfy: the address must be http(s)://server")
		}
		if path := strings.Trim(u.Path, "/"); path != "" && n.Topic == "" {
			n.Topic = path
		}
		n.URL = u.Scheme + "://" + u.Host
	}
	if n.Topic == "" {
		n.Topic = "ts"
	}
	return homelabNtfyWithDefaults(n), nil
}

// SetHomelabNtfy validates and stores the ntfy settings.
func SetHomelabNtfy(n HomelabNtfy) error {
	if ReadOnly {
		return errors.New("read-only mode")
	}
	n, err := NormalizeHomelabNtfy(n)
	if err != nil {
		return err
	}
	buf, err := json.Marshal(n)
	if err != nil {
		return err
	}
	homelabNtfyMu.Lock()
	defer homelabNtfyMu.Unlock()
	if tdb != nil {
		tdb.Set("Settings", "HomelabNtfy", buf)
	}
	homelabNtfyCached = &n
	return nil
}

// SetHomelabNtfyForTest replaces the ntfy settings without the DB (tests only).
func SetHomelabNtfyForTest(n HomelabNtfy) {
	homelabNtfyMu.Lock()
	defer homelabNtfyMu.Unlock()
	homelabNtfyCached = &n
}
