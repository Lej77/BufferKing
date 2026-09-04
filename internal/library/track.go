package library

import (
	"fmt"
	"path/filepath"
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

func (t *Track) String() string {
	return fmt.Sprintf("Artist: %s\t Album: %s\t Track #%d\t Title: %s\t Length: %s",
		t.Artist, t.Album, t.TrackNumber, t.Title, t.Length)
}
