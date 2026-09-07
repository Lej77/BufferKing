package library

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Artist struct {
	Name   string
	Albums map[string]*Album
}

type Album struct {
	Name string
	// Tracks holds track titles as keys for fast lookups
	// Track titles are formated as 'TrackNo - TrackTitle', see GetTrackKey method
	Tracks map[string]struct{}
}

func NewAlbum(albumName, trackName string) *Album {
	return &Album{
		Name: albumName,
		Tracks: map[string]struct{}{
			trackName: {},
		},
	}
}

type Library struct {
	Root    string
	Artists map[string]*Artist
	sync.Mutex
}

func (l *Library) GetArtistKey(t *Track) string {
	return SanitizeFilename(t.Artist)
}
func (l *Library) GetAlbumKey(t *Track) string {
	return SanitizeFilename(t.Album)
}
func (l *Library) GetTrackKey(t *Track) string {
	return SanitizeFilename(fmt.Sprintf("%d - %s", t.TrackNumber, t.Title))
}

func (l *Library) Stored(t *Track) bool {
	artists, ok := l.Artists[l.GetArtistKey(t)]
	if !ok {
		return false
	}
	albums, ok := artists.Albums[l.GetAlbumKey(t)]
	if !ok {
		return false
	}
	_, ok = albums.Tracks[l.GetTrackKey(t)]
	return ok
}

func (l *Library) MarkStored(t *Track) {
	var ok bool
	artistKey := l.GetArtistKey(t)
	albumKey := l.GetAlbumKey(t)
	titleKey := l.GetTrackKey(t)

	// Does artist exist?
	var artist *Artist
	if artist, ok = l.Artists[artistKey]; !ok {
		album := NewAlbum(albumKey, titleKey)

		l.Artists[artistKey] = &Artist{
			Name:   artistKey,
			Albums: map[string]*Album{artistKey: album},
		}

		return
	}

	// Does album exist?
	var album *Album
	if album, ok = artist.Albums[albumKey]; !ok {
		artist.Albums[albumKey] = NewAlbum(albumKey, titleKey)
		return
	}

	// Does track exist?
	if _, ok = album.Tracks[titleKey]; !ok {
		album.Tracks[titleKey] = struct{}{}
		return
	}
}

func (l *Library) UnmarkStored(t *Track) {
	var ok bool

	// Does artist exist?
	var artist *Artist
	if artist, ok = l.Artists[l.GetArtistKey(t)]; !ok {
		return
	}

	// Does album exist?
	var album *Album
	if album, ok = artist.Albums[l.GetAlbumKey(t)]; !ok {
		return
	}

	delete(album.Tracks, l.GetTrackKey(t))
}

// Unhide the file now that its finished
func (l *Library) FileMarkStored(t *Track, filename string) error {
	newPath := filepath.Join(l.Root, t.RelPath())

	dir := filepath.Dir(newPath)
	oldPath := filepath.Join(dir, filename)

	return os.Rename(oldPath, newPath)
}

func LoadLibrary(root string) (*Library, error) {
	l := &Library{Root: root, Artists: map[string]*Artist{}}
	re := regexp.MustCompile(`^([0-9]+) \- (.+)\.([0-9A-Za-z]+)$`)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}

		dirs := strings.Split(relPath, "/")
		if len(dirs) != 3 {
			// return fmt.Errorf("invalid path: %s %+v", relPath, dirs)
			return nil
		}
		// 1 -> TrackNumber; 2 -> Title; 3-> Format
		titleParts := re.FindStringSubmatch(dirs[2])

		if len(titleParts) < 3 {
			return nil
		}
		tn, err := strconv.Atoi(titleParts[1])
		if err != nil {
			return err
		}

		track := &Track{
			Artist:      dirs[0],
			Album:       dirs[1],
			Title:       titleParts[2],
			TrackNumber: int32(tn),
		}

		l.MarkStored(track)

		return nil
	})

	return l, err
}

func (l *Library) String() string {
	var s string

	newline := ""

	for _, artist := range l.Artists {
		s += newline
		s += fmt.Sprintf("Artist: %s\n", artist.Name)
		for _, album := range artist.Albums {
			s += fmt.Sprintf("\tAlbum: %s\n", album.Name)
			for track, _ := range album.Tracks {
				s += "\t\t" + track + "\n"
			}
		}
		newline = "\n"
	}

	return s
}
