package srt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Subtitle represents a single subtitle entry
type Subtitle struct {
	Index   int           `json:"index"`
	Start   time.Duration `json:"start"`
	End     time.Duration `json:"end"`
	Content string        `json:"content"`
}

// SubtitleObject represents the object structure used for translation
type SubtitleObject struct {
	Index     string  `json:"index"`
	Content   string  `json:"content"`
	TimeStart *string `json:"time_start,omitempty"`
	TimeEnd   *string `json:"time_end,omitempty"`
}

// ParseSRT parses SRT content from a string
func ParseSRT(content string) ([]Subtitle, error) {
	var subtitles []Subtitle

	content = strings.TrimPrefix(content, "\ufeff")

	// Split by empty lines to get individual subtitle blocks
	blocks := regexp.MustCompile(`\r?\n\s*\r?\n`).Split(strings.TrimSpace(content), -1)
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}

		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 3 {
			continue
		}

		// Parse index
		index, errIndex := strconv.Atoi(strings.TrimSpace(lines[0]))
		if errIndex != nil {
			continue
		}

		// Parse timing
		timingLine := strings.TrimSpace(lines[1])
		start, end, errTiming := parseTimestamp(timingLine)
		if errTiming != nil {
			continue
		}

		// Parse content (everything after the second line)
		content = strings.Join(lines[2:], "\n")

		subtitles = append(subtitles, Subtitle{
			Index:   index,
			Start:   start,
			End:     end,
			Content: strings.TrimSpace(content),
		})
	}

	return subtitles, nil
}

// parseTimestamp parses SRT timestamp format "00:00:00,000 --> 00:00:00,000"
func parseTimestamp(line string) (time.Duration, time.Duration, error) {
	parts := strings.Split(line, " --> ")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid timestamp format: %s", line)
	}

	start, errStart := parseDuration(strings.TrimSpace(parts[0]))
	if errStart != nil {
		return 0, 0, errStart
	}

	end, errEnd := parseDuration(strings.TrimSpace(parts[1]))
	if errEnd != nil {
		return 0, 0, errEnd
	}

	return start, end, nil
}

// parseDuration parses SRT duration format "00:00:00,000"
func parseDuration(s string) (time.Duration, error) {
	// Replace comma with dot for milliseconds
	s = strings.Replace(s, ",", ".", 1)

	// Add hours if missing
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	hours, errHours := strconv.Atoi(parts[0])
	if errHours != nil {
		return 0, errHours
	}

	minutes, errMinutes := strconv.Atoi(parts[1])
	if errMinutes != nil {
		return 0, errMinutes
	}

	seconds, errSeconds := strconv.ParseFloat(parts[2], 64)
	if errSeconds != nil {
		return 0, errSeconds
	}

	totalSeconds := float64(hours*3600+minutes*60) + seconds
	return time.Duration(totalSeconds * float64(time.Second)), nil
}

// formatDuration formats duration to SRT format "00:00:00,000"
func formatDuration(d time.Duration) string {
	totalMillis := int64(d / time.Millisecond)
	hours := totalMillis / 3600000
	minutes := (totalMillis % 3600000) / 60000
	seconds := (totalMillis % 60000) / 1000
	millis := totalMillis % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

// ComposeSRT converts subtitles back to SRT format
func ComposeSRT(subtitles []Subtitle) string {
	var parts []string

	for _, sub := range subtitles {
		block := fmt.Sprintf("%d\n%s --> %s\n%s",
			sub.Index,
			formatDuration(sub.Start),
			formatDuration(sub.End),
			sub.Content,
		)
		parts = append(parts, block)
	}

	return strings.Join(parts, "\n\n") + "\n"
}

func ComposeSRTObject(subtitles []SubtitleObject) string {
	var parts []string

	for _, sub := range subtitles {
		block := fmt.Sprintf("%s\n%s --> %s\n%s",
			sub.Index,
			*sub.TimeStart,
			*sub.TimeEnd,
			sub.Content,
		)
		parts = append(parts, block)
	}

	return strings.Join(parts, "\n\n") + "\n"
}
