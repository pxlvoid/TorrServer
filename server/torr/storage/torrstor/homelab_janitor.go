package torrstor

// homelab: the janitor of the persistent disk cache — global budget and age limit (see homelab.go).
//
// Lock order: never call into a Cache or anacrolix while holding hlMu (hlOnInit runs under the
// storage and client locks and takes hlMu). Files of closed torrents are removed under hlMu,
// pieces of open caches — via hlCache.release outside of it.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"server/log"
	"server/settings"
)

const (
	hlJanitorEvery = time.Minute
	hlMinFree      = int64(5 << 30) // always keep this much free on the cache disk
	hlSlack        = 0.95           // over the limit — evict down to 95% of it, not to the byte
)

var (
	hlJanitorOnce sync.Once
	hlKickCh      = make(chan struct{}, 1)
)

// HomelabStartJanitor starts the background janitor (once).
func HomelabStartJanitor() {
	hlJanitorOnce.Do(func() {
		go func() {
			t := time.NewTicker(hlJanitorEvery)
			defer t.Stop()
			for {
				select {
				case <-t.C:
				case <-hlKickCh:
				}
				hlJanitorPass(time.Now())
			}
		}()
	})
}

// HomelabKick runs a janitor pass now (after settings change).
func HomelabKick() {
	HomelabStartJanitor()
	select {
	case hlKickCh <- struct{}{}:
	default:
	}
}

// hlCand — something the janitor may evict: a piece of an open cache or a piece file of a closed torrent.
type hlCand struct {
	access int64
	size   int64
	hash   string
	h      *hlCache
	p      *Piece
	path   string
}

type hlFile struct {
	path  string
	size  int64
	mtime int64
}

func hlIsHash(name string) bool {
	if len(name) != 40 {
		return false
	}
	for _, r := range name {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// hlPieceFiles — piece files (named by piece index) in a torrent cache dir.
func hlPieceFiles(dir string) []hlFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]hlFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, hlFile{path: filepath.Join(dir, e.Name()), size: fi.Size(), mtime: fi.ModTime().Unix()})
	}
	return files
}

func hlOpenSnapshot() []*hlCache {
	hlMu.Lock()
	defer hlMu.Unlock()
	list := make([]*hlCache, 0, len(hlByHash))
	for _, h := range hlByHash {
		list = append(list, h)
	}
	return list
}

func hlIsOpenLocked(hash string) bool {
	if _, ok := hlByHash[hash]; ok {
		return true
	}
	return false
}

// hlRemoveClosed removes files of a closed torrent unless it has been opened meanwhile; drops an empty dir.
func hlRemoveClosed(hash string, paths []string) (freed int64) {
	hlMu.Lock()
	defer hlMu.Unlock()
	if hlIsOpenLocked(hash) {
		return 0
	}
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			if os.Remove(p) == nil {
				freed += fi.Size()
			}
		}
	}
	dir := filepath.Join(hlRoot(), hash)
	if len(hlPieceFiles(dir)) == 0 {
		_ = os.RemoveAll(dir)
	}
	return freed
}

// hlJanitorPass — one pass: flush metadata, drop old pieces, fit into the budget. Returns bytes freed.
func hlJanitorPass(now time.Time) (freed int64) {
	defer func() {
		if r := recover(); r != nil {
			log.TLogln("homelab: janitor panic:", r)
		}
	}()
	if !hlEnabled() {
		return 0
	}
	sets := settings.GetHomelabSets()
	root := hlRoot()

	open := hlOpenSnapshot()
	hlMu.Lock()
	for _, h := range open {
		h.flushLocked()
	}
	hlMu.Unlock()

	var cands []hlCand
	var total int64
	openHash := make(map[string]bool, len(open))

	// open caches: everything outside active reader windows is a candidate
	for _, h := range open {
		openHash[h.meta.Hash] = true
		c := h.c
		if c.isClosed.Load() {
			continue
		}
		for _, p := range c.getPieces() {
			if p.Size > 0 {
				total += p.Size
			}
		}
		if h.pinned() {
			continue
		}
		for _, p := range c.getRemPieces() {
			cands = append(cands, hlCand{access: p.Accessed, size: p.Size, hash: h.meta.Hash, h: h, p: p})
		}
	}

	// closed torrents: piece files on disk
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() || !hlIsHash(e.Name()) || openHash[e.Name()] {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files := hlPieceFiles(dir)
		if len(files) == 0 {
			hlRemoveClosed(e.Name(), nil) // leftover dir without pieces
			continue
		}
		meta, _ := hlReadMeta(dir)
		pinned := meta != nil && meta.Pinned
		for _, f := range files {
			total += f.size
			if !pinned {
				cands = append(cands, hlCand{access: f.mtime, size: f.size, hash: e.Name(), path: f.path})
			}
		}
	}

	// budget: LimitGB, and never less than hlMinFree free on the disk
	allowed := int64(-1)
	if sets.LimitGB > 0 {
		allowed = sets.LimitGB << 30
	}
	if free, _, ok := hlDiskStat(root); ok && free < hlMinFree {
		byFree := total - (hlMinFree - free)
		if byFree < 0 {
			byFree = 0
		}
		if allowed < 0 || byFree < allowed {
			allowed = byFree
		}
	}
	overBudget := allowed >= 0 && total > allowed
	target := allowed
	if overBudget {
		target = int64(float64(allowed) * hlSlack)
	}
	cutoff := int64(-1)
	if sets.KeepDays > 0 {
		cutoff = now.Unix() - int64(sets.KeepDays)*86400
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].access < cands[j].access })

	closed := map[string][]string{}
	for _, cd := range cands {
		old := cutoff >= 0 && cd.access < cutoff
		if !old && !(overBudget && total > target) {
			break // sorted by access: the rest is newer, and the budget is already fine
		}
		total -= cd.size
		if cd.h != nil {
			cd.h.release(cd.p)
			freed += cd.size
		} else {
			closed[cd.hash] = append(closed[cd.hash], cd.path)
		}
	}
	for hash, paths := range closed {
		freed += hlRemoveClosed(hash, paths)
	}
	if freed > 0 {
		log.TLogln(fmt.Sprintf("homelab: janitor freed %d MB, cache now %d MB", freed>>20, total>>20))
	}
	return freed
}
