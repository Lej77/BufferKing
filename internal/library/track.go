package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Track struct {
	Title       string
	Artist      string
	AlbumArtist string
	Album       string
	TrackNumber int32
	DiscNumber  int32
	Length      time.Duration
	ArtURL      string
	URL         string
	TrackID     string
	AutoRating  float64
	Format      string
}

func (t *Track) RelPath() string {
	var ext string
	if t.Format != "" {
		ext = "." + t.Format
	}
	return filepath.Join(t.Artist, t.Album, fmt.Sprintf("%d - %s%s", t.TrackNumber, t.Title, ext))
}

func (t *Track) SimpleString() string {
	return fmt.Sprintf("Artist: %s\t Album: %s\t Track #%d\t Title: %s\t Length: %s",
		t.Artist, t.Album, t.TrackNumber, t.Title, t.Length)
}
func (t *Track) String() string {
	// ANSI Color & Formatting Constants
	const (
		reset   = "\033[0m"
		bold    = "\033[1m"
		dim     = "\033[2m"
		cyan    = "\033[36m"
		green   = "\033[32m"
		magenta = "\033[35m"
		yellow  = "\033[33m"
		blue    = "\033[34m"
	)

	// Format Track / Disc numbers (e.g., "02" or "Disc 1, #02")
	trackStr := fmt.Sprintf("%02d", t.TrackNumber)
	if t.DiscNumber > 0 {
		trackStr = fmt.Sprintf("Disc %d, #%02d", t.DiscNumber, t.TrackNumber)
	}

	// Primary Line: Title, Artist, Album, Length
	out := fmt.Sprintf("%s%s%s%s %sby%s %s%s%s %s[%s]%s %s(%s)%s\n",
		bold, cyan, t.Title, reset,
		dim, reset,
		bold, t.Artist, reset,
		dim, t.Album, reset,
		green, t.Length, reset,
	)

	// Secondary Line: Track/Disc, Format, Rating
	out += fmt.Sprintf("  %s├─%s %sTrack:%s %-12s %sFormat:%s %-6s %sRating:%s %.0f%%\n",
		dim, reset,
		yellow, reset, trackStr,
		yellow, reset, strings.ToUpper(t.Format),
		yellow, reset, t.AutoRating*100,
	)

	// Optional Line: Album Artist (only if different from Track Artist)
	if t.AlbumArtist != "" && t.AlbumArtist != t.Artist {
		out += fmt.Sprintf("  %s├─%s %sAlbum Artist:%s %s\n",
			dim, reset, yellow, reset, t.AlbumArtist)
	}

	// Optional Line: Track ID / Web URL
	if t.URL != "" {
		out += fmt.Sprintf("  %s├─%s %sURL:%s %s%s%s\n",
			dim, reset, blue, reset, dim, t.URL, reset)
	}

	// Optional Line: Album Art URL
	if t.ArtURL != "" {
		artStr := t.ArtURL
		if len(artStr) > 80 && strings.HasPrefix(artStr, "data:") {
			artStr = artStr[:50] + "... [base64 truncated]"
		}
		out += fmt.Sprintf("  %s└─%s %sArt:%s %s%s%s",
			dim, reset, blue, reset, dim, artStr, reset)
	} else {
		// Clean trailing connector if ArtURL is missing
		out = strings.TrimSuffix(out, "\n")
	}

	return out
}
