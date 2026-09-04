package signal

import (
	"context"
	"fmt"
	"sync"
	"time"

	dbus "github.com/godbus/dbus/v5"
	"github.com/raphaelreyna/BufferKing/internal/library"
)

const DefaultDebounce = 50 * time.Millisecond

type Listener struct {
	TrackSignals chan<- *TrackSignal
	ObjectPath   string
	Parser
	DebounceDuration time.Duration

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

	l.sigChan = make(chan *dbus.Signal, cap(l.TrackSignals))
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
			latestTrack   library.Track
			latestStat    Status
			hasPending    bool
			isSuppressing bool
		)

		for {
			select {
			case <-ctx.Done():
				return

			case sig, ok := <-l.sigChan:
				if !ok {
					return
				}

				if sig.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" {
					continue
				}

				ts, err := l.Parse(sig)
				if err != nil {
					// Log the ignored signal and keep listening
					fmt.Println("Ignored signal:", err)
					continue
					// fmt.Println(err)
					// err = l.Stop()
					// if err != nil {
					// 	fmt.Println(err)
					// }
					// break
				} else if ts == nil {
					continue // unrelated change (for example a field such as "CanSeek" or "CanGoNext")
				}

				// Merge updates into state buffer
				if ts.Track.Title != "" {
					latestTrack = ts.Track
				}
				latestStat = ts.Status

				out := &TrackSignal{
					Track:  latestTrack,
					Status: latestStat,
				}

				if !isSuppressing {
					// Emit instantly on leading edge
					select {
					case l.TrackSignals <- out:
					case <-ctx.Done():
						return
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
					out := &TrackSignal{
						Track:  latestTrack,
						Status: latestStat,
					}

					select {
					case l.TrackSignals <- out:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return nil
}
