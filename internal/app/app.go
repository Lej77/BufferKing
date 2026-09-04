package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphaelreyna/BufferKing/internal/library"
	"github.com/raphaelreyna/BufferKing/internal/parec"
	"github.com/raphaelreyna/BufferKing/internal/signal"
)

type Conf struct {
	Root                  string
	SaveIncompletePaused  bool
	SaveIncompleteSkipped bool
	SaveIncompleteQuit    bool
	KeepPartials          bool
	AllowedDomains        []string
	AllowNoUrl            bool
	AllowFileUrl          bool
	// ObjectPath points to the dbus object we're listening to.
	// default: /org/mpris/MediaPlayer2
	ObjectPath string
	// Device can be found using `$ pacmd list | grep .monitor`
	// valid device strings look like: alsa_output.pci-0000_00_1f.3.analog-stereo.monitor
	Device string
	Format string
	Color  bool
}

// IsAllowedDomain checks if the URL matches any allowed domains.
// Returns true if no domains are specified (allow all by default).
func (c *Conf) IsAllowedDomain(rawURL string) bool {
	if rawURL == "" {
		return c.AllowNoUrl
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return c.AllowNoUrl
	}

	if parsed.Scheme == "file" {
		return c.AllowFileUrl
	}

	// Strip port numbers from host (e.g., "example.com:8080" -> "example.com")
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return c.AllowNoUrl
	}

	if len(c.AllowedDomains) == 0 {
		return true
	}

	for _, domain := range c.AllowedDomains {
		domain = strings.ToLower(domain)
		// Exact match or proper subdomain match (e.g., api.example.com)
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}

type App struct {
	Conf       *Conf
	Parec      *parec.Parec
	Library    *library.Library
	Listener   *signal.Listener
	SignalChan chan *signal.TrackSignal
}

// LoadConf expects Conf to not be nil
func (a *App) LoadConf() error {
	var err error
	c := a.Conf
	a.Parec = &parec.Parec{
		Root:   c.Root,
		Device: c.Device,
		Format: c.Format,
	}

	a.Library, err = library.LoadLibrary(c.Root)
	if err != nil {
		return err
	}

	a.Listener = &signal.Listener{
		TrackSignals: a.SignalChan,
		ObjectPath:   c.ObjectPath,
		Parser:       *signal.DefaultParser(),
	}
	return nil
}

func (a *App) StartListening(ctx context.Context) error {
	return a.Listener.Start(ctx)
}

func (a *App) finishWJ(wj *parec.WriteJob, saveIncomplete bool, failMsg string) error {
	l := a.Library
	if wj != nil {
		if completed, _ := wj.Completed(); completed {
			err := wj.EmbedMetadata()
			if err != nil {
				return err
			}

			l.Lock()
			l.MarkStored(wj.Track)
			err = l.FileMarkStored(wj.Track, wj.FileName())
			if err != nil {
				l.Unlock()
				return err
			}
			l.Unlock()
			a.Print(colorGreen, CompletedNewRecording, nil)
		} else {
			if saveIncomplete {
				err := wj.EmbedMetadata()
				if err != nil {
					return err
				}

				l.Lock()
				l.MarkStored(wj.Track)
				err = l.FileMarkStored(wj.Track, wj.FileName())
				if err != nil {
					l.Unlock()
					return err
				}
				l.Unlock()
			} else if !a.Conf.KeepPartials {
				path := filepath.Join(a.Conf.Root,
					wj.Track.Artist,
					wj.Track.Album,
					wj.FileName(),
				)
				if err := os.Remove(path); err != nil {
					return err
				}
			} else {
				// If keeping partials then save metadata for them:
				wj.EmbedMetadata()
			}
			a.Print(colorYellow, failMsg, nil)
		}
	}

	return nil
}

const (
	colorReset = "\033[0m"

	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

const (
	CompletedNewRecording = "completed recording new track"
	UnableToCompleteSkip  = "unable to complete recording track due to early track advancement"
	UnableToCompletePause = "unable to complete recording track due to pause"
	UnableToCompleteQuit  = "unable to complete recording track due to exiting BufferKing"

	TrackFoundIgnoring    = "track found in library, ignoring:"
	UrlDisallowedIgnoring = "track from disallowed URL, ignoring:"
	TrackStartedRecording = "started recording new track:"
	TrackUnableToResume   = "unable to resume recording incomplete track due to pause:"
)

type MsgPrinter func()

func (a *App) NewPrinter(color, message string, t *library.Track) MsgPrinter {
	return func() {
		a.Print(color, message, t)
	}
}

func (a *App) Print(color, message string, t *library.Track) {
	var s string
	switch a.Conf.Color {
	case true:
		if t == nil {
			s = fmt.Sprintf("%s%s%s\n\n", color, message, colorReset)
		} else {
			s = fmt.Sprintf("%s%s%s\n%s", color, message, colorReset, t)
		}
	case false:
		if t == nil {
			s = fmt.Sprintf("%s\n\n", message)
		} else {
			s = fmt.Sprintf("%s\n%s", message, t)
		}
	}

	if message == TrackFoundIgnoring {
		s += "\n"
	}

	fmt.Println(s)
}
