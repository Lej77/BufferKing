package signal

import (
	"fmt"
	"time"

	dbus "github.com/godbus/dbus/v5"
	"github.com/raphaelreyna/BufferKing/internal/library"
)

type Parser struct {
	MetaDataKey string

	TitleKey    string
	ArtistKey   string
	AlbumKey    string
	TrackNumber string
	LengthKey   string
	LengthUnit  time.Duration

	StatusKey  string
	PlayToken  string
	PauseToken string
}

func DefaultParser() *Parser {
	return &Parser{
		MetaDataKey: "Metadata",
		TitleKey:    "xesam:title",
		ArtistKey:   "xesam:artist",
		AlbumKey:    "xesam:album",
		TrackNumber: "xesam:trackNumber",
		LengthKey:   "mpris:length",
		LengthUnit:  time.Microsecond,
		StatusKey:   "PlaybackStatus",
		PlayToken:   "Playing",
		PauseToken:  "Paused",
	}
}

func (p *Parser) Parse(sign *dbus.Signal) (*TrackSignal, error) {
	if len(sign.Body) < 2 {
		return nil, fmt.Errorf("signal body too short")
	}

	resp := map[string]dbus.Variant{}
	err := dbus.Store(sign.Body[1:2], &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse signal body: %w", err)
	}

	hasMetadata := false
	hasStatus := false

	// 1. Extract Playback Status if present
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

	// 2. Extract Track Details if Metadata is present
	var track library.Track
	if metaVar, ok := resp[p.MetaDataKey]; ok && metaVar.Value() != nil {
		if md, ok := metaVar.Value().(map[string]dbus.Variant); ok {
			hasMetadata = true

			// fmt.Println("--- DBUS METADATA KEYS ---")
			// for k, v := range md {
			// 	fmt.Printf("Key: %q | Type: %T | Value: %#v\n", k, v.Value(), v.Value())
			// }
			// fmt.Println("--------------------------")

			track.Title, _ = parseString(md, p.TitleKey)
			track.Artist, _ = parseFirstString(md, p.ArtistKey)
			track.Album, _ = parseString(md, p.AlbumKey)
			track.TrackNumber = parseInt32(md, p.TrackNumber)
			lengthMicros := parseInt64(md, p.LengthKey)

			// Fallback to time.Microsecond if p.LengthUnit is unset (0)
			unit := p.LengthUnit
			if unit == 0 {
				unit = time.Microsecond
			}

			track.Length = time.Duration(lengthMicros) * unit
		}
	}

	// If neither status nor metadata changed in this signal, ignore it
	if !hasMetadata && !hasStatus {
		return nil, fmt.Errorf("signal contained neither status nor metadata updates")
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
		if val, ok := v.Value().(int32); ok {
			return val
		}
		if val, ok := v.Value().(int); ok {
			return int32(val)
		}
	}
	return 0
}

func parseInt64(m map[string]dbus.Variant, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}

	val := v.Value()

	// Unwrap nested dbus.Variant wrappers if present
	for {
		if inner, ok := val.(dbus.Variant); ok {
			val = inner.Value()
		} else {
			break
		}
	}

	if val == nil {
		return 0
	}

	switch num := val.(type) {
	case uint64:
		return int64(num)
	case int64:
		return num
	case uint32:
		return int64(num)
	case int32:
		return int64(num)
	case int:
		return int64(num)
	case float64:
		return int64(num)
	}

	return 0
}
