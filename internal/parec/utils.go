package parec

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func Available() bool {
	_, err := exec.LookPath("parec")
	_, err2 := exec.LookPath("pactl")
	return err == nil && err2 == nil
}

func Formats() ([]string, error) {
	formats := map[string]struct{}{}
	cmd := exec.Command("parec", "--list-file-formats")
	outputBytes, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Pull out file extension short form of format from each line
	// this is found in the first column of the output
	buf := bytes.NewBuffer(outputBytes)
	newline := byte('\n')
	for {
		line, err := buf.ReadString(newline)
		eof := err == io.EOF
		if err != nil && !eof {
			return nil, err
		}
		if eof {
			break
		}

		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line from parec: %s", line)
		}

		formats[parts[0]] = struct{}{}
	}

	formatsSlice := []string{}
	for format, _ := range formats {
		formatsSlice = append(formatsSlice, format)
	}
	return formatsSlice, nil
}

func Sources() ([]string, error) {
	// pactl works for both PipeWire and PulseAudio
	cmd := exec.Command("pactl", "list", "sources", "short")
	outputBytes, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	hashmap := map[string]struct{}{}
	lines := strings.Split(string(outputBytes), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		// Format: ID  NAME  MODULE  SAMPLE_SPEC  STATE
		if len(fields) >= 2 {
			sourceName := fields[1]
			// Capture monitor streams or input devices
			if strings.HasSuffix(sourceName, ".monitor") {
				hashmap[sourceName] = struct{}{}
			}
		}
	}

	sources := make([]string, 0, len(hashmap))
	for source := range hashmap {
		sources = append(sources, source)
	}

	return sources, nil
}
