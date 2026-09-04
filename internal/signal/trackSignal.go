package signal

import (
	"fmt"
	"time"

	"github.com/raphaelreyna/BufferKing/internal/library"
)

type Status int

const (
	Play  Status = 1
	Pause Status = -1

	// Used for comparing Track signals; diff = before - after
	None     Status = Play - Play  // 0
	Paused   Status = Play - Pause // 2
	Resumed  Status = Pause - Play // -2
	NewTrack Status = 3
	Seek     Status = 4
)

func (s Status) String() string {
	switch s {
	case Play:
		return "Play"
	case Pause:
		return "Pause"
	case None:
		return ""
	case Paused:
		return "Paused Playing"
	case Resumed:
		return "Resumed Playing"
	case NewTrack:
		return "New Track"
	case Seek:
		return "Seek"
	}

	return "INVALID_STATUS"
}

type TrackSignal struct {
	library.Track
	Status

	// The time when the song Started playing, can change if seek to 0 or using previous song command.
	Started time.Time
	// Did seek to non-zero time in the track.
	HasSeek bool
}

// Compare compares the old tracksignal to the new tracksignal
func (t *TrackSignal) Compare(tt *TrackSignal) Status {
	if t == nil && tt == nil {
		return None
	}
	hasNewMetadata := tt.Track.Title != "" || tt.Track.Artist != ""
	if t == nil {
		// First signal received
		if tt.HasSeek {
			return Seek
		}
		if tt.Status == Play && hasNewMetadata {
			return NewTrack
		}
		return Paused
	} else if tt == nil {
		// Can't assume audio is still playing, should never happen normally
		return Paused
	}
	// both t and tt are non null:

	if t.HasSeek != tt.HasSeek {
		return Seek
	}
	if hasNewMetadata {
		sameTrack := t.Title == tt.Title
		sameTrack = sameTrack && t.Artist == tt.Artist
		sameTrack = sameTrack && t.Album == tt.Album
		sameTrack = sameTrack && t.TrackNumber == tt.TrackNumber
		sameTrack = sameTrack && t.Length == tt.Length
		if !sameTrack {
			return NewTrack
		}
	}
	if !tt.Started.IsZero() && tt.Started != t.Started {
		return NewTrack
	}

	return t.Status - tt.Status
}

func (t *TrackSignal) String() string {
	return fmt.Sprintf("%s - started at %s (seek: %t) - %s", t.Status, t.Started, t.HasSeek, t.Track.SimpleString())
}
