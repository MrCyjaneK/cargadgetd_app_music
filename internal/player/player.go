// Package player supplies the music plugin. The backend name is the music
// plugin file's backend key. A new backend implements Source and registers
// itself with Register.
package player

import (
	"image"
	"strings"
)

// IdleTitle is the card copy when the selected backend has nothing playing.
const IdleTitle = "Nothing is playing"

// Track is one snapshot. Art is nil when the backend has no cover.
// LengthUs and PositionUs are microseconds, the MPRIS unit.
// Demo is the built-in Ghost Signal card.
type Track struct {
	ID          string
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Status      string
	LengthUs    int64
	PositionUs  int64
	Art         image.Image
	Demo        bool
	Err         string
}

// QueueTrack is one row of the player's track list.
type QueueTrack struct {
	ID, Title, Artist, Album string
}

// Live is a player that is actually on a track. Stopped, missing, and empty
// snapshots are not live, even if they still carry the previous title.
func (t Track) Live() bool {
	if t.Title == "" && t.Artist == "" {
		return false
	}
	return t.Status == "Playing" || t.Status == "Paused"
}

// Source is one music backend. Snapshot and Queue must not block. Close stops
// any background poll and may be called more than once. Control and GoTo must
// not block the caller. Control's cmd is playpause, next, or prev. SyncQueue
// asks for a queue refresh and must not block; the music sheet calls it when
// the sheet opens.
type Source interface {
	Snapshot() Track
	Queue() []QueueTrack
	Close()
	Control(cmd string)
	GoTo(id string)
	SyncQueue()
}

// Backend is one value of the music plugin's backend key.
type Backend struct {
	ID, Label string
}

// Backends is the settings list, in display order.
func Backends() []Backend {
	return []Backend{
		{ID: "mock", Label: "Mock"},
		{ID: "mpris", Label: "MPRIS"},
	}
}

// Normalize maps a config value onto a known backend id.
// Empty and unknown values are mpris.
func Normalize(kind string) string {
	kind = strings.TrimSpace(kind)
	for _, b := range Backends() {
		if strings.EqualFold(kind, b.ID) || strings.EqualFold(kind, b.Label) {
			return b.ID
		}
	}
	return "mpris"
}

// Label is the settings caption for a backend id.
func Label(kind string) string {
	kind = Normalize(kind)
	for _, b := range Backends() {
		if b.ID == kind {
			return b.Label
		}
	}
	return "MPRIS"
}

var backends = map[string]func() Source{}

// Register adds a backend. A later registration replaces
// the same id.
func Register(id string, open func() Source) {
	if id == "" || open == nil {
		return
	}
	backends[id] = open
}

func init() {
	Register("mock", func() Source { return mockSource{} })
	Register("mpris", openMPRIS)
}

// Open returns the named backend. Empty and unknown names are mpris.
func Open(kind string) Source {
	kind = Normalize(kind)
	if open, ok := backends[kind]; ok && open != nil {
		return open()
	}
	return mockSource{}
}
