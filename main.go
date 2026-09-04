package main

import (
	"context"
	"fmt"
	"os"
	osSignal "os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/Lej77/BufferKing/internal/app"
	"github.com/Lej77/BufferKing/internal/parec"
	"github.com/Lej77/BufferKing/internal/signal"
	flag "github.com/spf13/pflag"
)

// Injected at build time via -ldflags. Defaults to "dev" for local builds.
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func main() {
	// Setup program exit
	retCode := 1
	defer func() { os.Exit(retCode) }()

	// Does the machine have the parec binary for us to use?
	if !parec.Available() {
		fmt.Println("parec or pactl installation not found")
		return
	}

	// Setup and parse flags
	var (
		listFormats bool
		listSources bool
		version     bool
	)
	c, p := userConf(&listFormats, &listSources, &version)

	// Does the user just want some basic info or are we recording?
	if version {
		printVersion()
		retCode = 0
		return
	}
	if listFormats {
		if err := printFormats(); err == nil {
			retCode = 0
		}
		return
	}
	if listSources {
		if err := printSources(); err == nil {
			retCode = 0
		}
		return
	}

	// Make sure user gave a valid path to library
	argsCount := len(os.Args)
	if argsCount < 2 {
		fmt.Println("not enough args, need path to root directory for library")
		return
	}
	info, err := os.Stat(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}
	if !info.IsDir() {
		fmt.Println("need path to root directory for library")
		return
	}
	c.Root = os.Args[1]

	// Grab device that will be our audio source to record from
	if c.Device == "" {
		c.Device, err = source()
		if err != nil {
			fmt.Println("Failed to select audio source: ", err)
			return
		}
	}
	fmt.Printf("Recording audio from device: %s\n", c.Device)

	// Create and configure app
	a := &app.App{
		Conf:       c,
		SignalChan: make(chan *signal.TrackSignal),
	}
	if err := a.LoadConf(); err != nil {
		fmt.Println(err)
		return
	}
	a.Listener.Parser = *p

	// Create context, and listen for kill signal
	ctx, stop := osSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Run bufferking main logic
	if err := a.StartListening(ctx); err != nil && err != context.Canceled {
		fmt.Println(err)
		return
	}

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		fmt.Println("Error:", err)
		return
	}

	retCode = 0
}

func source() (string, error) {
	var device string
	sources, err := parec.Sources()
	if err != nil {
		return "", err
	}

	for index, source := range sources {
		fmt.Printf("%d) %s\n", index, source)
	}

	fmt.Print("Record from which source: ")
	var devIndexString string
	fmt.Scanln(&devIndexString)

	devIndex, err := strconv.Atoi(devIndexString)
	if err != nil {
		return "", err
	}

	if devIndex < len(sources) && 0 <= devIndex {
		device = sources[devIndex]
	} else {
		return "", fmt.Errorf("invalid source choice")
	}
	return device, nil
}

func printVersion() {
	v, c, d := getVersionInfo()

	if c != "none" || d != "unknown" {
		fmt.Printf("bufferking version %s (commit: %s, built: %s)\n", v, c, d)
		return
	}
	fmt.Printf("bufferking version: %s\n", v)
}

func getVersionInfo() (string, string, string) {
	v, c, d := version, commit, date

	// If -ldflags wasn't used, check for version info embedded by `go install`
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}

	// Fall back to tag version from `go install github.com/...@v1.2.3`
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}

	// Fall back to VCS metadata (Git commit & build time embedded by Go 1.18+)
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if c == "none" && len(setting.Value) >= 7 {
				c = setting.Value[:7]
			}
		case "vcs.time":
			if d == "unknown" {
				d = setting.Value
			}
		case "vcs.modified":
			if setting.Value == "true" && c != "none" {
				c += "-dirty"
			}
		}
	}

	return v, c, d
}

func printFormats() error {
	formats, err := parec.Formats()
	if err != nil {
		fmt.Println(err)
		return err
	}

	for _, format := range formats {
		fmt.Println(format)
	}

	return nil
}

func printSources() error {
	sources, err := parec.Sources()
	if err != nil {
		fmt.Println(err)
		return err
	}

	for _, source := range sources {
		fmt.Println(source)
	}

	return nil
}

// userConf parses users flag input into a Conf struct
func userConf(formats, sources, version *bool) (*app.Conf, *signal.Parser) {
	c := app.Conf{}
	flag.StringVarP(&c.Device, "device", "D", "", "Device to record audio from.")
	flag.StringVarP(&c.ObjectPath, "object-path", "o", "/org/mpris/MediaPlayer2", `DBus object path to listen to.`)
	flag.StringVarP(&c.Format, "format", "f", "flac", `Audio format to use when recording.`)
	flag.BoolVarP(&c.SaveIncompleteSkipped, "keep-skipped", "S", false, `Keep incomplete recording due to skipping and mark the track as completed.`)
	flag.BoolVarP(&c.SaveIncompletePaused, "keep-paused", "P", false, `Keep incomplete recording due to pausing and mark the track as completed.`)
	flag.BoolVarP(&c.SaveIncompleteSeek, "keep-after-seek", "E", false, `Keep incomplete recording due to seeking and mark the track as completed.`)
	flag.BoolVarP(&c.SaveIncompleteQuit, "keep-at-quit", "Q", false, `Keep incomplete recording due to exiting BufferKing and mark the track as completed.`)
	flag.BoolVarP(&c.KeepPartials, "keep-partials", "r", false, `Keep partial recording parts.`)
	flag.BoolVarP(&c.Color, "color", "c", false, `Use color coded output.`)
	flag.BoolVar(&c.AllowNoUrl, "allow-no-url", false, `If --allowed-domains is used then this flag can be enabled to also record tracks that don't have any URL, i.e. to record unknown tracks.`)
	flag.BoolVar(&c.AllowFileUrl, "allow-file-url", false, `Record audio for tracks that announce their URL using a "file://" schema, i.e. record local tracks.`)

	flag.BoolVar(formats, "list-formats", false, `List supported audio formats.`)
	flag.BoolVar(sources, "list-sources", false, `List available audio sources to record.`)
	flag.BoolVarP(version, "version", "v", false, `Print current version.`)

	var rawDomains string
	flag.StringVarP(&rawDomains, "allowed-domains", "d", "", "Comma-separated list of allowed domains (e.g., spotify.com,soundcloud.com,youtube.com)")

	pD := signal.DefaultParser()
	p := *signal.DefaultParser()
	flag.StringVar(&p.MetaDataKey, "metadata-key", pD.MetaDataKey, `DBus metadata key`)
	flag.StringVar(&p.TitleKey, "title-key", pD.TitleKey, `DBus title key`)
	flag.StringVar(&p.ArtistKey, "artist-key", pD.ArtistKey, `DBus artist key`)
	flag.StringVar(&p.AlbumKey, "album-key", pD.AlbumKey, `DBus album key`)
	flag.StringVar(&p.AlbumArtistKey, "album-artist-key", pD.AlbumArtistKey, `DBus album artist key`)
	flag.StringVar(&p.TrackNumber, "track-no-key", pD.TrackNumber, `DBus track number key`)
	flag.StringVar(&p.DiscNumber, "disc-no-key", pD.DiscNumber, `DBus disc number key`)
	flag.StringVar(&p.AutoRating, "auto-rating-key", pD.AutoRating, `DBus auto rating key`)
	flag.StringVar(&p.LengthKey, "length-key", pD.LengthKey, `DBus track length key`)
	flag.StringVar(&p.ArtUrlKey, "art-url-key", pD.ArtUrlKey, `DBus art URL key`)
	flag.StringVar(&p.UrlKey, "url-key", pD.UrlKey, `DBus track URL key`)
	flag.StringVar(&p.TrackIdKey, "track-id-key", pD.TrackIdKey, `DBus track id key`)
	flag.StringVar(&p.StatusKey, "status-key", pD.StatusKey, `DBus status key`)
	flag.StringVar(&p.PlayToken, "play-token", pD.PlayToken, `DBus play token`)
	flag.StringVar(&p.PauseToken, "pause-token", pD.PauseToken, `DBus pause token`)

	flag.Parse()

	// Parse rawDomains string into c.AllowedDomains
	if rawDomains != "" {
		domains := strings.Split(rawDomains, ",")
		for _, domain := range domains {
			trimmed := strings.TrimSpace(domain)
			if trimmed != "" {
				c.AllowedDomains = append(c.AllowedDomains, trimmed)
			}
		}
	}

	return &c, &p
}
