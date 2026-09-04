package signal

import (
	"fmt"
	"time"

	"github.com/Lej77/BufferKing/internal/library"
)

type Status int

const (
	Play  Status = 1
	Pause Status = -1

	// Used for comparing Track signals; diff = before - after
	None           Status = Play - Play  // 0
	Paused         Status = Play - Pause // 2
	Resumed        Status = Pause - Play // -2
	NewTrack       Status = 3
	Seek           Status = 4
	SwitchedPlayer Status = 5
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
	case SwitchedPlayer:
		return "Switched Media Player"
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
	// A name that identifies the media player that emitted this signal.
	MediaPlayerName string
}

// Compare compares the old tracksignal to the new tracksignal
func (t *TrackSignal) Compare(tt *TrackSignal) Status {
	if t == nil && tt == nil {
		return None
	}
	if t == nil {
		// First signal received, can't know for sure if safe to record, i.e. if new track
		return None
	} else if tt == nil {
		// Should never happen! To be safe don't assume audio is still playing:
		return Paused
	}
	// both t and tt are non null:

	if t.MediaPlayerName != tt.MediaPlayerName {
		return SwitchedPlayer
	}

	hasNewMetadata := tt.Track.Title != "" || tt.Track.Artist != ""
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
	if t.HasSeek != tt.HasSeek && tt.HasSeek {
		return Seek
	}
	if !tt.Started.IsZero() && !tt.Started.Equal(t.Started) {
		return NewTrack
	}

	return t.Status - tt.Status
}

func (t *TrackSignal) String() string {
	return fmt.Sprintf("%s - status: %s - started at: %s - seek: %t - %s", t.MediaPlayerName, t.Status, t.Started, t.HasSeek, t.Track.SimpleString())
}
