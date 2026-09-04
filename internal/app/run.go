package app

import (
	"context"
	"fmt"
	"github.com/raphaelreyna/BufferKing/internal/parec"
	"github.com/raphaelreyna/BufferKing/internal/signal"
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

	for ts := range a.SignalChan {
		diff := lastTS.Compare(ts)
		// fmt.Printf("Diff: %s\t Signal: %s\n", diff, ts)
		switch diff {
		case signal.NewTrack:
			l.Lock()
			stored := l.Stored(&ts.Track)
			l.Unlock()

			var finishedWJ *parec.WriteJob
			var printFunc MsgPrinter
			if stored {
				finishedWJ, err = p.StopWriteJob()
				printFunc = a.NewPrinter(colorCyan, TrackFoundIgnoring, &ts.Track)
			} else {
				finishedWJ, err = p.NewWriteJob(context.TODO(), &ts.Track, true)
				printFunc = a.NewPrinter(colorRed, TrackStartedRecording, &ts.Track)
			}
			if err != nil {
				return err
			}

			go func() {
				err := a.finishWJ(finishedWJ, c.SaveIncompletesSkipped, UnableToCompleteSkip)
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
					err := a.finishWJ(wj, c.SaveIncompletesPaused, UnableToCompletePause)
					if err != nil {
						fmt.Println(err)
					}
				}
			}()
		case signal.Resumed:
			l.Lock()
			stored := l.Stored(&lastTS.Track)
			l.Unlock()
			if !stored {
				a.Print(colorYellow, TrackUnableToResume, &lastTS.Track)
			}
		case signal.None:
		}

		if ts.Title != "" {
			lastTS = ts
		} else {
			lastTS.Status = ts.Status
		}
	}

	return err
}
