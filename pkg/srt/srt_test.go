package srt

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseSRT(t *testing.T) {
	content := `1
00:00:01,000 --> 00:00:04,000
Hello world!

2
00:00:05,000 --> 00:00:08,000
This is a test.

3
00:00:09,000 --> 00:00:12,000
Multiple lines
of text here.`

	subtitles, err := ParseSRT(content)
	if err != nil {
		t.Fatalf("ParseSRT failed: %v", err)
	}

	if len(subtitles) != 3 {
		t.Fatalf("Expected 3 subtitles, got %d", len(subtitles))
	}

	// Test first subtitle
	if subtitles[0].Index != 1 {
		t.Errorf("Expected index 1, got %d", subtitles[0].Index)
	}
	if subtitles[0].Content != "Hello world!" {
		t.Errorf("Expected 'Hello world!', got '%s'", subtitles[0].Content)
	}
	if subtitles[0].Start != time.Second {
		t.Errorf("Expected 1s start time, got %v", subtitles[0].Start)
	}
	if subtitles[0].End != 4*time.Second {
		t.Errorf("Expected 4s end time, got %v", subtitles[0].End)
	}

	// Test third subtitle (multi-line)
	if subtitles[2].Content != "Multiple lines\nof text here." {
		t.Errorf("Expected multi-line content, got '%s'", subtitles[2].Content)
	}
}

func TestComposeSRT(t *testing.T) {
	subtitles := []Subtitle{
		{
			Index:   1,
			Start:   time.Second,
			End:     4 * time.Second,
			Content: "Hello world!",
		},
		{
			Index:   2,
			Start:   5 * time.Second,
			End:     8 * time.Second,
			Content: "This is a test.",
		},
	}

	result := ComposeSRT(subtitles)
	expected := "1\n00:00:01,000 --> 00:00:04,000\nHello world!\n\n2\n00:00:05,000 --> 00:00:08,000\nThis is a test.\n"

	if result != expected {
		t.Errorf("ComposeSRT output doesn't match expected.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"00:00:01,000", time.Second},
		{"00:01:00,000", time.Minute},
		{"01:00:00,000", time.Hour},
		{"00:00:00,500", 500 * time.Millisecond},
		{"01:23:45,678", time.Hour + 23*time.Minute + 45*time.Second + 678*time.Millisecond},
	}

	for _, test := range tests {
		result, err := parseDuration(test.input)
		if err != nil {
			t.Errorf("parseDuration(%s) failed: %v", test.input, err)
			continue
		}
		if result != test.expected {
			t.Errorf("parseDuration(%s) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{time.Second, "00:00:01,000"},
		{time.Minute, "00:01:00,000"},
		{time.Hour, "01:00:00,000"},
		{500 * time.Millisecond, "00:00:00,500"},
		{time.Hour + 23*time.Minute + 45*time.Second + 678*time.Millisecond, "01:23:45,678"},
	}

	for _, test := range tests {
		result := formatDuration(test.input)
		if result != test.expected {
			t.Errorf("formatDuration(%v) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestParseSRT_EmptyContent(t *testing.T) {
	subtitles, err := ParseSRT("")
	if err != nil {
		t.Fatalf("ParseSRT with empty content failed: %v", err)
	}
	if len(subtitles) != 0 {
		t.Errorf("Expected 0 subtitles for empty content, got %d", len(subtitles))
	}
}

func TestParseSRT_WithBOM(t *testing.T) {
	content := "\ufeff1\n00:00:01,000 --> 00:00:04,000\nHello world!\n"
	subtitles, err := ParseSRT(content)
	if err != nil {
		t.Fatalf("ParseSRT with BOM failed: %v", err)
	}
	if len(subtitles) != 1 {
		t.Errorf("Expected 1 subtitle, got %d", len(subtitles))
	}
	if subtitles[0].Content != "Hello world!" {
		t.Errorf("Expected 'Hello world!', got '%s'", subtitles[0].Content)
	}
}

func TestParseSRT_InvalidFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "missing timing line",
			content: "1\nHello world!\n",
		},
		{
			name:    "invalid index",
			content: "abc\n00:00:01,000 --> 00:00:04,000\nHello world!\n",
		},
		{
			name:    "invalid timing format",
			content: "1\ninvalid timing\nHello world!\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subtitles, err := ParseSRT(tt.content)
			if err != nil {
				t.Fatalf("ParseSRT should not fail with invalid format: %v", err)
			}
			// Should skip invalid entries and return empty slice
			if len(subtitles) != 0 {
				t.Errorf("Expected 0 subtitles for invalid format, got %d", len(subtitles))
			}
		})
	}
}

func TestParseSRT_WindowsLineEndings(t *testing.T) {
	content := "1\r\n00:00:01,000 --> 00:00:04,000\r\nHello world!\r\n\r\n2\r\n00:00:05,000 --> 00:00:08,000\r\nSecond subtitle\r\n"
	subtitles, err := ParseSRT(content)
	if err != nil {
		t.Fatalf("ParseSRT with Windows line endings failed: %v", err)
	}
	if len(subtitles) != 2 {
		t.Errorf("Expected 2 subtitles, got %d", len(subtitles))
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart time.Duration
		wantEnd   time.Duration
		wantError bool
	}{
		{
			name:      "valid timestamp",
			input:     "00:00:01,000 --> 00:00:04,000",
			wantStart: time.Second,
			wantEnd:   4 * time.Second,
			wantError: false,
		},
		{
			name:      "invalid format missing arrow",
			input:     "00:00:01,000 00:00:04,000",
			wantError: true,
		},
		{
			name:      "invalid duration format",
			input:     "invalid --> 00:00:04,000",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parseTimestamp(tt.input)
			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.wantError {
				if start != tt.wantStart {
					t.Errorf("Expected start %v, got %v", tt.wantStart, start)
				}
				if end != tt.wantEnd {
					t.Errorf("Expected end %v, got %v", tt.wantEnd, end)
				}
			}
		})
	}
}

func TestParseDuration_Errors(t *testing.T) {
	tests := []string{
		"1:2",           // Not enough parts
		"a:b:c",         // Invalid hours
		"1:b:c",         // Invalid minutes
		"1:2:c",         // Invalid seconds
		"1:2:3:4",       // Too many parts
	}

	for _, input := range tests {
		_, err := parseDuration(input)
		if err == nil {
			t.Errorf("Expected error for input %q but got none", input)
		}
	}
}

func TestComposeSRT_Empty(t *testing.T) {
	result := ComposeSRT([]Subtitle{})
	expected := "\n"
	if result != expected {
		t.Errorf("Expected empty result with newline, got %q", result)
	}
}

func TestComposeSRT_SingleSubtitle(t *testing.T) {
	subtitles := []Subtitle{
		{
			Index:   1,
			Start:   time.Second,
			End:     4 * time.Second,
			Content: "Hello world!",
		},
	}

	result := ComposeSRT(subtitles)
	expected := "1\n00:00:01,000 --> 00:00:04,000\nHello world!\n"

	if result != expected {
		t.Errorf("ComposeSRT single subtitle output doesn't match.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestComposeSRT_MultilineContent(t *testing.T) {
	subtitles := []Subtitle{
		{
			Index:   1,
			Start:   time.Second,
			End:     4 * time.Second,
			Content: "Line 1\nLine 2\nLine 3",
		},
	}

	result := ComposeSRT(subtitles)
	if !strings.Contains(result, "Line 1\nLine 2\nLine 3") {
		t.Error("ComposeSRT should preserve multiline content")
	}
}

func TestSubtitleObject(t *testing.T) {
	timeStart := "00:00:01,000"
	timeEnd := "00:00:04,000"
	
	obj := SubtitleObject{
		Index:     "1",
		Content:   "Test content",
		TimeStart: &timeStart,
		TimeEnd:   &timeEnd,
	}

	if obj.Index != "1" {
		t.Errorf("Expected index '1', got %q", obj.Index)
	}
	if obj.Content != "Test content" {
		t.Errorf("Expected content 'Test content', got %q", obj.Content)
	}
	if obj.TimeStart == nil || *obj.TimeStart != timeStart {
		t.Errorf("Expected TimeStart %q, got %v", timeStart, obj.TimeStart)
	}
	if obj.TimeEnd == nil || *obj.TimeEnd != timeEnd {
		t.Errorf("Expected TimeEnd %q, got %v", timeEnd, obj.TimeEnd)
	}
}

func TestComposeSRTObject(t *testing.T) {
	startTime := "00:00:01,000"
	endTime := "00:00:04,000"
	
	objects := []SubtitleObject{
		{
			Index:     "1",
			Content:   "Hello world!",
			TimeStart: &startTime,
			TimeEnd:   &endTime,
		},
	}

	result := ComposeSRTObject(objects)
	expected := "1\n00:00:01,000 --> 00:00:04,000\nHello world!\n"

	if result != expected {
		t.Errorf("ComposeSRTObject output doesn't match.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestComposeSRTObject_Multiple(t *testing.T) {
	startTime1 := "00:00:01,000"
	endTime1 := "00:00:04,000"
	startTime2 := "00:00:05,000"
	endTime2 := "00:00:08,000"
	
	objects := []SubtitleObject{
		{
			Index:     "1",
			Content:   "First subtitle",
			TimeStart: &startTime1,
			TimeEnd:   &endTime1,
		},
		{
			Index:     "2",
			Content:   "Second subtitle",
			TimeStart: &startTime2,
			TimeEnd:   &endTime2,
		},
	}

	result := ComposeSRTObject(objects)
	expected := "1\n00:00:01,000 --> 00:00:04,000\nFirst subtitle\n\n2\n00:00:05,000 --> 00:00:08,000\nSecond subtitle\n"

	if result != expected {
		t.Errorf("ComposeSRTObject multiple objects output doesn't match.\nGot:\n%s\nExpected:\n%s", result, expected)
	}
}

func TestSubtitle_JSONTags(t *testing.T) {
	// Test that Subtitle struct has correct JSON tags
	subtitle := Subtitle{
		Index:   1,
		Start:   time.Second,
		End:     4 * time.Second,
		Content: "Test",
	}

	// This is a simple test to ensure the struct compiles with the expected fields
	if subtitle.Index != 1 {
		t.Error("Index field not accessible")
	}
	if subtitle.Start != time.Second {
		t.Error("Start field not accessible")
	}
	if subtitle.End != 4*time.Second {
		t.Error("End field not accessible")
	}
	if subtitle.Content != "Test" {
		t.Error("Content field not accessible")
	}
}

func TestRoundTripParsing(t *testing.T) {
	// Test that parsing and composing results in the same content
	original := []Subtitle{
		{
			Index:   1,
			Start:   time.Second,
			End:     4 * time.Second,
			Content: "Hello world!",
		},
		{
			Index:   2,
			Start:   5 * time.Second,
			End:     8 * time.Second,
			Content: "Multi-line\ncontent here",
		},
	}

	// Compose to SRT format
	srtContent := ComposeSRT(original)

	// Parse back
	parsed, err := ParseSRT(srtContent)
	if err != nil {
		t.Fatalf("Round-trip parsing failed: %v", err)
	}

	// Compare
	if len(parsed) != len(original) {
		t.Errorf("Expected %d subtitles, got %d", len(original), len(parsed))
	}

	for i := range original {
		if i >= len(parsed) {
			break
		}
		if !reflect.DeepEqual(original[i], parsed[i]) {
			t.Errorf("Subtitle %d doesn't match:\nOriginal: %+v\nParsed: %+v", i, original[i], parsed[i])
		}
	}
}

func TestFormatDuration_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "00:00:00,000",
		},
		{
			name:     "max reasonable duration",
			duration: 99*time.Hour + 59*time.Minute + 59*time.Second + 999*time.Millisecond,
			expected: "99:59:59,999",
		},
		{
			name:     "fractional milliseconds",
			duration: 1500 * time.Microsecond, // 1.5 milliseconds
			expected: "00:00:00,001",         // Should round to 1ms
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %s, expected %s", tt.duration, result, tt.expected)
			}
		})
	}
}