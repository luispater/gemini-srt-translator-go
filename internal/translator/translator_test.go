package translator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

func TestNewTranslator(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		wantPath string
	}{
		{
			name: "with input file",
			cfg: &config.Config{
				InputFile: "/path/to/test.srt",
			},
			wantPath: "/path/to/test_translated.srt",
		},
		{
			name: "with custom output file",
			cfg: &config.Config{
				InputFile:  "/path/to/test.srt",
				OutputFile: "/custom/output.srt",
			},
			wantPath: "/custom/output.srt",
		},
		{
			name:     "without input file",
			cfg:      &config.Config{},
			wantPath: "translated.srt",
		},
		{
			name: "with API keys",
			cfg: &config.Config{
				GeminiAPIKeys: []string{"key1", "key2"},
				InputFile:     "test.srt",
			},
			wantPath: "test_translated.srt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.cfg)

			if translator.config != tt.cfg {
				t.Error("Expected config to be set correctly")
			}
			if translator.outputFile != tt.wantPath {
				t.Errorf("Expected output file to be %q, got %q", tt.wantPath, translator.outputFile)
			}
			if translator.currentAPIIndex != 0 {
				t.Error("Expected currentAPIIndex to be 0")
			}
			if translator.batchNumber != 1 {
				t.Error("Expected batchNumber to be 1")
			}
			if len(tt.cfg.GeminiAPIKeys) > 0 && len(translator.apiKeys) != len(tt.cfg.GeminiAPIKeys) {
				t.Errorf("Expected %d API keys, got %d", len(tt.cfg.GeminiAPIKeys), len(translator.apiKeys))
			}
		})
	}
}

func TestTranslator_getCurrentAPIKey(t *testing.T) {
	tests := []struct {
		name            string
		apiKeys         []string
		currentAPIIndex int
		want            string
	}{
		{
			name:            "valid first key",
			apiKeys:         []string{"key1", "key2"},
			currentAPIIndex: 0,
			want:            "key1",
		},
		{
			name:            "valid second key",
			apiKeys:         []string{"key1", "key2"},
			currentAPIIndex: 1,
			want:            "key2",
		},
		{
			name:            "no keys",
			apiKeys:         []string{},
			currentAPIIndex: 0,
			want:            "",
		},
		{
			name:            "index out of range",
			apiKeys:         []string{"key1"},
			currentAPIIndex: 5,
			want:            "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := &Translator{
				apiKeys:         tt.apiKeys,
				currentAPIIndex: tt.currentAPIIndex,
			}

			got := translator.getCurrentAPIKey()
			if got != tt.want {
				t.Errorf("getCurrentAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslator_hasAPIKeys(t *testing.T) {
	tests := []struct {
		name    string
		apiKeys []string
		want    bool
	}{
		{
			name:    "has keys",
			apiKeys: []string{"key1", "key2"},
			want:    true,
		},
		{
			name:    "no keys",
			apiKeys: []string{},
			want:    false,
		},
		{
			name:    "nil keys",
			apiKeys: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := &Translator{
				apiKeys: tt.apiKeys,
			}

			got := translator.hasAPIKeys()
			if got != tt.want {
				t.Errorf("hasAPIKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranslator_validatePrerequisites(t *testing.T) {
	tests := []struct {
		name           string
		apiKeys        []string
		targetLanguage string
		wantError      bool
		errorMsg       string
	}{
		{
			name:           "valid prerequisites",
			apiKeys:        []string{"key1"},
			targetLanguage: "French",
			wantError:      false,
		},
		{
			name:           "no API keys",
			apiKeys:        []string{},
			targetLanguage: "French",
			wantError:      true,
			errorMsg:       "please provide a valid Gemini API key",
		},
		{
			name:      "no target language",
			apiKeys:   []string{"key1"},
			wantError: true,
			errorMsg:  "please provide a target language",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := &Translator{
				config: &config.Config{
					TargetLanguage: tt.targetLanguage,
				},
				apiKeys: tt.apiKeys,
			}

			err := translator.validatePrerequisites()
			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.wantError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error message to contain %q, got %q", tt.errorMsg, err.Error())
			}
		})
	}
}

func TestTranslator_validateConfig(t *testing.T) {
	// Create temporary test file for valid file tests
	tempFile, err := os.CreateTemp("", "test*.srt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	// Write some test content
	_, err = tempFile.WriteString("1\n00:00:01,000 --> 00:00:02,000\nTest subtitle\n\n")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		cfg       *config.Config
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 1000,
			},
			wantError: false,
		},
		{
			name: "no input file",
			cfg: &config.Config{
				ThinkingBudget: 1000,
			},
			wantError: true,
			errorMsg:  "please provide a subtitle file",
		},
		{
			name: "non-existent file",
			cfg: &config.Config{
				InputFile:      "/non/existent/file.srt",
				ThinkingBudget: 1000,
			},
			wantError: true,
			errorMsg:  "does not exist",
		},
		{
			name: "invalid thinking budget negative",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: -1,
			},
			wantError: true,
			errorMsg:  "thinking budget must be between 0 and 24576",
		},
		{
			name: "invalid thinking budget too large",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 25000,
			},
			wantError: true,
			errorMsg:  "thinking budget must be between 0 and 24576",
		},
		{
			name: "invalid temperature negative",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 1000,
				Temperature:    floatPtr(-0.1),
			},
			wantError: true,
			errorMsg:  "temperature must be between 0.0 and 2.0",
		},
		{
			name: "invalid temperature too high",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 1000,
				Temperature:    floatPtr(2.1),
			},
			wantError: true,
			errorMsg:  "temperature must be between 0.0 and 2.0",
		},
		{
			name: "invalid top P negative",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 1000,
				TopP:           floatPtr(-0.1),
			},
			wantError: true,
			errorMsg:  "top P must be between 0.0 and 1.0",
		},
		{
			name: "invalid top P too high",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 1000,
				TopP:           floatPtr(1.1),
			},
			wantError: true,
			errorMsg:  "top P must be between 0.0 and 1.0",
		},
		{
			name: "invalid top K negative",
			cfg: &config.Config{
				InputFile:      tempFile.Name(),
				ThinkingBudget: 1000,
				TopK:           floatPtr(-1),
			},
			wantError: true,
			errorMsg:  "top K must be a non-negative integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := &Translator{
				config: tt.cfg,
			}

			err := translator.validateConfig()
			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.wantError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error message to contain %q, got %q", tt.errorMsg, err.Error())
			}
		})
	}
}

func TestTranslator_saveProgress(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "translator_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	progressFile := filepath.Join(tempDir, "test.progress")
	outputFile := filepath.Join(tempDir, "test_output.srt")

	translator := &Translator{
		config: &config.Config{
			InputFile: "/path/to/input.srt",
		},
		progressFile: progressFile,
		outputFile:   outputFile,
	}

	subtitles := []srt.Subtitle{
		{
			Index:   1,
			Start:   1000000000, // 1 second in nanoseconds
			End:     2000000000,
			Content: "Translated content 1",
		},
		{
			Index:   2,
			Start:   3000000000,
			End:     4000000000,
			Content: "Translated content 2",
		},
	}

	// Save progress
	translator.saveProgress(2, subtitles)

	// Check progress file exists and has correct content
	progressData, err := os.ReadFile(progressFile)
	if err != nil {
		t.Fatalf("Failed to read progress file: %v", err)
	}

	var progress ProgressInfo
	if err = json.Unmarshal(progressData, &progress); err != nil {
		t.Fatalf("Failed to parse progress JSON: %v", err)
	}

	if progress.Line != 2 {
		t.Errorf("Expected progress line to be 2, got %d", progress.Line)
	}
	if progress.InputFile != "/path/to/input.srt" {
		t.Errorf("Expected input file to be '/path/to/input.srt', got %q", progress.InputFile)
	}

	// Check output file exists
	if _, err = os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected output file to be created")
	}
}

func TestTranslator_processTranslatedLines(t *testing.T) {
	translator := &Translator{}

	// Create test data
	translatedSubtitles := []srt.Subtitle{
		{Index: 1, Content: ""},
		{Index: 2, Content: ""},
		{Index: 3, Content: ""},
	}

	batch := []srt.SubtitleObject{
		{Index: "0", Content: "Original content 1"},
		{Index: "1", Content: "Original content 2"},
		{Index: "2", Content: "Original content 3"},
	}

	translatedLines := []srt.SubtitleObject{
		{Index: "0", Content: "Translated content 1"},
		{Index: "1", Content: "Translated content 2"},
		{Index: "2", Content: "Translated content 3"},
	}

	err := translator.processTranslatedLines(translatedLines, translatedSubtitles, batch)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that subtitles were updated correctly
	expectedContents := []string{"Translated content 1", "Translated content 2", "Translated content 3"}
	for i, expected := range expectedContents {
		if translatedSubtitles[i].Content != expected {
			t.Errorf("Expected subtitle %d content to be %q, got %q", i, expected, translatedSubtitles[i].Content)
		}
	}
}

func TestTranslator_validateTranslatedResponse_MismatchedCount(t *testing.T) {
	translator := &Translator{}

	batch := []srt.SubtitleObject{{Index: "0", Content: "Original"}}
	translatedLines := []srt.SubtitleObject{
		{Index: "0", Content: "Translated 1"},
		{Index: "1", Content: "Translated 2"}, // Extra line
	}

	err := translator.validateTranslatedResponse(translatedLines, batch)
	if err == nil {
		t.Error("Expected error for mismatched line count")
	}
	if !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("Expected error about unexpected response, got: %v", err)
	}
}

func TestTranslator_validateTranslatedResponse_EmptyTranslation(t *testing.T) {
	translator := &Translator{}

	batch := []srt.SubtitleObject{{Index: "0", Content: "Original content"}}
	translatedLines := []srt.SubtitleObject{{Index: "0", Content: ""}} // Empty translation

	err := translator.validateTranslatedResponse(translatedLines, batch)
	if err == nil {
		t.Error("Expected error for empty translation")
	}
	if !strings.Contains(err.Error(), "empty translation") {
		t.Errorf("Expected error about empty translation, got: %v", err)
	}
}

func TestTranslator_processTranslatedLines_MismatchedCount(t *testing.T) {
	translator := &Translator{}

	translatedSubtitles := []srt.Subtitle{{Index: 1}, {Index: 2}}
	batch := []srt.SubtitleObject{{Index: "0", Content: "Original"}}
	translatedLines := []srt.SubtitleObject{
		{Index: "0", Content: "Translated 1"},
		{Index: "1", Content: "Translated 2"}, // Extra line
	}

	// This should now succeed as validation is separate
	err := translator.processTranslatedLines(translatedLines, translatedSubtitles, batch)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestTranslator_processTranslatedLines_ValidInput(t *testing.T) {
	translator := &Translator{}

	translatedSubtitles := []srt.Subtitle{{Index: 1}}
	batch := []srt.SubtitleObject{{Index: "0", Content: "Original content"}}
	translatedLines := []srt.SubtitleObject{{Index: "0", Content: "Translated content"}}

	err := translator.processTranslatedLines(translatedLines, translatedSubtitles, batch)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if translatedSubtitles[0].Content != "Translated content" {
		t.Errorf("Expected content to be 'Translated content', got %q", translatedSubtitles[0].Content)
	}
}

func TestTranslator_isDominantRTL(t *testing.T) {
	translator := &Translator{}

	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "Arabic text",
			text: "مرحبا بالعالم",
			want: true,
		},
		{
			name: "Hebrew text",
			text: "שלום עולם",
			want: true,
		},
		{
			name: "English text",
			text: "Hello world",
			want: false,
		},
		{
			name: "Mixed with RTL dominant",
			text: "مرحبا hello عالم",
			want: true,
		},
		{
			name: "Mixed with LTR dominant",
			text: "Hello مرحبا world test",
			want: false,
		},
		{
			name: "Empty text",
			text: "",
			want: false,
		},
		{
			name: "Numbers and symbols",
			text: "123 !@# 456",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translator.isDominantRTL(tt.text)
			if got != tt.want {
				t.Errorf("isDominantRTL(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestProgressInfo_JSON(t *testing.T) {
	original := ProgressInfo{
		Line:      42,
		InputFile: "/path/to/input.srt",
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal ProgressInfo: %v", err)
	}

	// Unmarshal from JSON
	var unmarshaled ProgressInfo
	if err = json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ProgressInfo: %v", err)
	}

	// Compare
	if original.Line != unmarshaled.Line {
		t.Errorf("Expected Line to be %d, got %d", original.Line, unmarshaled.Line)
	}
	if original.InputFile != unmarshaled.InputFile {
		t.Errorf("Expected InputFile to be %q, got %q", original.InputFile, unmarshaled.InputFile)
	}
}

// Helper function
func floatPtr(f float32) *float32 {
	return &f
}
