package signal

import (
	"fmt"
	"strings"
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

type SeekEvent struct {
	Time  time.Time
	Value int64
}

// How far into playback this seek event occurred.
func (e SeekEvent) Offset(playbackStarted time.Time) time.Duration {
	return e.Time.Sub(playbackStarted)
}

// The seek target position.
func (e SeekEvent) Target() time.Duration {
	return time.Duration(e.Value) * time.Microsecond
}

// The change between target position and untouched playback position.
func (e SeekEvent) Delta(playbackStarted time.Time) time.Duration {
	return e.Target() - e.Offset(playbackStarted)
}

// Returns true if the absolute seek delta exceeds minThreshold.
func (e SeekEvent) IsSignificant(playbackStarted time.Time, minThreshold time.Duration) bool {
	d := e.Delta(playbackStarted)
	if d < 0 {
		d = -d
	}
	return d >= minThreshold
}

type TrackSignal struct {
	library.Track
	Status

	// The time when the song Started playing, can change if seek to 0 or using previous song command.
	Started time.Time
	// Did seek to non-zero time in the track.
	HasSeek    bool
	SeekEvents []SeekEvent
}

func (t *TrackSignal) FormatSeekEvents(playbackStarted time.Time) string {
	if playbackStarted.IsZero() {
		return ""
	}

	var sb strings.Builder
	total := len(t.SeekEvents)

	for i, e := range t.SeekEvents {
		offsetSec := e.Offset(playbackStarted).Round(100 * time.Millisecond).Seconds()
		targetSec := e.Target().Round(100 * time.Millisecond).Seconds()
		deltaSec := e.Delta(playbackStarted).Round(100 * time.Millisecond).Seconds()

		if total > 1 {
			fmt.Fprintf(&sb, "\n\t%d: @ +%.1fs -> Seek to %.1fs (Change: %+.1fs)", i+1, offsetSec, targetSec, deltaSec)
		} else {
			fmt.Fprintf(&sb, " @ +%.1fs -> Seek to %.1fs (Change: %+.1fs)\n", offsetSec, targetSec, deltaSec)
		}
	}
	if total > 1 {
		fmt.Fprint(&sb, "\n")
	}

	return sb.String()
}

func (ts *TrackSignal) HasSignificantSeek(playbackStarted time.Time, threshold time.Duration) bool {
	if len(ts.SeekEvents) == 0 || threshold == 0 {
		return true
	}
	for _, e := range ts.SeekEvents {
		if e.IsSignificant(playbackStarted, threshold) {
			return true
		}
	}
	return false
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

	if t.MediaPlayer != tt.MediaPlayer {
		return SwitchedPlayer
	}

	hasNewMetadata := tt.Track.Title != "" || tt.Track.Artist != ""
	if hasNewMetadata {
		if !t.Track.IsSameTrackAs(&tt.Track) {
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
	return fmt.Sprintf("status: %s - started at: %s - seek: %t - %s", t.Status, t.Started, t.HasSeek, &t.Track)
}
