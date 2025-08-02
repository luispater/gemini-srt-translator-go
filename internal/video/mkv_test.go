package video

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsEnglishTrack(t *testing.T) {
	testCases := []struct {
		language string
		expected bool
	}{
		{"en", true},
		{"eng", true},
		{"english", true},
		{"EN", true},
		{"ENG", true},
		{"ENGLISH", true},
		{"fr", false},
		{"es", false},
		{"", false},
	}

	for _, tc := range testCases {
		result := isEnglishTrack(tc.language)
		if result != tc.expected {
			t.Errorf("isEnglishTrack(%q) = %v, expected %v", tc.language, result, tc.expected)
		}
	}
}

func TestIsSDHTrack(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"English SDH", true},
		{"English (SDH)", true},
		{"English - SDH", true},
		{"SDH", true},
		{"English CC", true},
		{"Closed Caption", true},
		{"English - Deaf and Hard of hearing", true},
		{"English", false},
		{"Regular subtitles", false},
		{"", false},
	}

	for _, tc := range testCases {
		result := isSDHTrack(tc.name)
		if result != tc.expected {
			t.Errorf("isSDHTrack(%q) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

func TestSelectBestEnglishTrack(t *testing.T) {
	parser := &MKVParser{
		tracks: []SubtitleTrack{
			{Number: 1, Language: "en", Name: "English SDH", Codec: "S_TEXT/UTF8"},
			{Number: 2, Language: "en", Name: "English", Codec: "S_TEXT/UTF8"},
			{Number: 3, Language: "fr", Name: "French", Codec: "S_TEXT/UTF8"},
		},
	}

	track, err := parser.SelectBestEnglishTrack()
	if err != nil {
		t.Fatalf("SelectBestEnglishTrack() failed: %v", err)
	}

	// Should select the non-SDH English track
	if track.Number != 2 {
		t.Errorf("Expected track number 2, got %d", track.Number)
	}
	if track.Name != "English" {
		t.Errorf("Expected track name 'English', got %q", track.Name)
	}
}

func TestSelectBestEnglishTrack_OnlySDH(t *testing.T) {
	parser := &MKVParser{
		tracks: []SubtitleTrack{
			{Number: 1, Language: "en", Name: "English SDH", Codec: "S_TEXT/UTF8"},
			{Number: 3, Language: "fr", Name: "French", Codec: "S_TEXT/UTF8"},
		},
	}

	track, err := parser.SelectBestEnglishTrack()
	if err != nil {
		t.Fatalf("SelectBestEnglishTrack() failed: %v", err)
	}

	// Should select the SDH track when it's the only English option
	if track.Number != 1 {
		t.Errorf("Expected track number 1, got %d", track.Number)
	}
}

func TestExtractToSRT(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "mkv_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	outputPath := filepath.Join(tempDir, "test.srt")

	// Create a test track with subtitle entries
	track := &SubtitleTrack{
		Number:   1,
		Language: "en",
		Name:     "English",
		Codec:    "S_TEXT/UTF8",
		Entries: []SubtitleEntry{
			{
				Start:    1 * time.Second,
				End:      3 * time.Second,
				Text:     "Hello, world!",
				Duration: 2 * time.Second,
			},
			{
				Start:    4 * time.Second,
				End:      6 * time.Second,
				Text:     "This is a test.",
				Duration: 2 * time.Second,
			},
		},
	}

	parser := &MKVParser{}
	err = parser.ExtractToSRT(track, outputPath)
	if err != nil {
		t.Fatalf("ExtractToSRT() failed: %v", err)
	}

	// Verify the file was created
	if _, err = os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("Output file was not created: %s", outputPath)
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	expectedContent := "1\n00:00:01,000 --> 00:00:03,000\nHello, world!\n\n2\n00:00:04,000 --> 00:00:06,000\nThis is a test.\n"
	if string(content) != expectedContent {
		t.Errorf("Output content mismatch\nExpected:\n%q\nGot:\n%q", expectedContent, string(content))
	}
}
