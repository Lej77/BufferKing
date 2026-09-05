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
	MediaPlayer string
}

func (t *Track) RelPath() string {
	var ext string
	if t.Format != "" {
		ext = "." + t.Format
	}
	return filepath.Join(t.Artist, t.Album, fmt.Sprintf("%d - %s%s", t.TrackNumber, t.Title, ext))
}

func (t *Track) IsSameTrackAs(other *Track) bool {
	return t != nil && other != nil &&
		t.Title != "" &&
		t.Title == other.Title &&
		t.Artist == other.Artist &&
		t.Album == other.Album
}

func (t *Track) UpdateTrack(other *Track) {
	if t == nil || other == nil {
		return
	}
	if other.Title != "" {
		t.Title = other.Title
	}
	if other.Artist != "" {
		t.Artist = other.Artist
	}
	if other.AlbumArtist != "" {
		t.AlbumArtist = other.AlbumArtist
	}
	if other.Album != "" {
		t.Album = other.Album
	}
	if other.TrackNumber != 0 {
		t.TrackNumber = other.TrackNumber
	}
	if other.DiscNumber != 0 {
		t.DiscNumber = other.DiscNumber
	}
	if other.Length != 0 {
		t.Length = other.Length
	}
	if other.ArtURL != "" {
		t.ArtURL = other.ArtURL
	}
	if other.URL != "" {
		t.URL = other.URL
	}
	if other.TrackID != "" {
		t.TrackID = other.TrackID
	}
	if other.AutoRating > 0 {
		t.AutoRating = other.AutoRating
	}
	if other.Format != "" {
		t.Format = other.Format
	}
	if other.MediaPlayer != "" {
		t.MediaPlayer = other.MediaPlayer
	}
}

func (t *Track) String() string {
	return fmt.Sprintf("Artist: %s\t Album: %s\t Track #%d\t Title: %s\t Length: %s\t Player: %s",
		t.Artist, t.Album, t.TrackNumber, t.Title, t.Length, t.MediaPlayer)
}

func (t *Track) FancyString(enableColors bool) string {
	// ANSI Color & Formatting Constants (empty strings if disabled)
	var reset, bold, dim, cyan, green, yellow, blue string
	if enableColors {
		reset = "\033[0m"
		bold = "\033[1m"
		dim = "\033[2m"
		cyan = "\033[36m"
		green = "\033[32m"
		yellow = "\033[33m"
		blue = "\033[34m"
	}

	// Track / Disc numbers formatting (e.g., "02" or "Disc 1, #02")
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

	// Details as key-value pairs
	type detail struct {
		label string
		value string
		color string
	}

	// Always included details
	details := []detail{
		{
			label: "Track",
			value: fmt.Sprintf("%-12s %sFormat:%s %-6s %sRating:%s %.0f%%",
				trackStr,
				yellow, reset, strings.ToUpper(t.Format),
				yellow, reset, t.AutoRating*100,
			),
			color: yellow,
		},
	}

	// Conditional details
	if t.MediaPlayer != "" {
		details = append(details, detail{
			label: "Media Player",
			value: t.MediaPlayer,
			color: yellow,
		})
	}

	if t.AlbumArtist != "" && t.AlbumArtist != t.Artist {
		details = append(details, detail{
			label: "Album Artist",
			value: t.AlbumArtist,
			color: yellow,
		})
	}

	if t.TrackID != "" {
		details = append(details, detail{
			label: "Track ID",
			value: t.TrackID,
			color: blue,
		})
	}

	if t.URL != "" {
		details = append(details, detail{
			label: "URL",
			value: fmt.Sprintf("%s%s%s", dim, t.URL, reset),
			color: blue,
		})
	}

	if t.ArtURL != "" {
		artStr := t.ArtURL
		if len(artStr) > 80 && strings.HasPrefix(artStr, "data:") {
			artStr = artStr[:50] + "... [base64 truncated]"
		}
		details = append(details, detail{
			label: "Art",
			value: fmt.Sprintf("%s%s%s", dim, artStr, reset),
			color: blue,
		})
	}

	// Build tree output
	for i, d := range details {
		connector := "├─"
		if i == len(details)-1 {
			connector = "└─"
		}

		out += fmt.Sprintf("  %s%s%s %s%s:%s %s\n",
			dim, connector, reset,
			d.color, d.label, reset,
			d.value,
		)
	}

	return strings.TrimSuffix(out, "\n")
}
