package signal

import (
	"context"
	"fmt"
	"sync"
	"time"

	dbus "github.com/godbus/dbus/v5"
)

const DefaultDebounce = 500 * time.Millisecond

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

		dbst := time.Now() // debounce start time
		for {
			select {
			case <-ctx.Done():
				return // Use return to exit the goroutine cleanly

			case sig, ok := <-l.sigChan:
				if !ok {
					// sigChan was closed by godbus on conn.Close()
					return
				}

				now := time.Now()
				if dt := now.Sub(dbst); dt < l.DebounceDuration {
					continue
				}
				dbst = now

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
				}

				select {
				case l.TrackSignals <- ts:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return nil
}
