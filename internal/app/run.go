package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Lej77/BufferKing/internal/library"
	"github.com/Lej77/BufferKing/internal/parec"
	"github.com/Lej77/BufferKing/internal/signal"
)

func (a *App) Run(ctx context.Context) error {
	l := a.Library
	p := a.Parec
	c := a.Conf

	var err error
	var lastTS *signal.TrackSignal

	err = l.Watch(ctx)
	if err != nil {
		fmt.Println(err)
		return err
	}

	for {
		select {
		case <-ctx.Done():
			// Context was canceled (e.g., Ctrl+C was pressed)
			wj, err := p.StopWriteJob()
			if err != nil {
				fmt.Println("Error stopping write job:", err)
			}
			if wj != nil {
				if finishErr := a.finishWJ(wj, c.SaveIncompleteQuit, UnableToCompleteQuit); finishErr != nil {
					fmt.Println(finishErr)
				}
			}
			return ctx.Err()

		case ts, ok := <-a.SignalChan:
			if !ok {
				// SignalChan was closed
				return nil
			}

			diff := lastTS.Compare(ts)
			// fmt.Println("Buffered Signal - Diff is ", diff, " - Info ", ts)
			switch diff {
			case signal.NewTrack, signal.SwitchedPlayer:
				if ts.Started.IsZero() {
					ts.Started = time.Now()
				} else if ts.Title == "" && lastTS != nil && lastTS.MediaPlayerName == ts.MediaPlayerName {
					// Likely did seek to beginning of track, re-use info
					ts.Track = lastTS.Track
				}

				l.Lock()
				stored := l.Stored(&ts.Track)
				l.Unlock()

				var finishedWJ *parec.WriteJob
				var printFunc MsgPrinter
				if ts.Title == "" {
					finishedWJ, err = p.StopWriteJob()
					printFunc = a.NewPrinter(colorYellow, TrackWithoutMetadata, nil)
				} else if stored {
					finishedWJ, err = p.StopWriteJob()
					printFunc = a.NewPrinter(colorCyan, TrackFoundIgnoring, &ts.Track)
				} else if !a.Conf.IsAllowedDomain(ts.Track.URL) {
					finishedWJ, err = p.StopWriteJob()
					printFunc = a.NewPrinter(colorYellow, UrlDisallowedIgnoring, &ts.Track)
				} else {
					finishedWJ, err = p.NewWriteJob(ctx, &ts.Track, true)
					printFunc = a.NewPrinter(colorRed, TrackStartedRecording, &ts.Track)
				}
				if err != nil {
					return err
				}

				go func() {
					var err error
					if diff == signal.SwitchedPlayer {
						err = a.finishWJ(finishedWJ, c.SaveIncompletePlayer, UnableToCompletePlayer)
					} else {
						err = a.finishWJ(finishedWJ, c.SaveIncompleteSkipped, UnableToCompleteSkip)
					}
					if err != nil {
						fmt.Println(err)
					}
					printFunc()
				}()

			case signal.Paused:
				wj, err := p.StopWriteJob()
				if err != nil {
					return err
				}

				go func() {
					if wj != nil {
						err := a.finishWJ(wj, c.SaveIncompletePaused, UnableToCompletePause)
						if err != nil {
							fmt.Println(err)
						}
					}
				}()

			case signal.Resumed:
				var track *library.Track = nil
				if ts != nil && ts.Track.Title != "" {
					track = &ts.Track
				} else if lastTS != nil && lastTS.Track.Title != "" && lastTS.MediaPlayerName == ts.MediaPlayerName {
					track = &lastTS.Track
				}
				if track != nil {
					l.Lock()
					stored := l.Stored(track)
					l.Unlock()
					if !stored && a.Conf.IsAllowedDomain(track.URL) {
						a.Print(colorYellow, TrackUnableToResume, track)
					}
				}

			case signal.Seek:
				wj, err := p.StopWriteJob()
				if err != nil {
					return err
				}

				go func() {
					if wj != nil {
						err := a.finishWJ(wj, c.SaveIncompleteSeek, UnableToCompleteSeek)
						if err != nil {
							fmt.Println(err)
						}
					}
				}()

			case signal.None:
			}

			if diff != signal.NewTrack && lastTS != nil && lastTS.MediaPlayerName == ts.MediaPlayerName {
				// preserve old info except for new changes
				if ts.Track.Title != "" {
					lastTS.Track = ts.Track
				}
				if ts.Status != signal.None {
					lastTS.Status = ts.Status
				}
				if !ts.Started.IsZero() {
					lastTS.Started = ts.Started
				}
				if ts.HasSeek {
					lastTS.HasSeek = true
				}
			} else {
				lastTS = ts
			}
		}
	}
}
