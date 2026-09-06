package parec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Lej77/BufferKing/internal/library"
)

type WriteJob struct {
	Track *library.Track

	parec     *Parec
	parecCmd  *exec.Cmd
	ffmpegCmd *exec.Cmd

	parecStderr  bytes.Buffer
	ffmpegStderr bytes.Buffer

	started time.Time
	stopped time.Time
}

func (wj *WriteJob) StartTime() time.Time {
	return wj.started
}

func (wj *WriteJob) Start(ctx context.Context) error {
	wj.parecStderr.Reset()
	wj.ffmpegStderr.Reset()

	p := wj.parec
	e := p.Encode
	wj.started = time.Now()

	if wj.parecCmd != nil || wj.ffmpegCmd != nil {
		return nil
	}

	p.partsCount += 1

	track := wj.Track
	track.Format = p.Format
	writePath := filepath.Join(p.Root, track.RelPath())

	// Make sure the directory we'll be writing to exists
	dir := filepath.Dir(writePath)
	fileName := wj.FileName()
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	filePath := filepath.Join(dir, fileName)

	parecArgs := []string{
		"-d", p.Device,
		"--format=" + e.ParecFormat,
		fmt.Sprintf("--rate=%d", e.SampleRate),
		fmt.Sprintf("--channels=%d", e.Channels),
	}

	if !e.FfmpegEncode {
		// Direct recording (parec only)
		parecArgs = append(parecArgs, "--file-format="+p.Format, filePath)
		wj.parecCmd = exec.CommandContext(ctx, "parec", parecArgs...)
		wj.parecCmd.Stderr = &wj.parecStderr
		return wj.parecCmd.Start()
	}

	// Re-encoding pipeline (parec -> ffmpeg)
	wj.parecCmd = exec.CommandContext(ctx, "parec", parecArgs...)
	wj.parecCmd.Stderr = &wj.parecStderr

	ffmpegArgs := []string{
		"-y",
		"-f", e.ParecFormat,
		"-ar", strconv.FormatInt(e.SampleRate, 10),
		"-ac", strconv.FormatInt(e.Channels, 10),
		"-i", "pipe:0",
	}

	if e.Bitrate != "" && strings.ToLower(e.Bitrate) != "default" {
		ffmpegArgs = append(ffmpegArgs, "-b:a", e.Bitrate)
	}

	ffmpegArgs = append(ffmpegArgs, filePath)
	wj.ffmpegCmd = exec.CommandContext(ctx, "ffmpeg", ffmpegArgs...)
	wj.ffmpegCmd.Stderr = &wj.ffmpegStderr

	pipeReader, pipeWriter := io.Pipe()
	wj.parecCmd.Stdout = pipeWriter
	wj.ffmpegCmd.Stdin = pipeReader

	if err := wj.parecCmd.Start(); err != nil {
		pipeWriter.Close()
		pipeReader.Close()
		return fmt.Errorf("failed starting parec: %w (stderr: %s)", err, strings.TrimSpace(wj.parecStderr.String()))
	}

	if err := wj.ffmpegCmd.Start(); err != nil {
		_ = wj.parecCmd.Process.Kill()
		pipeWriter.Close()
		pipeReader.Close()
		return fmt.Errorf("failed starting ffmpeg: %w (stderr: %s)", err, strings.TrimSpace(wj.ffmpegStderr.String()))
	}

	// Monitor parec: if it exits (or crashes), close the pipe to signal ffmpeg
	go func() {
		err := wj.parecCmd.Wait()
		if err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("parec failed: %w", err))
		} else {
			_ = pipeWriter.Close() // Normal EOF
		}
	}()

	return nil
}

func (wj *WriteJob) Stop() error {
	if wj.parecCmd == nil || wj.parecCmd.Process == nil {
		return nil
	}

	defer func() { wj.stopped = time.Now() }()

	// Send interrupt signal to parec to stop audio stream cleanly
	if err := wj.parecCmd.Process.Signal(os.Interrupt); err != nil {
		_ = wj.parecCmd.Process.Kill()
	}

	// Wait for parec first (captures root errors like bad audio devices)
	parecErr := wj.parecCmd.Wait()

	// If re-encoding, wait for ffmpeg to finish flushing its buffer
	var ffmpegErr error
	if wj.ffmpegCmd != nil && wj.ffmpegCmd.Process != nil {
		// If parec failed, ensure ffmpeg also exits:
		if parecErr != nil {
			_ = wj.ffmpegCmd.Process.Kill()
		}
		ffmpegErr = wj.ffmpegCmd.Wait()
	}

	if ffmpegErr != nil && !isCleanExit(ffmpegErr) {
		stderr := strings.TrimSpace(wj.ffmpegStderr.String())
		return fmt.Errorf("ffmpeg exited with error: %v | stderr: %s", ffmpegErr, stderr)
	}

	if parecErr != nil && !isCleanExit(parecErr) {
		stderr := strings.TrimSpace(wj.parecStderr.String())
		return fmt.Errorf("parec exited with error: %v | stderr: %s", parecErr, stderr)
	}

	return nil
}

// Helper to check if process termination was intentional via signal
func isCleanExit(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "signal: killed") || strings.Contains(msg, "signal: interrupt")
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
	return fmt.Sprintf(".(%d)%d - %s.%s", wj.parec.partsCount, t.TrackNumber, library.SanitizeFilename(t.Title), t.Format)
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
	if !wj.parec.EmbedMetadata {
		return nil
	}
	if wj == nil || wj.Track == nil {
		return fmt.Errorf("writejob or track is nil")
	}

	p := wj.parec
	t := wj.Track
	t.Format = strings.ToLower(p.Format)

	// Paths
	dir := filepath.Dir(filepath.Join(p.Root, t.RelPath()))
	originalPath := filepath.Join(dir, wj.FileName())
	tempTaggedPath := filepath.Join(dir, ".tagged_"+wj.FileName())

	// Cover art paths
	tempArtPath := filepath.Join(dir, fmt.Sprintf(".art_%d.jpg", time.Now().UnixNano()))
	albumCoverPath := filepath.Join(dir, "cover.jpg")

	// Ogg containers do not support attached_pic streams with stream copy (-c copy).
	supportsAttachedPic := t.Format == "mp3" || t.Format == "m4a" || t.Format == "flac" || t.Format == "mkv"

	hasCover := false
	if t.ArtURL != "" {
		_, coverErr := os.Stat(albumCoverPath)
		missingFolderCover := os.IsNotExist(coverErr)

		// Download art if we need it for embedding or as album cover.jpg
		if missingFolderCover || supportsAttachedPic {
			if err := downloadArt(t.ArtURL, tempArtPath); err == nil {
				hasCover = true
				defer os.Remove(tempArtPath) // Clean up image download when done

				if missingFolderCover {
					_ = copyFile(tempArtPath, albumCoverPath)
				}
			}
		}
	}

	args := []string{"-y", "-i", originalPath}

	// Add cover art input if possible
	if hasCover && supportsAttachedPic {
		args = append(args,
			"-i", tempArtPath,
			"-map", "0:a", // Use audio from first input (recorded file)
			"-map", "1:v", // Use image from second input (album art)
			"-disposition:v:0", "attached_pic",
		)
	} else {
		args = append(args, "-map", "0:a")
	}

	// Standard metadata tags
	args = append(args,
		"-metadata", fmt.Sprintf("title=%s", t.Title),
		"-metadata", fmt.Sprintf("artist=%s", t.Artist),
		"-metadata", fmt.Sprintf("album=%s", t.Album),
		"-metadata", fmt.Sprintf("album_artist=%s", t.AlbumArtist),
		"-metadata", fmt.Sprintf("track=%d", t.TrackNumber),
		"-metadata", fmt.Sprintf("disc=%d", t.DiscNumber),
	)

	// Comments metadata
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

	// Rating metadata (normalized to 0-100 scale)
	if t.AutoRating > 0 {
		rating := t.AutoRating
		if rating <= 1 {
			rating *= 100
		}
		if rating <= 100 {
			args = append(args, "-metadata", fmt.Sprintf("rating=%d", int(rating)))
		}
	}

	// Stream copy to preserve audio quality without re-encoding
	args = append(args, "-c", "copy", tempTaggedPath)

	cmd := exec.Command("ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg tagging failed: %w (output: %s)", err, string(output))
	}

	// Atomic replace of the original file with the tagged version
	if err := os.Rename(tempTaggedPath, originalPath); err != nil {
		return fmt.Errorf("failed to finalize tagged file: %w", err)
	}

	return nil
}
