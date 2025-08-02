package translator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/luispater/gemini-srt-translator-go/internal/providers"
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
				APIKeys:   []string{"key1", "key2"},
				InputFile: "test.srt",
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
			if translator.batchNumber != 1 {
				t.Error("Expected batchNumber to be 1")
			}
		})
	}
}

func TestTranslator_validatePrerequisites(t *testing.T) {
	tests := []struct {
		name       string
		translator *Translator
		wantErr    bool
	}{
		{
			name: "valid prerequisites",
			translator: &Translator{
				config: &config.Config{
					TargetLanguage: "French",
				},
				provider: &mockProvider{},
			},
			wantErr: false,
		},
		{
			name: "no provider",
			translator: &Translator{
				config: &config.Config{
					TargetLanguage: "French",
				},
				provider: nil,
			},
			wantErr: true,
		},
		{
			name: "no target language",
			translator: &Translator{
				config: &config.Config{
					TargetLanguage: "",
				},
				provider: &mockProvider{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.translator.validatePrerequisites()
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePrerequisites() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTranslator_prepareSRTFile(t *testing.T) {
	// Create temporary test files
	tempDir, err := os.MkdirTemp("", "translator_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create a test SRT file
	srtPath := filepath.Join(tempDir, "test.srt")
	srtContent := "1\n00:00:01,000 --> 00:00:03,000\nTest subtitle\n\n"
	if err = os.WriteFile(srtPath, []byte(srtContent), 0644); err != nil {
		t.Fatalf("Failed to create test SRT file: %v", err)
	}

	tests := []struct {
		name      string
		inputFile string
		wantSame  bool
		wantErr   bool
	}{
		{
			name:      "SRT file",
			inputFile: srtPath,
			wantSame:  true,
			wantErr:   false,
		},
		{
			name:      "non-existent MKV file",
			inputFile: filepath.Join(tempDir, "nonexistent.mkv"),
			wantSame:  false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := &Translator{
				config: &config.Config{
					InputFile: tt.inputFile,
				},
			}

			result, errPrepareSRTFile := translator.prepareSRTFile()
			if (errPrepareSRTFile != nil) != tt.wantErr {
				t.Errorf("prepareSRTFile() error = %v, wantErr %v", errPrepareSRTFile, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.wantSame && result != tt.inputFile {
					t.Errorf("prepareSRTFile() = %v, want %v", result, tt.inputFile)
				}
			}
		})
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
			name: "English text",
			text: "Hello world",
			want: false,
		},
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
			name: "Mixed with more English",
			text: "Hello مرحبا world",
			want: false,
		},
		{
			name: "Mixed with more Arabic",
			text: "مرحبا بالعالم Hello",
			want: true,
		},
		{
			name: "Empty text",
			text: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translator.isDominantRTL(tt.text)
			if got != tt.want {
				t.Errorf("isDominantRTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProgressInfo_JSON(t *testing.T) {
	progress := ProgressInfo{
		Line:      42,
		InputFile: "/path/to/test.srt",
	}

	// Test marshaling
	data, err := json.Marshal(progress)
	if err != nil {
		t.Fatalf("Failed to marshal ProgressInfo: %v", err)
	}

	// Test unmarshaling
	var unmarshaled ProgressInfo
	if err = json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ProgressInfo: %v", err)
	}

	if unmarshaled.Line != progress.Line {
		t.Errorf("Line mismatch: got %d, want %d", unmarshaled.Line, progress.Line)
	}
	if unmarshaled.InputFile != progress.InputFile {
		t.Errorf("InputFile mismatch: got %q, want %q", unmarshaled.InputFile, progress.InputFile)
	}
}

// mockProvider is a simple mock implementation for testing
type mockProvider struct{}

func (m *mockProvider) GetModels(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func (m *mockProvider) GetTokenLimit(ctx context.Context, modelName string) (int32, error) {
	return 1000, nil
}

func (m *mockProvider) CountTokens(ctx context.Context, modelName string, content string) (int32, error) {
	return int32(len(content)), nil
}

func (m *mockProvider) TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []providers.ContextMessage, config *providers.TranslationConfig) (*providers.TranslationResponse, error) {
	return &providers.TranslationResponse{
		TranslatedBatch: batch,
		Context:         previousContext,
	}, nil
}

func (m *mockProvider) GetName() string {
	return "mock"
}
