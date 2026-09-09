package video

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luispater/gemini-srt-translator-go/pkg/ass"
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
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

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

func TestExtractToASS(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test.ass")

	track := &SubtitleTrack{
		Number:   1,
		Language: "en",
		Name:     "English",
		Codec:    "S_TEXT/ASS",
		Entries: []SubtitleEntry{
			{
				Start:    1 * time.Second,
				End:      3 * time.Second,
				Text:     "0,0,Default,,0,0,0,,Hello, world!",
				Duration: 2 * time.Second,
			},
			{
				Start:    4 * time.Second,
				End:      6 * time.Second,
				Text:     "1,0,Default,,0,0,0,,This is a test.",
				Duration: 2 * time.Second,
			},
		},
	}

	parser := &MKVParser{}
	errExtract := parser.ExtractToASS(track, outputPath)
	if errExtract != nil {
		t.Fatalf("ExtractToASS() failed: %v", errExtract)
	}

	content, errRead := os.ReadFile(outputPath)
	if errRead != nil {
		t.Fatalf("Failed to read output file: %v", errRead)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "[Script Info]") {
		t.Errorf("Expected [Script Info] header in output")
	}
	if !strings.Contains(contentStr, "Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,Hello, world!") {
		t.Errorf("Expected dialogue line not found in output:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,This is a test.") {
		t.Errorf("Expected dialogue line 2 not found in output:\n%s", contentStr)
	}
}

func TestExtractToASS_HeaderWithEventsWithoutFormat(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test.ass")

	headerWithStylesFormat := "[V4+ Styles]\nFormat: Name, Fontname\nStyle: Default,Arial\n\n[Events]\n"
	track := &SubtitleTrack{
		Number:       1,
		Language:     "en",
		Name:         "English",
		Codec:        "S_TEXT/ASS",
		CodecPrivate: []byte(headerWithStylesFormat),
		Entries: []SubtitleEntry{
			{
				Start: 1 * time.Second,
				End:   2 * time.Second,
				Text:  "0,0,Default,,0,0,0,,Sample",
			},
		},
	}

	parser := &MKVParser{}
	if errExtract := parser.ExtractToASS(track, outputPath); errExtract != nil {
		t.Fatalf("ExtractToASS() failed: %v", errExtract)
	}

	content, errRead := os.ReadFile(outputPath)
	if errRead != nil {
		t.Fatalf("Failed to read output file: %v", errRead)
	}

	contentStr := string(content)
	eventsIdx := strings.Index(contentStr, "[Events]")
	if eventsIdx == -1 {
		t.Fatalf("missing [Events] in output")
	}
	if !strings.Contains(contentStr[eventsIdx:], "Format:") {
		t.Errorf("Format: was not added after [Events] when header had styles Format:\n%s", contentStr)
	}
}

func TestExtractToSRT_FromASSTrack(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test.srt")

	track := &SubtitleTrack{
		Number:   1,
		Language: "en",
		Name:     "English",
		Codec:    "S_TEXT/ASS",
		Entries: []SubtitleEntry{
			{
				Start: 1 * time.Second,
				End:   2 * time.Second,
				Text:  "0,0,Default,,0,0,0,,{\\i1}First line{\\i0}\\Nsecond line",
			},
			{
				Start: 3 * time.Second,
				End:   4 * time.Second,
				Text:  "1,0,Default,,0,0,0,,{\\p1}m 0 0 l 10 10{\\p0}",
			},
			{
				Start: 5 * time.Second,
				End:   6 * time.Second,
				Text:  "2,0,Default,,0,0,0,,{\\pos(100,200)}Third line",
			},
			{
				Start: 7 * time.Second,
				End:   8 * time.Second,
				Text:  "3,0,Default,,0,0,0,,{\\p1}m 0 0 l 10 10{\\p0}Fourth mixed line",
			},
			{
				Start: 9 * time.Second,
				End:   10 * time.Second,
				Text:  "4,0,Default,,0,0,0,,Fifth\\hline",
			},
			{
				Start: 11 * time.Second,
				End:   12 * time.Second,
				Text:  "5,0,Default,,0,0,0,,literal [[ASSTAG0]] preserved",
			},
		},
	}

	parser := &MKVParser{}
	if errExtract := parser.ExtractToSRT(track, outputPath); errExtract != nil {
		t.Fatalf("ExtractToSRT() failed: %v", errExtract)
	}

	content, errRead := os.ReadFile(outputPath)
	if errRead != nil {
		t.Fatalf("Failed to read output file: %v", errRead)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "0,0,Default") {
		t.Errorf("ExtractToSRT should strip ASS packet header from text, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, `{\i1}`) || strings.Contains(contentStr, `{\pos(100,200)}`) {
		t.Errorf("ExtractToSRT should strip ASS override tags, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "m 0 0") {
		t.Errorf("ExtractToSRT should skip pure drawing entries, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "First line\nsecond line") {
		t.Errorf("ExtractToSRT should convert \\N to newline, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Third line") {
		t.Errorf("ExtractToSRT missing third line text, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Fourth mixed line") {
		t.Errorf("ExtractToSRT missing fourth line text, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "m 0 0 l 10 10") {
		t.Errorf("ExtractToSRT should strip drawing commands from mixed line, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Fifth line") {
		t.Errorf("ExtractToSRT should convert \\h to space, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "\x00") {
		t.Errorf("ExtractToSRT output contains NUL byte:\n%q", contentStr)
	}
	if !strings.Contains(contentStr, "literal [[ASSTAG0]] preserved") {
		t.Errorf("ExtractToSRT corrupted literal placeholder, got:\n%s", contentStr)
	}
}

func TestExtractToSRT_FromASSTrack_WrapStyle2(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test_wrap2.srt")

	track := &SubtitleTrack{
		Number:       1,
		Language:     "en",
		Name:         "English",
		Codec:        "S_TEXT/ASS",
		CodecPrivate: []byte("[Script Info]\nWrapStyle:2\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"),
		Entries: []SubtitleEntry{
			{
				Start: 1 * time.Second,
				End:   2 * time.Second,
				Text:  "0,0,Default,,0,0,0,,Soft\\nbreak",
			},
		},
	}

	parser := &MKVParser{}
	if errExtract := parser.ExtractToSRT(track, outputPath); errExtract != nil {
		t.Fatalf("ExtractToSRT() failed: %v", errExtract)
	}

	content, errRead := os.ReadFile(outputPath)
	if errRead != nil {
		t.Fatalf("Failed to read output file: %v", errRead)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Soft\nbreak") {
		t.Errorf("expected \\n in WrapStyle 2 to convert to newline, got:\n%s", contentStr)
	}
}

func TestInspectLionessMKV(t *testing.T) {
	mkvPath := "/Volumes/storage/Downloads/upload/mkv/Lioness.2023.S03E03.The.Bear.Is.Infected.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb.mkv"
	if _, errStat := os.Stat(mkvPath); os.IsNotExist(errStat) {
		t.Skip("sample file not found")
	}
	extractedPath, errExtract := ExtractSubtitlesFromMKV(mkvPath, 0)
	if errExtract != nil {
		t.Fatalf("ExtractSubtitlesFromMKV failed: %v", errExtract)
	}
	defer func() {
		if errRemove := os.Remove(extractedPath); errRemove != nil && !os.IsNotExist(errRemove) {
			t.Logf("Failed to clean up temporary file %s: %v", extractedPath, errRemove)
		}
	}()

	if !strings.HasSuffix(extractedPath, ".ass") {
		t.Errorf("expected extracted path to have .ass extension, got %s", extractedPath)
	}

	content, errRead := os.ReadFile(extractedPath)
	if errRead != nil {
		t.Fatalf("failed to read extracted file: %v", errRead)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "[Script Info]") {
		t.Errorf("extracted ASS missing [Script Info]")
	}
	if !strings.Contains(contentStr, "[Events]") {
		t.Errorf("extracted ASS missing [Events]")
	}
	if !strings.Contains(contentStr, "Dialogue: 0,0:00:06.34,0:00:07.46,Default,,0,0,0,,Where is it?") {
		t.Errorf("extracted ASS missing expected first dialogue line")
	}

	parsed, errParse := ass.ParseASS(contentStr)
	if errParse != nil {
		t.Fatalf("ass.ParseASS failed: %v", errParse)
	}

	dialogues := parsed.GetDialogues()
	if len(dialogues) != 853 {
		t.Fatalf("expected 853 dialogues, got %d", len(dialogues))
	}
	t.Logf("Successfully extracted and parsed %d ASS dialogues from Lioness MKV", len(dialogues))
}
