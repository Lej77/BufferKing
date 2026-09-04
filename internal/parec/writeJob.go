package parec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/raphaelreyna/BufferKing/internal/library"
)

type WriteJob struct {
	Track *library.Track

	parec *Parec
	cmd   *exec.Cmd

	started time.Time
	stopped time.Time
}

func (wj *WriteJob) Start(ctx context.Context) error {
	p := wj.parec
	wj.started = time.Now()
	if wj.cmd != nil {
		return nil
	}

	p.partsCount += 1

	track := wj.Track
	track.Format = p.Format
	writePath := filepath.Join(p.Root, track.RelPath())

	// Make sure the directory we'll be writing to exists
	dir := filepath.Dir(writePath)
	fileName := wj.FileName()
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	wj.cmd = exec.CommandContext(ctx, "parec",
		"-d", p.Device,
		"--file-format="+p.Format,
		filepath.Join(dir, fileName),
	)

	err = wj.cmd.Start()
	if err != nil {
		return err
	}

	return nil
}

func (wj *WriteJob) Stop() error {
	cmd := wj.cmd
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	err := cmd.Process.Signal(os.Interrupt)
	if err != nil {
		_ = cmd.Process.Kill()
		wj.stopped = time.Now()
		return err
	}

	//Wait for parec to stop
	err = cmd.Wait()
	if err != nil && (strings.Contains(err.Error(), "signal: killed") || strings.Contains(err.Error(), "signal: interrupt")) {
		// ignore error that indicates it was killed early
		err = nil
	}
	wj.stopped = time.Now()
	return err
}

func (wj *WriteJob) Running() bool {
	started := !wj.started.IsZero()
	stopped := !wj.stopped.IsZero()

	return started && !stopped
}

// Completed returns if a track was completely recorded (with some fuzzing ._.)
// The second return value is how long the recording lasted for
func (wj *WriteJob) Completed() (bool, time.Duration) {
	if wj.stopped.IsZero() || wj.started.IsZero() {
		return false, 0
	}

	timeRecorded := wj.stopped.Sub(wj.started)
	dt := timeRecorded - wj.Track.Length

	// If dt >= 0 then at least the entire track was recorded
	// For some reason there is about a 1.5s difference between the recording time and the track length
	// 2.5 seconds should be okay for now ... :(
	return dt >= -2500*time.Millisecond, timeRecorded
}

// FileName gives the filename of the file that this writejob is writing to.
// The file is hidden and includes a part number, formatted as: '.(PART_COUNT)TRACK_NO - TITLE.FORMAT'
func (wj *WriteJob) FileName() string {
	if wj == nil {
		return ""
	}

	t := wj.Track
	t.Format = wj.parec.Format
	return fmt.Sprintf(".(%d)%d - %s.%s", wj.parec.partsCount, t.TrackNumber, t.Title, t.Format)
}

func downloadArt(artURL, dstPath string) error {
	if artURL == "" {
		return fmt.Errorf("artURL is empty")
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(artURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch cover art: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// Embed metadata into written file. Should run just after Stop.
func (wj *WriteJob) EmbedMetadata() error {
	if wj == nil || wj.Track == nil {
		return fmt.Errorf("writejob or track is nil")
	}

	p := wj.parec
	t := wj.Track
	t.Format = p.Format

	// Paths
	dir := filepath.Dir(filepath.Join(p.Root, t.RelPath()))
	originalPath := filepath.Join(dir, wj.FileName())
	tempTaggedPath := filepath.Join(dir, ".tagged_"+wj.FileName())

	// Cover art paths
	tempArtPath := filepath.Join(dir, fmt.Sprintf(".art_%d.jpg", time.Now().UnixNano()))
	albumCoverPath := filepath.Join(dir, "cover.jpg")

	hasCover := false
	if t.ArtURL != "" {
		if err := downloadArt(t.ArtURL, tempArtPath); err == nil {
			hasCover = true
			defer os.Remove(tempArtPath) // Clean up image download when done

			// Save/copy to cover.jpg in the album folder if it doesn't already exist
			if _, err := os.Stat(albumCoverPath); os.IsNotExist(err) {
				_ = copyFile(tempArtPath, albumCoverPath)
			}
		}
	}

	args := []string{"-y", "-i", originalPath}

	// Add cover art input if available
	if hasCover {
		args = append(args, "-i", tempArtPath)
		args = append(args,
			"-map", "0:a", // Use audio from first input (recorded file)
			"-map", "1:v", // Use image from second input (album art)
			"-disposition:v:0", "attached_pic",
		)
	} else {
		args = append(args, "-map", "0:a")
	}

	// Standard Universal Tags
	args = append(args,
		"-metadata", fmt.Sprintf("title=%s", t.Title),
		"-metadata", fmt.Sprintf("artist=%s", t.Artist),
		"-metadata", fmt.Sprintf("album=%s", t.Album),
		"-metadata", fmt.Sprintf("album_artist=%s", t.AlbumArtist),
		"-metadata", fmt.Sprintf("track=%d", t.TrackNumber),
		"-metadata", fmt.Sprintf("disc=%d", t.DiscNumber),
	)

	var comments []string

	if t.URL != "" {
		comments = append(comments, "URL: "+t.URL)
	}
	if t.TrackID != "" {
		comments = append(comments, "TrackId: "+t.TrackID)
	}
	if t.ArtURL != "" {
		comments = append(comments, "Art URL: "+t.ArtURL)
	}

	if len(comments) > 0 {
		args = append(args, "-metadata", fmt.Sprintf("comment=%s", strings.Join(comments, "\n")))
	}

	// Add rating metadata if present (auto rating 0.0 - 1.0 mapped to 0-100)
	if t.AutoRating > 0 && t.AutoRating <= 1 {
		args = append(args, "-metadata", fmt.Sprintf("rating=%d", int(t.AutoRating*100)))
	} else if t.AutoRating > 0 && t.AutoRating <= 100 {
		args = append(args, "-metadata", fmt.Sprintf("rating=%d", int(t.AutoRating)))
	}

	// Direct stream copy (no re-encoding)
	args = append(args, "-c", "copy", tempTaggedPath)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg tagging failed: %w (output: %s)", err, string(output))
	}

	// Replace original file with tagged version
	if err := os.Rename(tempTaggedPath, originalPath); err != nil {
		return fmt.Errorf("failed to finalize tagged file: %w", err)
	}

	return nil
}
