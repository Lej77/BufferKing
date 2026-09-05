package signal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	dbus "github.com/godbus/dbus/v5"
)

const DefaultDebounce = 150 * time.Millisecond

type Listener struct {
	TrackSignals chan<- *TrackSignal
	ObjectPath   string
	Parser
	DebounceDuration     time.Duration
	EmitInstantly        bool
	MediaPlayerWhitelist []string

	conn     *dbus.Conn
	sigChan  chan *dbus.Signal
	stopOnce sync.Once
}

func (l *Listener) Stop() error {
	var err error
	l.stopOnce.Do(func() {
		if l.conn == nil {
			return
		}

		objp := dbus.ObjectPath(l.ObjectPath)
		mopt := dbus.WithMatchObjectPath(objp)
		_ = l.conn.RemoveMatchSignal(mopt)

		if l.sigChan != nil {
			l.conn.RemoveSignal(l.sigChan)
			// DO NOT call close(l.sigChan) - godbus handles this on conn.Close()
		}

		// Close the D-Bus connection (this closes sigChan internally)
		err = l.conn.Close()

		// Safely close output channel last
		if l.TrackSignals != nil {
			close(l.TrackSignals)
		}
	})
	return err
}

func (l *Listener) Start(ctx context.Context) error {
	if l.conn != nil {
		return nil
	}
	var err error
	if err = ctx.Err(); err != nil {
		return err
	}

	l.conn, err = dbus.SessionBus()
	if err != nil {
		return err
	}

	objp := dbus.ObjectPath(l.ObjectPath)
	mopt := dbus.WithMatchObjectPath(objp)
	err = l.conn.AddMatchSignal(mopt)
	if err != nil {
		l.conn.Close()
		return err
	}

	l.sigChan = make(chan *dbus.Signal, 100)
	l.conn.Signal(l.sigChan)

	if l.DebounceDuration == 0 {
		l.DebounceDuration = DefaultDebounce
	}

	go func() {
		defer func() {
			_ = l.Stop()
		}()

		var (
			timer         *time.Timer
			timerChan     <-chan time.Time
			latest        TrackSignal
			prevSender    string
			prevPlayer    string
			hasPending    bool
			isSuppressing bool
		)

		emitSignal := func() {
			out := new(TrackSignal)
			*out = latest
			latest = TrackSignal{}

			select {
			case l.TrackSignals <- out:
			case <-ctx.Done():
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return

			case sig, ok := <-l.sigChan:
				if !ok {
					return
				}

				ts, err := l.Parse(sig)
				// fmt.Println("Raw Signal ", sig, "\n\tparsed as: ", ts, "")
				if err != nil {
					// Log the ignored signal and keep listening
					fmt.Println("failed to parse signal, ignoring it: ", err)
					continue
					// fmt.Println(err)
					// err = l.Stop()
					// if err != nil {
					// 	fmt.Println(err)
					// }
					// break
				} else if ts == nil {
					continue
				}

				// lookup media player name:
				var player string
				if sig.Sender == prevSender {
					player = prevPlayer // cached
				} else if playerName, err := l.ResolvePlayerName(sig.Sender); err == nil {
					player = playerName // human readable
				} else {
					player = sig.Sender // fallback to unique name ":1.123"
				}

				// Cache player name:
				prevSender = sig.Sender
				prevPlayer = player

				if (len(l.MediaPlayerWhitelist) != 0) {
					isAllowed := false
					playerLowerCase := strings.ToLower(player)
					for _, allowedPlayer := range l.MediaPlayerWhitelist {
						allowedPlayer = strings.ToLower(allowedPlayer)
						// Match: "firefox" or "firefox.instance_1_111"
						if allowedPlayer == playerLowerCase || strings.HasPrefix(playerLowerCase, allowedPlayer+".") {
							isAllowed = true
							break
						}
					}
					if !isAllowed {
						continue
					}
				}

				// Every signal should be associated with a media player:
				ts.Track.MediaPlayer = player

				// Merge updates
				if latest.Track.MediaPlayer != "" && latest.Track.MediaPlayer != ts.Track.MediaPlayer && hasPending {
					// can't merge updates from different media players, should be very rare to switch between players
					hasPending = false
					emitSignal()
				}
				latest.Track.MediaPlayer = ts.Track.MediaPlayer
				if ts.Track.Title != "" {
					if latest.Track.IsSameTrackAs(&ts.Track) {
						latest.Track.UpdateTrack(&ts.Track)
					} else {
						latest.Track = ts.Track
					}
				}
				if ts.Status != None {
					latest.Status = ts.Status
				}
				if !ts.Started.IsZero() {
					latest.Started = ts.Started
				}
				if ts.HasSeek || !ts.Started.IsZero() {
					latest.HasSeek = ts.HasSeek
				}

				if !isSuppressing {
					if l.EmitInstantly {
						// Emit instantly on leading edge
						emitSignal()
					} else {
						hasPending = true
					}

					// Start suppression window to absorb burst signals
					isSuppressing = true
					timer = time.NewTimer(l.DebounceDuration)
					timerChan = timer.C
				} else {
					// Buffer intermediate updates while suppressing
					hasPending = true
				}

			case <-timerChan:
				timer = nil
				timerChan = nil
				isSuppressing = false

				// Flush final merged state if new data arrived during suppression
				if hasPending {
					hasPending = false
					emitSignal()
				}
			}
		}
	}()

	return nil
}

// ResolvePlayerName resolves sender bus names like ":1.123" to their MPRIS name like "spotify".
func (l *Listener) ResolvePlayerName(sender string) (string, error) {
	var names []string
	// Ask D-Bus daemon for all active well-known names
	err := l.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		return "", err
	}

	for _, name := range names {
		if strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
			var owner string
			err := l.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner)
			if err == nil && owner == sender {
				// Returns "spotify" from "org.mpris.MediaPlayer2.spotify"
				return strings.TrimPrefix(name, "org.mpris.MediaPlayer2."), nil
			}
		}
	}
	return "", fmt.Errorf("player name not found for sender %s", sender)
}
