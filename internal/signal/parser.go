package signal

import (
	"fmt"
	"time"

	dbus "github.com/godbus/dbus/v5"
	"github.com/raphaelreyna/BufferKing/internal/library"
)

type Parser struct {
	MetaDataKey string

	TitleKey       string
	ArtistKey      string
	AlbumKey       string
	AlbumArtistKey string
	TrackNumber    string
	DiscNumber     string
	AutoRating     string
	LengthKey      string
	LengthUnit     time.Duration
	ArtUrlKey      string
	UrlKey         string
	TrackIdKey     string

	StatusKey  string
	PlayToken  string
	PauseToken string
}

func DefaultParser() *Parser {
	return &Parser{
		MetaDataKey:    "Metadata",
		TitleKey:       "xesam:title",
		ArtistKey:      "xesam:artist",
		AlbumKey:       "xesam:album",
		AlbumArtistKey: "xesam:albumArtist",
		TrackNumber:    "xesam:trackNumber",
		DiscNumber:     "xesam:discNumber",
		AutoRating:     "xesam:autoRating",
		LengthKey:      "mpris:length",
		LengthUnit:     time.Microsecond,
		ArtUrlKey:      "mpris:artUrl",
		UrlKey:         "xesam:url",
		TrackIdKey:     "mpris:trackid",
		StatusKey:      "PlaybackStatus",
		PlayToken:      "Playing",
		PauseToken:     "Paused",
	}
}

func (p *Parser) Parse(sign *dbus.Signal) (*TrackSignal, error) {
	if sign.Name == "org.mpris.MediaPlayer2.Player.previous" {
		return &TrackSignal{HasSeek: false, Started: time.Now()}, nil
	}
	if sign.Name == "org.mpris.MediaPlayer2.Player.Seeked" {
		if len(sign.Body) > 0 {
			seek := castToInt64(sign.Body[0])
			if seek == 0 {
				return &TrackSignal{HasSeek: false, Started: time.Now()}, nil
			}
		}
		return &TrackSignal{HasSeek: true}, nil
	}
	if sign.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" {
		// ignore other signals
		return nil, nil
	}
	if len(sign.Body) < 2 {
		if len(sign.Body) == 1 {
			return nil, fmt.Errorf("signal body too short, name=%s, first item in body=%s", sign.Name, sign.Body[0])
		}
		return nil, fmt.Errorf("signal body too short, name=%s", sign.Name)
	}

	resp := map[string]dbus.Variant{}
	err := dbus.Store(sign.Body[1:2], &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse signal body: %w", err)
	}

	hasMetadata := false
	hasStatus := false

	// Extract Playback Status if present
	var stat Status = Play
	if statusVar, ok := resp[p.StatusKey]; ok && statusVar.Value() != nil {
		if sstat, ok := statusVar.Value().(string); ok {
			hasStatus = true
			switch sstat {
			case p.PlayToken:
				stat = Play
			case p.PauseToken:
				stat = Pause
			}
		}
	}

	// Extract Track Details if Metadata is present
	var track library.Track
	if metaVar, ok := resp[p.MetaDataKey]; ok && metaVar.Value() != nil {
		if md, ok := metaVar.Value().(map[string]dbus.Variant); ok {
			hasMetadata = true
			// fmt.Println("--- DBUS METADATA KEYS ---")
			// for k, v := range md {
			// 	fmt.Printf("Key: %q | Type: %T | Value: %#v\n", k, v.Value(), v.Value())
			// }
			// fmt.Println("--------------------------")

			// Basic Tags
			track.Title, _ = parseString(md, p.TitleKey)
			track.Artist, _ = parseFirstString(md, p.ArtistKey)
			track.AlbumArtist, _ = parseFirstString(md, p.AlbumArtistKey)
			track.Album, _ = parseString(md, p.AlbumKey)

			// Numbers
			track.TrackNumber = parseInt32(md, p.TrackNumber)
			track.DiscNumber = parseInt32(md, p.DiscNumber)
			track.AutoRating = parseFloat64(md, p.AutoRating)

			// URLs & IDs
			track.ArtURL, _ = parseString(md, p.ArtUrlKey)
			track.URL, _ = parseString(md, p.UrlKey)
			track.TrackID, _ = parseString(md, p.TrackIdKey)

			// Duration
			track.Length = time.Duration(parseInt64(md, p.LengthKey)) * p.LengthUnit
		}
	}

	// If PropertiesChanged but neither status nor metadata changed in this signal, ignore it
	if !hasMetadata && !hasStatus {
		// unrelated change (for example a field such as "CanSeek" or "CanGoNext")
		return nil, nil
	}

	return &TrackSignal{
		Track:  track,
		Status: stat,
	}, nil
}

// Helpers that return errors for missing required fields

func parseString(m map[string]dbus.Variant, key string) (string, error) {
	v, ok := m[key]
	if !ok || v.Value() == nil {
		return "", fmt.Errorf("key %q not found", key)
	}
	str, ok := v.Value().(string)
	if !ok {
		return "", fmt.Errorf("key %q is type %T, expected string", key, v.Value())
	}
	if str == "" {
		return "", fmt.Errorf("key %q is empty", key)
	}
	return str, nil
}

func parseFirstString(m map[string]dbus.Variant, key string) (string, error) {
	v, ok := m[key]
	if !ok || v.Value() == nil {
		return "", fmt.Errorf("key %q not found", key)
	}

	switch val := v.Value().(type) {
	case []string:
		if len(val) == 0 || val[0] == "" {
			return "", fmt.Errorf("key %q array is empty", key)
		}
		return val[0], nil
	case string:
		if val == "" {
			return "", fmt.Errorf("key %q is empty", key)
		}
		return val, nil
	default:
		return "", fmt.Errorf("key %q is type %T, expected []string or string", key, v.Value())
	}
}

func parseInt32(m map[string]dbus.Variant, key string) int32 {
	if v, ok := m[key]; ok && v.Value() != nil {
		switch num := v.Value().(type) {
		case int32:
			return num
		case int:
			return int32(num)
		case uint32:
			return int32(num)
		case int64:
			return int32(num)
		}
	}
	return 0
}

func parseFloat64(m map[string]dbus.Variant, key string) float64 {
	if v, ok := m[key]; ok && v.Value() != nil {
		if val, ok := v.Value().(float64); ok {
			return val
		}
	}
	return 0.0
}

func parseInt64(m map[string]dbus.Variant, key string) int64 {
	if v, ok := m[key]; ok {
		return castToInt64(v.Value())
	}
	return 0
}

func castToInt64(value any) int64 {
	if value != nil {
		switch num := value.(type) {
		case uint64:
			return int64(num)
		case int64:
			return num
		case int32:
			return int64(num)
		case uint32:
			return int64(num)
		case int:
			return int64(num)
		}
	}
	return 0
}
