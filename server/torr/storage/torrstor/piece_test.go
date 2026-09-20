package torrstor

import (
	"testing"

	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

// Toggling "use disk" while a torrent was open used to take the server down: a piece keeps the store it
// was created with, but WriteAt/ReadAt/Release dispatched on the current setting and dereferenced the
// nil one. Seen in the wild as a nil pointer panic on "drop all torrents" right after the switch.
func TestPieceSurvivesUseDiskToggle(t *testing.T) {
	tests := []struct {
		name    string
		useDisk bool
	}{
		{name: "disk piece, switched to memory", useDisk: true},
		{name: "memory piece, switched to disk", useDisk: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			prevBT, prevHL := settings.BTsets, settings.GetHomelabSets()
			settings.BTsets = &settings.BTSets{UseDisk: tc.useDisk, TorrentsSavePath: root}
			settings.SetHomelabSetsForTest(settings.HomelabSets{})
			t.Cleanup(func() {
				settings.BTsets = prevBT
				settings.SetHomelabSetsForTest(prevHL)
			})

			info := &metainfo.Info{Name: "x", PieceLength: 16, Length: 32, Pieces: make([]byte, 2*20)}
			impl, err := NewStorage(64).OpenTorrent(info, metainfo.NewHashFromHex("cccccccccccccccccccccccccccccccccccccccc"))
			if err != nil {
				t.Fatal(err)
			}
			c := impl.(*Cache)
			p := c.pieces[0]

			settings.BTsets.UseDisk = !tc.useDisk // the switch is flipped while the torrent is open

			if _, err := p.WriteAt(make([]byte, 8), 0); err != nil {
				t.Fatalf("WriteAt after the toggle: %v", err)
			}
			if _, err := p.ReadAt(make([]byte, 8), 0); err != nil {
				t.Fatalf("ReadAt after the toggle: %v", err)
			}
			p.Release() // this is where it used to dereference nil
		})
	}
}
