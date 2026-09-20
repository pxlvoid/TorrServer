package torrstor

import (
	"errors"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
	"server/settings"
)

// a piece with neither store: only reachable if NewPiece failed to make one
var errNoStore = errors.New("piece has no store")

type Piece struct {
	storage.PieceImpl `json:"-"`

	Id   int   `json:"-"`
	Size int64 `json:"size"`

	Complete bool  `json:"complete"`
	Accessed int64 `json:"accessed"`

	mPiece *MemPiece  `json:"-"`
	dPiece *DiskPiece `json:"-"`

	cache *Cache `json:"-"`
}

func NewPiece(id int, cache *Cache) *Piece {
	p := &Piece{
		Id:    id,
		cache: cache,
	}

	if !settings.BTsets.UseDisk {
		p.mPiece = NewMemPiece(p)
	} else {
		p.dPiece = NewDiskPiece(p)
	}
	return p
}

// Which store a piece uses is decided once, in NewPiece. Dispatching on the current UseDisk instead
// would follow the setting the moment it is toggled, while every piece of an open torrent still has
// only the other store: the nil one was dereferenced and the server died (a nil pointer panic on
// "drop all torrents" right after "use disk" was switched off).

func (p *Piece) WriteAt(b []byte, off int64) (n int, err error) {
	if p.mPiece != nil {
		return p.mPiece.WriteAt(b, off)
	}
	if p.dPiece != nil {
		return p.dPiece.WriteAt(b, off)
	}
	return 0, errNoStore
}

func (p *Piece) ReadAt(b []byte, off int64) (n int, err error) {
	if p.mPiece != nil {
		return p.mPiece.ReadAt(b, off)
	}
	if p.dPiece != nil {
		return p.dPiece.ReadAt(b, off)
	}
	return 0, errNoStore
}

func (p *Piece) MarkComplete() error {
	p.Complete = true
	return nil
}

func (p *Piece) MarkNotComplete() error {
	p.Complete = false
	return nil
}

func (p *Piece) Completion() storage.Completion {
	return storage.Completion{
		Complete: p.Complete,
		Ok:       true,
	}
}

func (p *Piece) Release() {
	if p.mPiece != nil {
		p.mPiece.Release()
	} else if p.dPiece != nil {
		p.dPiece.Release()
	}
	if p.cache == nil || p.cache.torrent == nil {
		return // released while the cache is being torn down
	}
	p.cache.torrent.Piece(p.Id).SetPriority(torrent.PiecePriorityNone)
	p.cache.torrent.Piece(p.Id).UpdateCompletion()
}
