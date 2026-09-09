package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/luispater/gemini-srt-translator-go/internal/logger"
	"github.com/luispater/gemini-srt-translator-go/internal/providers"
	"github.com/luispater/gemini-srt-translator-go/internal/video"
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

func TestTranslator_validateConfigOpenAIProtocol(t *testing.T) {
	inputFile, err := os.CreateTemp("", "translator_protocol_*.srt")
	if err != nil {
		t.Fatalf("failed to create temporary input: %v", err)
	}
	inputPath := inputFile.Name()
	if err = inputFile.Close(); err != nil {
		t.Fatalf("failed to close temporary input: %v", err)
	}
	defer func() {
		_ = os.Remove(inputPath)
	}()

	tests := []struct {
		name     string
		protocol string
		wantErr  bool
	}{
		{name: "empty protocol preserves compatibility", protocol: "", wantErr: false},
		{name: "chat completions", protocol: "chat-completions", wantErr: false},
		{name: "responses", protocol: "responses", wantErr: false},
		{name: "invalid protocol", protocol: "legacy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			cfg.Provider = "openai"
			cfg.OpenAIProtocol = tt.protocol
			cfg.InputFile = inputPath
			translator := &Translator{config: cfg}

			errValidate := translator.validateConfig()
			if (errValidate != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", errValidate, tt.wantErr)
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

func TestTranslator_validateTranslatedResponseRejectsMismatchedIndex(t *testing.T) {
	translator := &Translator{}
	originalBatch := []srt.SubtitleObject{
		{Index: 305, Content: "With Rob-Will...", Guard: "GST_LINE_000305"},
		{Index: 306, Content: "that night.", Guard: "GST_LINE_000306"},
	}
	translatedBatch := []srt.SubtitleObject{
		{Index: 305, Content: "和罗布-威尔在一起", Guard: "GST_LINE_000305"},
		{Index: 307, Content: "那晚", Guard: "GST_LINE_000306"},
	}

	err := translator.validateTranslatedResponse(translatedBatch, originalBatch)
	if err == nil {
		t.Fatal("Expected mismatched index to be rejected")
	}
}

func TestTranslator_validateTranslatedResponseRejectsMismatchedGuard(t *testing.T) {
	translator := &Translator{}
	originalBatch := []srt.SubtitleObject{
		{Index: 305, Content: "With Rob-Will...", Guard: "GST_LINE_000305"},
		{Index: 306, Content: "that night.", Guard: "GST_LINE_000306"},
	}
	translatedBatch := []srt.SubtitleObject{
		{Index: 305, Content: "和罗布-威尔在一起    那晚", Guard: "GST_LINE_000306"},
		{Index: 306, Content: "现在我们达成共识了", Guard: "GST_LINE_000307"},
	}

	err := translator.validateTranslatedResponse(translatedBatch, originalBatch)
	if err == nil {
		t.Fatal("Expected mismatched guard to be rejected")
	}
}

func TestTranslator_processBatchAttemptPreservesFullContext(t *testing.T) {
	provider := &contextRecordingProvider{}
	translator := &Translator{
		config:   &config.Config{},
		provider: provider,
	}
	translatedSubtitles := make([]srt.Subtitle, 3)

	for i := range 3 {
		batch := []srt.SubtitleObject{
			{Index: i, Content: "source", Guard: translator.lineGuard(i)},
		}
		nextContext, errProcess := translator.processBatchAttempt(
			context.Background(),
			batch,
			translatedSubtitles,
			&ProgressBarWrapper{},
			"",
		)
		if errProcess != nil {
			t.Fatalf("processBatchAttempt() error = %v", errProcess)
		}
		translator.context = nextContext
	}

	wantPreviousContextLengths := []int{0, 2, 4}
	if len(provider.previousContextLengths) != len(wantPreviousContextLengths) {
		t.Fatalf("provider received %d requests, want %d", len(provider.previousContextLengths), len(wantPreviousContextLengths))
	}
	for i, wantLength := range wantPreviousContextLengths {
		if provider.previousContextLengths[i] != wantLength {
			t.Errorf("request %d previous context length = %d, want %d", i+1, provider.previousContextLengths[i], wantLength)
		}
	}
	if len(translator.context) != 6 {
		t.Fatalf("final context length = %d, want 6", len(translator.context))
	}
}

func TestTranslator_processBatchAttemptExcludesFailedContext(t *testing.T) {
	provider := &contextRecordingProvider{failCall: 2}
	translator := &Translator{
		config:   &config.Config{},
		provider: provider,
	}
	translatedSubtitles := make([]srt.Subtitle, 2)
	firstBatch := []srt.SubtitleObject{{Index: 0, Content: "first"}}
	secondBatch := []srt.SubtitleObject{{Index: 1, Content: "second"}}

	firstContext, errFirst := translator.processBatchAttempt(context.Background(), firstBatch, translatedSubtitles, &ProgressBarWrapper{}, "")
	if errFirst != nil {
		t.Fatalf("first processBatchAttempt() error = %v", errFirst)
	}
	translator.context = firstContext

	failedContext, errFailed := translator.processBatchAttempt(context.Background(), secondBatch, translatedSubtitles, &ProgressBarWrapper{}, "")
	if errFailed == nil || failedContext != nil {
		t.Fatalf("failed attempt returned context %+v and error %v", failedContext, errFailed)
	}

	secondContext, errSecond := translator.processBatchAttempt(context.Background(), secondBatch, translatedSubtitles, &ProgressBarWrapper{}, "")
	if errSecond != nil {
		t.Fatalf("retry processBatchAttempt() error = %v", errSecond)
	}
	translator.context = secondContext

	wantPreviousContextLengths := []int{0, 2, 2}
	for i, wantLength := range wantPreviousContextLengths {
		if provider.previousContextLengths[i] != wantLength {
			t.Errorf("attempt %d previous context length = %d, want %d", i+1, provider.previousContextLengths[i], wantLength)
		}
	}
	if len(translator.context) != 4 {
		t.Fatalf("final context length = %d, want 4", len(translator.context))
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
		Line:                   42,
		InputFile:              "/path/to/test.srt",
		Provider:               "openai",
		Protocol:               "responses",
		Model:                  "gpt-5",
		TranslationFingerprint: "fingerprint",
		PromptCacheKey:         "123e4567-e89b-42d3-a456-426614174000",
		Context: []providers.ContextMessage{
			{RawItem: json.RawMessage(`{"type":"reasoning","encrypted_content":"signature"}`)},
		},
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
	if unmarshaled.Provider != progress.Provider || unmarshaled.Protocol != progress.Protocol || unmarshaled.Model != progress.Model {
		t.Errorf("progress metadata mismatch: got provider=%q protocol=%q model=%q", unmarshaled.Provider, unmarshaled.Protocol, unmarshaled.Model)
	}
	if unmarshaled.TranslationFingerprint != progress.TranslationFingerprint {
		t.Errorf("translation fingerprint = %q, want %q", unmarshaled.TranslationFingerprint, progress.TranslationFingerprint)
	}
	if unmarshaled.PromptCacheKey != progress.PromptCacheKey {
		t.Errorf("prompt cache key = %q, want %q", unmarshaled.PromptCacheKey, progress.PromptCacheKey)
	}
	if len(unmarshaled.Context) != 1 || !strings.Contains(string(unmarshaled.Context[0].RawItem), "signature") {
		t.Errorf("progress context was not preserved: %+v", unmarshaled.Context)
	}
}

func TestTranslatorSaveProgressPersistsResponsesContext(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	progressPath := filepath.Join(tempDir, "input.progress")
	outputPath := filepath.Join(tempDir, "output.srt")
	translator := &Translator{
		config: &config.Config{
			Provider:             "openai",
			OpenAIProtocol:       "responses",
			OpenAIPromptCacheKey: "123e4567-e89b-42d3-a456-426614174000",
			ModelName:            "gpt-5",
			InputFile:            inputPath,
		},
		progressFile: progressPath,
		outputFile:   outputPath,
		context: []providers.ContextMessage{
			{RawItem: json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"summary"}],"encrypted_content":"encrypted"}`)},
			{RawItem: json.RawMessage(`{"type":"function_call","call_id":"call_1","name":"submit_translations","arguments":"{}"}`)},
			{RawItem: json.RawMessage(`{"type":"function_call_output","call_id":"call_1","output":"{\"accepted\":true}"}`)},
		},
	}

	if errSave := translator.saveProgress(12, nil, translator.context); errSave != nil {
		t.Fatalf("saveProgress() error = %v", errSave)
	}

	progressData, errRead := os.ReadFile(progressPath)
	if errRead != nil {
		t.Fatalf("failed to read saved progress: %v", errRead)
	}
	var progress ProgressInfo
	if errUnmarshal := json.Unmarshal(progressData, &progress); errUnmarshal != nil {
		t.Fatalf("failed to decode saved progress: %v", errUnmarshal)
	}
	if progress.Provider != "openai" || progress.Protocol != "responses" || progress.Model != "gpt-5" {
		t.Errorf("saved progress metadata = provider %q, protocol %q, model %q", progress.Provider, progress.Protocol, progress.Model)
	}
	expectedFingerprint, errFingerprint := translator.translationConfigFingerprint()
	if errFingerprint != nil {
		t.Fatalf("translationConfigFingerprint() error = %v", errFingerprint)
	}
	if progress.TranslationFingerprint != expectedFingerprint {
		t.Errorf("saved translation fingerprint = %q, want %q", progress.TranslationFingerprint, expectedFingerprint)
	}
	if progress.PromptCacheKey != translator.config.OpenAIPromptCacheKey {
		t.Errorf("saved prompt cache key = %q, want %q", progress.PromptCacheKey, translator.config.OpenAIPromptCacheKey)
	}
	if len(progress.Context) != len(translator.context) {
		t.Fatalf("saved context length = %d, want %d", len(progress.Context), len(translator.context))
	}
	contextData, errMarshal := json.Marshal(progress.Context)
	if errMarshal != nil {
		t.Fatalf("failed to marshal saved context: %v", errMarshal)
	}
	for _, expected := range []string{"summary_text", "encrypted", "submit_translations", "accepted"} {
		if !strings.Contains(string(contextData), expected) {
			t.Errorf("saved context does not contain %q: %s", expected, contextData)
		}
	}
}

func TestTranslatorRestartCheckpointRollsBackArtifactsOnFailure(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "output.srt")
	oldOutput := []byte("previous translation")
	if errWriteOld := os.WriteFile(outputPath, oldOutput, 0644); errWriteOld != nil {
		t.Fatalf("Failed to create previous output: %v", errWriteOld)
	}
	metadataPath := filepath.Join(tempDir, "output.srt.gst-meta.json")
	translator := &Translator{
		config:            &config.Config{InputFile: filepath.Join(tempDir, "input.mkv"), SubtitleTrack: 2},
		outputFile:        outputPath,
		progressFile:      filepath.Join(tempDir, "missing", "input.progress"),
		metadataFile:      metadataPath,
		sourceFingerprint: "source-sha256",
		restartPending:    true,
	}

	errSave := translator.saveProgress(1, []srt.Subtitle{{Index: 1, Content: "new translation"}}, nil)
	if errSave == nil {
		t.Fatal("saveProgress() returned no error for an unavailable progress directory")
	}
	restoredOutput, errReadOutput := os.ReadFile(outputPath)
	if errReadOutput != nil {
		t.Fatalf("Failed to read restored output: %v", errReadOutput)
	}
	if string(restoredOutput) != string(oldOutput) {
		t.Errorf("Output was not rolled back: got %q, want %q", restoredOutput, oldOutput)
	}
	if _, errStatMetadata := os.Stat(metadataPath); !os.IsNotExist(errStatMetadata) {
		t.Errorf("New metadata was not rolled back: %v", errStatMetadata)
	}
	if !translator.restartPending {
		t.Error("Failed checkpoint cleared restartPending")
	}
}

func TestTranslatorSaveProgressPersistsMKVSourceMetadata(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "movie.mkv")
	metadataPath := filepath.Join(tempDir, "movie.srt.gst-meta.json")
	translator := &Translator{
		config: &config.Config{
			InputFile:     inputPath,
			SubtitleTrack: 4,
		},
		outputFile:        filepath.Join(tempDir, "movie.srt"),
		progressFile:      filepath.Join(tempDir, "movie.progress"),
		metadataFile:      metadataPath,
		sourceFingerprint: "source-sha256",
	}

	if errSave := translator.saveProgress(1, nil, nil); errSave != nil {
		t.Fatalf("saveProgress() error = %v", errSave)
	}
	metadataData, errReadMetadata := os.ReadFile(metadataPath)
	if errReadMetadata != nil {
		t.Fatalf("Failed to read translation metadata: %v", errReadMetadata)
	}
	var metadata translationMetadata
	if errUnmarshal := json.Unmarshal(metadataData, &metadata); errUnmarshal != nil {
		t.Fatalf("Failed to decode translation metadata: %v", errUnmarshal)
	}
	if metadata.InputFile != inputPath || metadata.SubtitleTrack != 4 || metadata.SourceFingerprint != "source-sha256" {
		t.Errorf("Translation metadata = %+v", metadata)
	}
	if matches, reason := translator.outputMetadataMatchesSource(); !matches {
		t.Errorf("outputMetadataMatchesSource() = false, reason %q", reason)
	}
	translator.config.SubtitleTrack = 5
	if matches, _ := translator.outputMetadataMatchesSource(); matches {
		t.Error("outputMetadataMatchesSource() accepted a different subtitle track")
	}
}

func TestTranslatorRemoveCompletedTaskFilesRemovesMKVMetadata(t *testing.T) {
	tempDir := t.TempDir()
	progressPath := filepath.Join(tempDir, "movie.progress")
	metadataPath := filepath.Join(tempDir, "movie.srt.gst-meta.json")
	outputPath := filepath.Join(tempDir, "movie.srt")
	for _, filePath := range []string{progressPath, metadataPath, outputPath} {
		if errWrite := os.WriteFile(filePath, []byte("test"), 0644); errWrite != nil {
			t.Fatalf("Failed to create task file %s: %v", filePath, errWrite)
		}
	}
	translator := &Translator{
		config:       &config.Config{InputFile: filepath.Join(tempDir, "movie.mkv")},
		outputFile:   outputPath,
		progressFile: progressPath,
		metadataFile: metadataPath,
	}

	translator.removeCompletedTaskFiles()
	for _, removedPath := range []string{progressPath, metadataPath} {
		if _, errStat := os.Stat(removedPath); !os.IsNotExist(errStat) {
			t.Errorf("Completed task file still exists: %s", removedPath)
		}
	}
	if _, errStatOutput := os.Stat(outputPath); errStatOutput != nil {
		t.Errorf("Translated output was removed: %v", errStatOutput)
	}
}

func TestTranslatorRemoveCompletedTaskFilesRemovesMetadataForNonMKV(t *testing.T) {
	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "output.srt.gst-meta.json")
	if errWrite := os.WriteFile(metadataPath, []byte("test"), 0644); errWrite != nil {
		t.Fatalf("Failed to create metadata file: %v", errWrite)
	}

	translator := &Translator{
		config:       &config.Config{InputFile: filepath.Join(tempDir, "input.srt")},
		metadataFile: metadataPath,
	}

	translator.removeCompletedTaskFiles()
	if _, errStat := os.Stat(metadataPath); !os.IsNotExist(errStat) {
		t.Errorf("Completed metadata file still exists: %v", errStat)
	}
}

func TestTranslatorCheckSavedProgressRestoresOnlyMatchingResponsesContext(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	progressPath := filepath.Join(tempDir, "input.progress")
	resume := true
	contextMessages := []providers.ContextMessage{
		{RawItem: json.RawMessage(`{"type":"reasoning","encrypted_content":"saved-signature"}`)},
	}
	matchingConfig := &config.Config{
		Provider:       "openai",
		OpenAIProtocol: "responses",
		ModelName:      "gpt-5",
		InputFile:      inputPath,
		Resume:         &resume,
	}
	fingerprintTranslator := &Translator{config: matchingConfig}
	fingerprint, errFingerprint := fingerprintTranslator.translationConfigFingerprint()
	if errFingerprint != nil {
		t.Fatalf("translationConfigFingerprint() error = %v", errFingerprint)
	}
	progress := ProgressInfo{
		Line:                   9,
		InputFile:              inputPath,
		Provider:               "openai",
		Protocol:               "responses",
		Model:                  "gpt-5",
		TranslationFingerprint: fingerprint,
		PromptCacheKey:         "123e4567-e89b-42d3-a456-426614174000",
		Context:                contextMessages,
	}
	progressData, errMarshal := json.Marshal(progress)
	if errMarshal != nil {
		t.Fatalf("failed to marshal progress: %v", errMarshal)
	}
	if errWrite := os.WriteFile(progressPath, progressData, 0644); errWrite != nil {
		t.Fatalf("failed to write progress: %v", errWrite)
	}

	matchingTranslator := &Translator{
		config:       matchingConfig,
		progressFile: progressPath,
	}
	matchingTranslator.checkSavedProgress()
	if matchingTranslator.config.StartLine != progress.Line {
		t.Errorf("matching progress start line = %d, want %d", matchingTranslator.config.StartLine, progress.Line)
	}
	if len(matchingTranslator.context) != 1 || !strings.Contains(string(matchingTranslator.context[0].RawItem), "saved-signature") {
		t.Errorf("matching progress context was not restored: %+v", matchingTranslator.context)
	}
	if matchingTranslator.config.OpenAIPromptCacheKey != progress.PromptCacheKey {
		t.Errorf("restored prompt cache key = %q, want %q", matchingTranslator.config.OpenAIPromptCacheKey, progress.PromptCacheKey)
	}

	mismatchedTranslator := &Translator{
		config: &config.Config{
			Provider:       "openai",
			OpenAIProtocol: "responses",
			ModelName:      "gpt-5-mini",
			InputFile:      inputPath,
			Resume:         &resume,
		},
		progressFile: progressPath,
	}
	mismatchedTranslator.checkSavedProgress()
	if len(mismatchedTranslator.context) != 0 {
		t.Errorf("mismatched progress context was restored: %+v", mismatchedTranslator.context)
	}
}

func TestTranslatorCheckSavedProgressRestoresLineOnePromptCacheKey(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	outputPath := filepath.Join(tempDir, "output.srt")
	progressPath := filepath.Join(tempDir, "input.progress")
	promptCacheKey := "123e4567-e89b-42d3-a456-426614174000"

	newConfig := func(resume *bool) *config.Config {
		return &config.Config{
			Provider:       "openai",
			OpenAIProtocol: "responses",
			ModelName:      "gpt-5",
			InputFile:      inputPath,
			Resume:         resume,
		}
	}
	progressConfig := newConfig(nil)
	fingerprintTranslator := &Translator{config: progressConfig}
	fingerprint, errFingerprint := fingerprintTranslator.translationConfigFingerprint()
	if errFingerprint != nil {
		t.Fatalf("translationConfigFingerprint() error = %v", errFingerprint)
	}
	progress := ProgressInfo{
		Line:                   1,
		InputFile:              inputPath,
		Provider:               "openai",
		Protocol:               "responses",
		Model:                  "gpt-5",
		TranslationFingerprint: fingerprint,
		PromptCacheKey:         promptCacheKey,
	}
	progressData, errMarshal := json.Marshal(progress)
	if errMarshal != nil {
		t.Fatalf("failed to marshal progress: %v", errMarshal)
	}
	if errWriteProgress := os.WriteFile(progressPath, progressData, 0644); errWriteProgress != nil {
		t.Fatalf("failed to write progress: %v", errWriteProgress)
	}

	translator := &Translator{
		config:       newConfig(nil),
		outputFile:   outputPath,
		progressFile: progressPath,
	}
	translator.checkSavedProgress()
	if translator.config.StartLine != 1 {
		t.Errorf("restored start line = %d, want 1", translator.config.StartLine)
	}
	if translator.config.OpenAIPromptCacheKey != promptCacheKey {
		t.Errorf("restored prompt cache key = %q, want %q", translator.config.OpenAIPromptCacheKey, promptCacheKey)
	}

	resume := false
	if errWriteProgress := os.WriteFile(progressPath, progressData, 0644); errWriteProgress != nil {
		t.Fatalf("failed to rewrite progress: %v", errWriteProgress)
	}
	if errWriteOutput := os.WriteFile(outputPath, []byte("stale output"), 0644); errWriteOutput != nil {
		t.Fatalf("failed to write stale output: %v", errWriteOutput)
	}
	newTaskTranslator := &Translator{
		config:       newConfig(&resume),
		outputFile:   outputPath,
		progressFile: progressPath,
	}
	newTaskTranslator.checkSavedProgress()
	if newTaskTranslator.config.StartLine != 0 {
		t.Errorf("new task start line = %d, want 0", newTaskTranslator.config.StartLine)
	}
	if newTaskTranslator.config.OpenAIPromptCacheKey != "" {
		t.Errorf("new task restored prompt cache key %q", newTaskTranslator.config.OpenAIPromptCacheKey)
	}
	if !newTaskTranslator.restartPending {
		t.Error("new task did not schedule a safe restart")
	}
	if _, errStatProgress := os.Stat(progressPath); errStatProgress != nil {
		t.Errorf("progress file was removed before the first new checkpoint: %v", errStatProgress)
	}
	if _, errStatOutput := os.Stat(outputPath); errStatOutput != nil {
		t.Errorf("output file was removed before the first new checkpoint: %v", errStatOutput)
	}
}

func TestTranslatorResponsesInitializesPromptCacheKeyBeforeRequestsAndInitialProgress(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	outputPath := filepath.Join(tempDir, "output.srt")
	progressPath := filepath.Join(tempDir, "input.progress")
	inputContent := "1\n00:00:00,000 --> 00:00:01,000\nSource\n"
	if errWriteInput := os.WriteFile(inputPath, []byte(inputContent), 0644); errWriteInput != nil {
		t.Fatalf("failed to write input: %v", errWriteInput)
	}

	uuidV4Pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	modelRequestSawInitializedKey := false
	initialProgressKey := ""
	initialProgressLine := 0
	requestKeys := make([]string, 0, 2)
	responsesRequestCount := 0
	var translatorConfig *config.Config
	submitArguments := `{"records":[{"index":0,"content":"Traduit","guard":"GST_LINE_000000"}]}`

	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/models":
			modelRequestSawInitializedKey = translatorConfig != nil && uuidV4Pattern.MatchString(translatorConfig.OpenAIPromptCacheKey)
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = responseWriter.Write([]byte(`{"object":"list","data":[{"id":"gpt-test","object":"model","created":1,"owned_by":"test"}]}`))
		case "/v1/responses":
			requestBody, errReadBody := io.ReadAll(request.Body)
			if errReadBody != nil {
				http.Error(responseWriter, errReadBody.Error(), http.StatusInternalServerError)
				return
			}
			var requestData struct {
				PromptCacheKey string `json:"prompt_cache_key"`
			}
			if errUnmarshalRequest := json.Unmarshal(requestBody, &requestData); errUnmarshalRequest != nil {
				http.Error(responseWriter, errUnmarshalRequest.Error(), http.StatusBadRequest)
				return
			}
			requestKeys = append(requestKeys, requestData.PromptCacheKey)
			responsesRequestCount++
			responseWriter.Header().Set("Content-Type", "application/json")
			if responsesRequestCount == 1 {
				progressData, errReadProgress := os.ReadFile(progressPath)
				if errReadProgress == nil {
					var progress ProgressInfo
					if errUnmarshalProgress := json.Unmarshal(progressData, &progress); errUnmarshalProgress == nil {
						initialProgressKey = progress.PromptCacheKey
						initialProgressLine = progress.Line
					}
				}
				_, _ = responseWriter.Write([]byte(`{"id":"resp_read","object":"response","created_at":1,"model":"gpt-test","status":"completed","output":[{"type":"function_call","id":"fc_read","call_id":"call_read","name":"read_translation_batch","arguments":"{}","status":"completed"}]}`))
				return
			}
			responseBody := fmt.Sprintf(`{"id":"resp_submit","object":"response","created_at":1,"model":"gpt-test","status":"completed","output":[{"type":"function_call","id":"fc_submit","call_id":"call_submit","name":"submit_translations","arguments":%q,"status":"completed"}]}`, submitArguments)
			_, _ = responseWriter.Write([]byte(responseBody))
		default:
			http.Error(responseWriter, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	translatorConfig = config.NewConfig()
	translatorConfig.Provider = "openai"
	translatorConfig.OpenAIProtocol = "responses"
	translatorConfig.APIKeys = []string{"test-key"}
	translatorConfig.BaseURL = server.URL + "/v1"
	translatorConfig.ModelName = "gpt-test"
	translatorConfig.InputFile = inputPath
	translatorConfig.OutputFile = outputPath
	translatorConfig.TargetLanguage = "French"
	translatorConfig.BatchSize = 1
	translatorConfig.RetryCount = 0
	translatorConfig.Streaming = false
	translatorConfig.Thinking = false

	translator := NewTranslator(translatorConfig)
	if errTranslate := translator.Translate(context.Background()); errTranslate != nil {
		t.Fatalf("Translate() error = %v", errTranslate)
	}
	if !modelRequestSawInitializedKey {
		t.Error("prompt cache key was not initialized before model validation request")
	}
	if len(requestKeys) != 2 {
		t.Fatalf("Responses request count = %d, want 2", len(requestKeys))
	}
	if !uuidV4Pattern.MatchString(requestKeys[0]) || requestKeys[1] != requestKeys[0] {
		t.Errorf("Responses request prompt cache keys = %v, want one stable UUID v4", requestKeys)
	}
	if initialProgressLine != 1 {
		t.Errorf("initial progress line = %d, want 1", initialProgressLine)
	}
	if initialProgressKey != requestKeys[0] {
		t.Errorf("initial progress key = %q, first request key = %q", initialProgressKey, requestKeys[0])
	}
}

func TestTranslatorProgressContextMatchesTranslationConfiguration(t *testing.T) {
	newConfig := func() *config.Config {
		temperature := float32(0.4)
		topP := float32(0.8)
		topK := float32(32)
		return &config.Config{
			Provider:       "openai",
			OpenAIProtocol: "responses",
			ModelName:      "gpt-5",
			InputFile:      "input.srt",
			BaseURL:        " https://api.example.test/v1/ ",
			APIKeys:        []string{"first-secret"},
			TargetLanguage: "French",
			Description:    "Informal dialogue",
			Thinking:       true,
			ThinkingLevel:  "high",
			Temperature:    &temperature,
			TopP:           &topP,
			TopK:           &topK,
		}
	}

	matchingTranslator := &Translator{config: newConfig()}
	fingerprint, errFingerprint := matchingTranslator.translationConfigFingerprint()
	if errFingerprint != nil {
		t.Fatalf("translationConfigFingerprint() error = %v", errFingerprint)
	}
	progress := ProgressInfo{
		InputFile:              matchingTranslator.config.InputFile,
		Provider:               matchingTranslator.config.Provider,
		Protocol:               matchingTranslator.config.OpenAIProtocol,
		Model:                  matchingTranslator.config.ModelName,
		TranslationFingerprint: fingerprint,
	}
	if !matchingTranslator.progressContextMatches(progress) {
		t.Fatal("progressContextMatches() = false for matching translation configuration")
	}

	equivalentEndpointConfig := newConfig()
	equivalentEndpointConfig.BaseURL = "https://api.example.test/v1"
	if !(&Translator{config: equivalentEndpointConfig}).progressContextMatches(progress) {
		t.Fatal("progressContextMatches() = false after BaseURL normalization")
	}

	rotatedKeyConfig := newConfig()
	rotatedKeyConfig.APIKeys = []string{"rotated-secret"}
	if !(&Translator{config: rotatedKeyConfig}).progressContextMatches(progress) {
		t.Fatal("progressContextMatches() = false after API key rotation")
	}

	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{name: "base URL", mutate: func(cfg *config.Config) { cfg.BaseURL = "https://other.example.test/v1" }},
		{name: "target language", mutate: func(cfg *config.Config) { cfg.TargetLanguage = "German" }},
		{name: "description", mutate: func(cfg *config.Config) { cfg.Description = "Formal dialogue" }},
		{name: "thinking", mutate: func(cfg *config.Config) { cfg.Thinking = false }},
		{name: "thinking level", mutate: func(cfg *config.Config) { cfg.ThinkingLevel = "low" }},
		{name: "temperature", mutate: func(cfg *config.Config) { value := float32(0.5); cfg.Temperature = &value }},
		{name: "top p", mutate: func(cfg *config.Config) { value := float32(0.7); cfg.TopP = &value }},
		{name: "top k", mutate: func(cfg *config.Config) { value := float32(16); cfg.TopK = &value }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			mismatchedConfig := newConfig()
			testCase.mutate(mismatchedConfig)
			mismatchedTranslator := &Translator{config: mismatchedConfig}
			if mismatchedTranslator.progressContextMatches(progress) {
				t.Error("progressContextMatches() = true for changed translation configuration")
			}
		})
	}
}

func TestTranslatorPerformTranslationStopsWithoutAdvancingProgressOrContextOnOutputWriteFailure(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	outputPath := filepath.Join(tempDir, "output.srt")
	progressPath := filepath.Join(tempDir, "input.progress")
	inputContent := "1\n00:00:00,000 --> 00:00:01,000\nSource\n"
	if errWriteInput := os.WriteFile(inputPath, []byte(inputContent), 0644); errWriteInput != nil {
		t.Fatalf("failed to write input: %v", errWriteInput)
	}

	initialContext := []providers.ContextMessage{{Role: "user", Content: "existing context"}}
	provider := &outputBlockingProvider{outputPath: outputPath}
	translator := &Translator{
		config: &config.Config{
			Provider:       "openai",
			OpenAIProtocol: "responses",
			ModelName:      "gpt-5",
			InputFile:      inputPath,
			TargetLanguage: "French",
			BatchSize:      1,
			StartLine:      1,
			Thinking:       true,
			ThinkingLevel:  "high",
		},
		provider:     provider,
		outputFile:   outputPath,
		progressFile: progressPath,
		context:      append([]providers.ContextMessage(nil), initialContext...),
	}

	errTranslate := translator.performTranslation(context.Background())
	if errTranslate == nil || !strings.Contains(errTranslate.Error(), "failed to write output file") {
		t.Fatalf("performTranslation() error = %v, want output write failure", errTranslate)
	}
	if len(translator.context) != len(initialContext) || translator.context[0].Content != initialContext[0].Content {
		t.Errorf("context advanced after output write failure: %+v", translator.context)
	}

	progressData, errReadProgress := os.ReadFile(progressPath)
	if errReadProgress != nil {
		t.Fatalf("failed to read progress after write failure: %v", errReadProgress)
	}
	var progress ProgressInfo
	if errUnmarshal := json.Unmarshal(progressData, &progress); errUnmarshal != nil {
		t.Fatalf("failed to decode progress after write failure: %v", errUnmarshal)
	}
	if progress.Line != 1 {
		t.Errorf("progress line advanced to %d after output write failure, want 1", progress.Line)
	}
	if len(progress.Context) != len(initialContext) || progress.Context[0].Content != initialContext[0].Content {
		t.Errorf("saved context advanced after output write failure: %+v", progress.Context)
	}
}

func TestTranslatorRejectsProgressFromDifferentSubtitleTrack(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "movie.mkv")
	outputPath := filepath.Join(tempDir, "movie.en.srt")
	progressPath := filepath.Join(tempDir, "movie.progress")
	extractedPath := filepath.Join(tempDir, "movie_extracted.srt")
	progressData, errMarshal := json.Marshal(ProgressInfo{
		Line:          5,
		InputFile:     inputPath,
		SubtitleTrack: 1,
	})
	if errMarshal != nil {
		t.Fatalf("Failed to marshal progress: %v", errMarshal)
	}
	for filePath, content := range map[string][]byte{
		outputPath:    []byte("translated"),
		progressPath:  progressData,
		extractedPath: []byte("extracted"),
	} {
		if errWrite := os.WriteFile(filePath, content, 0644); errWrite != nil {
			t.Fatalf("Failed to create %s: %v", filePath, errWrite)
		}
	}

	translator := &Translator{
		config: &config.Config{
			InputFile:     inputPath,
			SubtitleTrack: 2,
		},
		outputFile:   outputPath,
		progressFile: progressPath,
	}
	translator.checkSavedProgress()

	if translator.config.StartLine != 0 {
		t.Errorf("Start line = %d, want 0 after track mismatch", translator.config.StartLine)
	}
	if !translator.restartPending {
		t.Error("Track mismatch did not schedule a safe restart")
	}
	for _, filePath := range []string{outputPath, progressPath} {
		if _, errStat := os.Stat(filePath); errStat != nil {
			t.Errorf("Previous artifact %s was removed before translation: %v", filePath, errStat)
		}
	}
	if _, errStatExtracted := os.Stat(extractedPath); errStatExtracted != nil {
		t.Errorf("Unowned extracted file was removed after track mismatch: %v", errStatExtracted)
	}
}

func TestTranslatorLoadSubtitlesFailsWithoutInteractiveStartLine(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	outputPath := filepath.Join(tempDir, "output.srt")
	srtContent := "1\n00:00:00,000 --> 00:00:01,000\nSubtitle\n"
	for _, filePath := range []string{inputPath, outputPath} {
		if errWrite := os.WriteFile(filePath, []byte(srtContent), 0644); errWrite != nil {
			t.Fatalf("Failed to create subtitle file: %v", errWrite)
		}
	}
	translator := &Translator{
		config:     &config.Config{InputFile: inputPath, BatchSize: 1},
		outputFile: outputPath,
	}

	logger.SetQuietMode(true)
	defer logger.SetQuietMode(false)
	if errLoad := translator.loadSubtitles(inputPath); errLoad == nil || !strings.Contains(errLoad.Error(), "--start-line") {
		t.Fatalf("loadSubtitles() error = %v, want non-interactive start line guidance", errLoad)
	}
}

func TestTranslatorLoadSubtitlesRestartsWhenResumeOutputIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.srt")
	progressPath := filepath.Join(tempDir, "input.progress")
	inputContent := "1\n00:00:00,000 --> 00:00:01,000\nSource\n"
	if errWriteInput := os.WriteFile(inputPath, []byte(inputContent), 0644); errWriteInput != nil {
		t.Fatalf("Failed to create input: %v", errWriteInput)
	}
	if errWriteProgress := os.WriteFile(progressPath, []byte("progress"), 0644); errWriteProgress != nil {
		t.Fatalf("Failed to create progress: %v", errWriteProgress)
	}
	translator := &Translator{
		config:       &config.Config{InputFile: inputPath, StartLine: 5, BatchSize: 1},
		outputFile:   filepath.Join(tempDir, "missing-output.srt"),
		progressFile: progressPath,
		metadataFile: filepath.Join(tempDir, "missing-output.srt.gst-meta.json"),
		context:      []providers.ContextMessage{{Role: "user", Content: "stale context"}},
	}

	if errLoad := translator.loadSubtitles(inputPath); errLoad != nil {
		t.Fatalf("loadSubtitles() error = %v", errLoad)
	}
	if translator.config.StartLine != 1 {
		t.Errorf("Start line = %d, want 1", translator.config.StartLine)
	}
	if len(translator.context) != 0 {
		t.Errorf("Stale context was retained: %+v", translator.context)
	}
	if !translator.restartPending {
		t.Error("Missing resume output did not schedule a safe restart")
	}
	if _, errStatProgress := os.Stat(progressPath); errStatProgress != nil {
		t.Errorf("Stale progress was removed before a translated checkpoint: %v", errStatProgress)
	}
}

func TestTranslatorLegacyProgressKeepsContextSynthesisCompatibility(t *testing.T) {
	tempDir := t.TempDir()
	progressPath := filepath.Join(tempDir, "input.progress")
	inputPath := filepath.Join(tempDir, "input.srt")
	resume := true
	legacyProgress := []byte(fmt.Sprintf(`{"line":5,"input_file":%q}`, inputPath))
	if errWrite := os.WriteFile(progressPath, legacyProgress, 0644); errWrite != nil {
		t.Fatalf("failed to write legacy progress: %v", errWrite)
	}
	translator := &Translator{
		config:       &config.Config{InputFile: inputPath, Resume: &resume},
		progressFile: progressPath,
	}

	translator.checkSavedProgress()

	if translator.config.StartLine != 5 {
		t.Errorf("legacy progress start line = %d, want 5", translator.config.StartLine)
	}
	if len(translator.context) != 0 {
		t.Errorf("legacy progress unexpectedly restored context: %+v", translator.context)
	}
}

type outputBlockingProvider struct {
	mockProvider
	outputPath string
}

func (p *outputBlockingProvider) TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []providers.ContextMessage, translationConfig *providers.TranslationConfig) (*providers.TranslationResponse, error) {
	errRemove := os.Remove(p.outputPath)
	if errRemove != nil {
		return nil, fmt.Errorf("failed to remove output before blocking it: %w", errRemove)
	}
	errMkdir := os.Mkdir(p.outputPath, 0755)
	if errMkdir != nil {
		return nil, fmt.Errorf("failed to block output path: %w", errMkdir)
	}
	return &providers.TranslationResponse{
		TranslatedBatch: batch,
		Context: []providers.ContextMessage{
			{Role: "user", Content: "new request"},
			{Role: "model", Content: "new response"},
		},
	}, nil
}

type contextRecordingProvider struct {
	mockProvider
	previousContextLengths []int
	failCall               int
}

func (p *contextRecordingProvider) TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []providers.ContextMessage, config *providers.TranslationConfig) (*providers.TranslationResponse, error) {
	p.previousContextLengths = append(p.previousContextLengths, len(previousContext))
	translatedBatch := batch
	if p.failCall == len(p.previousContextLengths) {
		translatedBatch = append([]srt.SubtitleObject(nil), batch...)
		translatedBatch[0].Index++
	}
	return &providers.TranslationResponse{
		TranslatedBatch: translatedBatch,
		Context: []providers.ContextMessage{
			{Role: "user", Content: "request"},
			{Role: "model", Content: "response"},
		},
	}, nil
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
		Context: []providers.ContextMessage{
			{Role: "user", Content: "request"},
			{Role: "model", Content: "response"},
		},
	}, nil
}

func (m *mockProvider) GetName() string {
	return "mock"
}

func TestTranslator_ASSFile_LoadAndSave(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
ScriptType: v4.00+
PlayResX: 1920
PlayResY: 1080

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,2,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,{\i1}Hello world{\i0}
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,{\pos(100,200)}Second line
Dialogue: 0,0:00:07.00,0:00:08.00,Default,,0,0,0,,{\pos(300,400)}
Dialogue: 0,0:00:09.00,0:00:10.00,Default,,0,0,0,,{\p1}m 0 0 l 100 100{\p0}
Dialogue: 0,0:00:11.00,0:00:12.00,Default,,0,0,0,,Inline {\i1}style{\i0} test
`
	inputPath := filepath.Join(tempDir, "input.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write test ASS file: %v", errWrite)
	}

	outputPath := filepath.Join(tempDir, "output.srt")
	cfg := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
	}

	tr := NewTranslator(cfg)
	if errLoad := tr.loadSubtitles(inputPath); errLoad != nil {
		t.Fatalf("loadSubtitles() failed: %v", errLoad)
	}

	if len(tr.originalSubtitles) != 5 {
		t.Fatalf("expected 5 subtitles, got %d", len(tr.originalSubtitles))
	}
	// CleanText should protect {\i1} and {\i0} as placeholders
	if tr.originalSubtitles[0].Content != "[[ASSTAG0]]Hello world[[ASSTAG1]]" {
		t.Errorf("expected protected content, got %q", tr.originalSubtitles[0].Content)
	}
	// CleanText should protect {\pos(100,200)}
	if tr.originalSubtitles[1].Content != "[[ASSTAG0]]Second line" {
		t.Errorf("expected protected content, got %q", tr.originalSubtitles[1].Content)
	}
	// Pure tag line should have empty content
	if tr.originalSubtitles[2].Content != "" {
		t.Errorf("expected empty content for pure tag line, got %q", tr.originalSubtitles[2].Content)
	}
	// Drawing line should have empty content
	if tr.originalSubtitles[3].Content != "" {
		t.Errorf("expected empty content for drawing line, got %q", tr.originalSubtitles[3].Content)
	}
	// Inline tag line should have placeholder in CleanText
	if !strings.Contains(tr.originalSubtitles[4].Content, "[[ASSTAG0]]") {
		t.Errorf("expected inline tag placeholder in line 5, got %q", tr.originalSubtitles[4].Content)
	}

	translated := []srt.Subtitle{
		{
			Index:   1,
			Start:   1 * time.Second,
			End:     3 * time.Second,
			Content: "[[ASSTAG0]]你好世界[[ASSTAG1]]",
		},
		{
			Index:   2,
			Start:   4 * time.Second,
			End:     6 * time.Second,
			Content: "[[ASSTAG0]]第二行",
		},
		{
			Index:   3,
			Start:   7 * time.Second,
			End:     8 * time.Second,
			Content: "",
		},
		{
			Index:   4,
			Start:   9 * time.Second,
			End:     10 * time.Second,
			Content: "",
		},
		{
			Index:   5,
			Start:   11 * time.Second,
			End:     12 * time.Second,
			Content: "行内 [[ASSTAG0]]样式[[ASSTAG1]] 测试",
		},
	}

	batchObj := []srt.SubtitleObject{
		{Index: 0, Content: translated[0].Content},
		{Index: 1, Content: translated[1].Content},
		{Index: 4, Content: translated[4].Content},
	}
	if errProcess := tr.processTranslatedLines(batchObj, translated, batchObj); errProcess != nil {
		t.Fatalf("processTranslatedLines failed: %v", errProcess)
	}

	if errSave := tr.saveProgress(5, translated, nil); errSave != nil {
		t.Fatalf("saveProgress() failed: %v", errSave)
	}

	savedBytes, errReadSaved := os.ReadFile(outputPath)
	if errReadSaved != nil {
		t.Fatalf("failed to read saved output file: %v", errReadSaved)
	}

	savedStr := string(savedBytes)
	if strings.Contains(savedStr, "[Script Info]") || strings.Contains(savedStr, "Dialogue:") {
		t.Errorf("saved file should be SRT, but contains ASS headers/events:\n%s", savedStr)
	}

	parsedSRT, errParseSRT := srt.ParseSRT(savedStr)
	if errParseSRT != nil {
		t.Fatalf("failed to parse saved SRT file: %v", errParseSRT)
	}

	if len(parsedSRT) != 3 {
		t.Fatalf("expected 3 SRT entries (excluding non-translatable drawing and pure tags), got %d", len(parsedSRT))
	}

	if parsedSRT[0].Index != 1 || parsedSRT[0].Content != "你好世界" {
		t.Errorf("unexpected SRT entry 1: %+v", parsedSRT[0])
	}
	if parsedSRT[1].Index != 2 || parsedSRT[1].Content != "第二行" {
		t.Errorf("unexpected SRT entry 2: %+v", parsedSRT[1])
	}
	if parsedSRT[2].Index != 3 || parsedSRT[2].Content != "行内 样式 测试" {
		t.Errorf("unexpected SRT entry 3: %+v", parsedSRT[2])
	}
}

func TestTranslator_NewTranslator_ASSOutputFile(t *testing.T) {
	cfgAss := &config.Config{
		InputFile:      "movie.ass",
		TargetLanguage: "Simplified Chinese",
	}
	trAss := NewTranslator(cfgAss)
	if !strings.HasSuffix(trAss.outputFile, ".chs.srt") {
		t.Errorf("expected .chs.srt suffix, got %s", trAss.outputFile)
	}

	cfgExplicitAss := &config.Config{
		InputFile:      "movie.ass",
		OutputFile:     "custom.ass",
		TargetLanguage: "Simplified Chinese",
	}
	trExplicitAss := NewTranslator(cfgExplicitAss)
	if !strings.HasSuffix(trExplicitAss.outputFile, ".srt") {
		t.Errorf("expected custom.srt suffix when explicit ASS output is requested, got %s", trExplicitAss.outputFile)
	}

	cfgSrt := &config.Config{
		InputFile:      "movie.srt",
		TargetLanguage: "Simplified Chinese",
	}
	trSrt := NewTranslator(cfgSrt)
	if !strings.HasSuffix(trSrt.outputFile, ".chs.srt") {
		t.Errorf("expected .chs.srt suffix, got %s", trSrt.outputFile)
	}

	sampleMKV := "/Volumes/storage/Downloads/upload/mkv/Lioness.2023.S03E03.The.Bear.Is.Infected.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb.mkv"
	if _, errStat := os.Stat(sampleMKV); errStat == nil {
		cfgMkv := &config.Config{
			InputFile:      sampleMKV,
			SubtitleTrack:  3,
			TargetLanguage: "Simplified Chinese",
		}
		trMkv := NewTranslator(cfgMkv)
		if !strings.HasSuffix(trMkv.outputFile, ".chs.srt") {
			t.Errorf("expected MKV with ASS to default to .chs.srt, got %s", trMkv.outputFile)
		}
	}
}

func TestTranslator_ASSResumePreservesTags(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
Title: Resume Test

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,{\i1}Hello world{\i0}
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,Second line
`
	inputPath := filepath.Join(tempDir, "input.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write input ASS file: %v", errWrite)
	}

	outputPath := filepath.Join(tempDir, "output.srt")

	// Phase 1: Translate line 1
	cfg1 := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
		StartLine:      1,
	}
	tr1 := NewTranslator(cfg1)
	if errLoad1 := tr1.loadSubtitles(inputPath); errLoad1 != nil {
		t.Fatalf("tr1.loadSubtitles failed: %v", errLoad1)
	}
	trans1 := []srt.Subtitle{
		{Index: 1, Start: 1 * time.Second, End: 3 * time.Second, Content: "[[ASSTAG0]]你好世界[[ASSTAG1]]"},
		{Index: 2, Start: 4 * time.Second, End: 6 * time.Second, Content: "Second line"},
	}
	batch1 := []srt.SubtitleObject{{Index: 0, Content: trans1[0].Content}}
	if errProcess1 := tr1.processTranslatedLines(batch1, trans1, batch1); errProcess1 != nil {
		t.Fatalf("tr1.processTranslatedLines failed: %v", errProcess1)
	}
	if errSave1 := tr1.saveProgress(1, trans1, nil); errSave1 != nil {
		t.Fatalf("tr1.saveProgress failed: %v", errSave1)
	}

	// Verify line 1 was saved properly as SRT
	data1, errRead1 := os.ReadFile(outputPath)
	if errRead1 != nil {
		t.Fatalf("failed to read phase 1 output: %v", errRead1)
	}
	parsed1, errParse1 := srt.ParseSRT(string(data1))
	if errParse1 != nil {
		t.Fatalf("phase 1 output is not valid SRT: %v", errParse1)
	}
	if len(parsed1) < 1 || parsed1[0].Content != "你好世界" {
		t.Fatalf("phase 1 output missing translated line 1: %v", parsed1)
	}

	// Phase 2: Resume translation from line 2
	cfg2 := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
		StartLine:      2,
	}
	tr2 := NewTranslator(cfg2)
	if errLoad2 := tr2.loadSubtitles(inputPath); errLoad2 != nil {
		t.Fatalf("tr2.loadSubtitles failed: %v", errLoad2)
	}
	trans2 := []srt.Subtitle{
		{Index: 1, Start: 1 * time.Second, End: 3 * time.Second, Content: tr2.translatedSubtitles[0].Content},
		{Index: 2, Start: 4 * time.Second, End: 6 * time.Second, Content: "第二行"},
	}
	// Simulate model translating line 2 via processTranslatedLines
	batchObj := []srt.SubtitleObject{{Index: 1, Content: "第二行"}}
	if errProcess := tr2.processTranslatedLines(batchObj, trans2, batchObj); errProcess != nil {
		t.Fatalf("tr2.processTranslatedLines failed: %v", errProcess)
	}
	if errSave2 := tr2.saveProgress(2, trans2, nil); errSave2 != nil {
		t.Fatalf("tr2.saveProgress failed: %v", errSave2)
	}

	// Phase 3: Verify line 1 still has translated content and line 2 is translated in SRT
	data2, errRead2 := os.ReadFile(outputPath)
	if errRead2 != nil {
		t.Fatalf("failed to read phase 2 output: %v", errRead2)
	}
	parsed2, errParse2 := srt.ParseSRT(string(data2))
	if errParse2 != nil {
		t.Fatalf("phase 2 output is not valid SRT: %v", errParse2)
	}
	if len(parsed2) != 2 {
		t.Fatalf("expected 2 SRT entries, got %d", len(parsed2))
	}
	if parsed2[0].Content != "你好世界" {
		t.Errorf("resumed output lost line 1 translation: got %q", parsed2[0].Content)
	}
	if parsed2[1].Content != "第二行" {
		t.Errorf("resumed output missing line 2 translation: got %q", parsed2[1].Content)
	}
}

func TestValidateASSTagPlaceholders(t *testing.T) {
	// Matching tags
	if errValidate := validateASSTagPlaceholders("[[ASSTAG0]]Hello[[ASSTAG1]]", "[[ASSTAG0]]你好[[ASSTAG1]]", 1); errValidate != nil {
		t.Errorf("expected matching tags to pass, got: %v", errValidate)
	}

	// Missing tag
	if errValidate := validateASSTagPlaceholders("[[ASSTAG0]]Hello[[ASSTAG1]]", "[[ASSTAG0]]你好", 1); errValidate == nil {
		t.Errorf("expected missing tag to fail")
	}

	// Altered tag
	if errValidate := validateASSTagPlaceholders("[[ASSTAG0]]Hello", "[[ASSTAG1]]你好", 1); errValidate == nil {
		t.Errorf("expected altered tag to fail")
	}

	// Duplicate tag
	if errValidate := validateASSTagPlaceholders("[[ASSTAG0]]Hello", "[[ASSTAG0]][[ASSTAG0]]你好", 1); errValidate == nil {
		t.Errorf("expected duplicate tag to fail")
	}

	// Fabricated tag when original had zero tags
	if errValidate := validateASSTagPlaceholders("Hello", "Bonjour[[ASSTAG0]]", 1); errValidate == nil {
		t.Errorf("expected fabricated tag when original had zero tags to fail")
	}
}

func TestValidateTranslatedResponse_SRTDoesNotCheckASSTags(t *testing.T) {
	tr := &Translator{
		subtitleFormat: video.SubtitleFormatSRT,
	}

	origBatch := []srt.SubtitleObject{
		{Index: 0, Content: "[[ASSTAG0]] literal text"},
	}
	transBatch := []srt.SubtitleObject{
		{Index: 0, Content: "[[ASSTAG 0]] altered text"},
	}

	// For SRT format, altered or missing [[ASSTAG0]] should NOT fail validation
	if errValidate := tr.validateTranslatedResponse(transBatch, origBatch); errValidate != nil {
		t.Errorf("validateTranslatedResponse failed for SRT: %v", errValidate)
	}

	// For ASS format, altered [[ASSTAG0]] MUST fail validation
	trASS := &Translator{
		subtitleFormat: video.SubtitleFormatASS,
	}
	if errValidateASS := trASS.validateTranslatedResponse(transBatch, origBatch); errValidateASS == nil {
		t.Errorf("validateTranslatedResponse should have failed for ASS with altered tags")
	}
}

func TestValidateTranslatedResponse_RejectsOnlyTags(t *testing.T) {
	tr := &Translator{
		subtitleFormat: video.SubtitleFormatASS,
	}

	origBatch := []srt.SubtitleObject{
		{Index: 0, Content: "[[ASSTAG0]]Hello[[ASSTAG1]]"},
	}
	// Model returns only tags, dropped the text!
	transBatch := []srt.SubtitleObject{
		{Index: 0, Content: "[[ASSTAG0]][[ASSTAG1]]"},
	}

	if errValidate := tr.validateTranslatedResponse(transBatch, origBatch); errValidate == nil {
		t.Errorf("validateTranslatedResponse should have failed when model dropped all text outside tags")
	}

	// But if model returns translated text with tags, it succeeds
	validBatch := []srt.SubtitleObject{
		{Index: 0, Content: "[[ASSTAG0]]你好[[ASSTAG1]]"},
	}
	if errValidate := tr.validateTranslatedResponse(validBatch, origBatch); errValidate != nil {
		t.Errorf("validateTranslatedResponse failed on valid translated text: %v", errValidate)
	}

	// If original had "Hello" and model returns raw tag block "{Bonjour}", it should fail
	origNoTags := []srt.SubtitleObject{{Index: 0, Content: "Hello"}}
	rawTagBatch := []srt.SubtitleObject{{Index: 0, Content: "{Bonjour}"}}
	if errValidate := tr.validateTranslatedResponse(rawTagBatch, origNoTags); errValidate == nil {
		t.Errorf("validateTranslatedResponse should have failed when model returned raw tag block {Bonjour}")
	}

	// If model returns only raw layout escapes like \N\h\n with no visible text, it should fail
	rawLayoutBatch := []srt.SubtitleObject{{Index: 0, Content: `\N\h\n`}}
	if errValidate := tr.validateTranslatedResponse(rawLayoutBatch, origNoTags); errValidate == nil {
		t.Errorf("validateTranslatedResponse should have failed when model returned only layout escapes")
	}
}

func TestTranslator_ASSResumePreservesReorderedTags(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
Title: Reorder Tags Test

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,{\c&H0000FF&}blue{\c} and {\c&HFF0000&}red{\c}
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,{\b1}bold{\b0} and {\i1}italic{\i0}
`
	inputPath := filepath.Join(tempDir, "input.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write input ASS file: %v", errWrite)
	}

	outputPath := filepath.Join(tempDir, "output.srt")

	// Phase 1: Translate line 1 (red before blue) and line 2 (italic before bold)
	cfg1 := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
		StartLine:      1,
	}
	tr1 := NewTranslator(cfg1)
	if errLoad1 := tr1.loadSubtitles(inputPath); errLoad1 != nil {
		t.Fatalf("tr1.loadSubtitles failed: %v", errLoad1)
	}
	trans1 := []srt.Subtitle{
		{Index: 1, Start: 1 * time.Second, End: 3 * time.Second, Content: "[[ASSTAG2]]红色[[ASSTAG3]] 和 [[ASSTAG0]]蓝色[[ASSTAG1]]"},
		{Index: 2, Start: 4 * time.Second, End: 6 * time.Second, Content: "[[ASSTAG2]]斜体[[ASSTAG3]] 和 [[ASSTAG0]]粗体[[ASSTAG1]]"},
	}
	batch1 := []srt.SubtitleObject{
		{Index: 0, Content: trans1[0].Content},
		{Index: 1, Content: trans1[1].Content},
	}
	if errProcess1 := tr1.processTranslatedLines(batch1, trans1, batch1); errProcess1 != nil {
		t.Fatalf("tr1.processTranslatedLines failed: %v", errProcess1)
	}
	if errSave1 := tr1.saveProgress(2, trans1, nil); errSave1 != nil {
		t.Fatalf("tr1.saveProgress failed: %v", errSave1)
	}

	// Verify line 1 and line 2 were saved properly in output.srt
	data1, errRead1 := os.ReadFile(outputPath)
	if errRead1 != nil {
		t.Fatalf("failed to read output file: %v", errRead1)
	}
	parsed1, errParse1 := srt.ParseSRT(string(data1))
	if errParse1 != nil {
		t.Fatalf("phase 1 output is not valid SRT: %v", errParse1)
	}
	if len(parsed1) != 2 {
		t.Fatalf("expected 2 SRT entries, got %d", len(parsed1))
	}
	expectedLine1 := "红色 和 蓝色"
	expectedLine2 := "斜体 和 粗体"
	if parsed1[0].Content != expectedLine1 || parsed1[1].Content != expectedLine2 {
		t.Fatalf("phase 1 output does not match expected translated content:\nGot: %q and %q\nWant: %q and %q", parsed1[0].Content, parsed1[1].Content, expectedLine1, expectedLine2)
	}

	// Phase 2: Resume from line 2
	cfg2 := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
		StartLine:      2,
	}
	tr2 := NewTranslator(cfg2)
	if errLoad2 := tr2.loadSubtitles(inputPath); errLoad2 != nil {
		t.Fatalf("tr2.loadSubtitles failed: %v", errLoad2)
	}

	// Initial checkpoint before translation starts MUST NOT corrupt line 2
	if errInitCheck := tr2.saveProgress(1, tr2.translatedSubtitles, nil); errInitCheck != nil {
		t.Fatalf("tr2 initial checkpoint failed: %v", errInitCheck)
	}
	dataInit, errReadInit := os.ReadFile(outputPath)
	if errReadInit != nil {
		t.Fatalf("failed to read initial checkpoint output: %v", errReadInit)
	}
	parsedInit, errParseInit := srt.ParseSRT(string(dataInit))
	if errParseInit != nil {
		t.Fatalf("initial checkpoint output is not valid SRT: %v", errParseInit)
	}
	if parsedInit[1].Content != expectedLine2 {
		t.Fatalf("initial checkpoint corrupted line 2 before translation:\n%s", string(dataInit))
	}

	// Translate line 2 with new translation
	trans2 := []srt.Subtitle{
		{Index: 1, Start: 1 * time.Second, End: 3 * time.Second, Content: tr2.translatedSubtitles[0].Content},
		{Index: 2, Start: 4 * time.Second, End: 6 * time.Second, Content: "全新第二行"},
	}
	batch2 := []srt.SubtitleObject{{Index: 1, Content: "全新第二行"}}
	if errProcess2 := tr2.processTranslatedLines(batch2, trans2, batch2); errProcess2 != nil {
		t.Fatalf("tr2.processTranslatedLines failed: %v", errProcess2)
	}
	if errSave2 := tr2.saveProgress(2, trans2, nil); errSave2 != nil {
		t.Fatalf("tr2.saveProgress failed: %v", errSave2)
	}

	// Phase 3: Verify line 1 STILL has translated content and line 2 was updated
	data2, errRead2 := os.ReadFile(outputPath)
	if errRead2 != nil {
		t.Fatalf("failed to read phase 2 output: %v", errRead2)
	}
	parsed2, errParse2 := srt.ParseSRT(string(data2))
	if errParse2 != nil {
		t.Fatalf("phase 2 output is not valid SRT: %v", errParse2)
	}
	if parsed2[0].Content != expectedLine1 {
		t.Errorf("resumed output modified line 1:\nGot: %q\nWant: %q", parsed2[0].Content, expectedLine1)
	}
	if parsed2[1].Content != "全新第二行" {
		t.Errorf("resumed output missing translated line 2: got %q", parsed2[1].Content)
	}

	// Phase 4: Initial checkpoint preservation when starting from line 1 with existing output file
	cfg3 := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
		StartLine:      1,
	}
	tr3 := NewTranslator(cfg3)
	if errLoad3 := tr3.loadSubtitles(inputPath); errLoad3 != nil {
		t.Fatalf("tr3.loadSubtitles failed: %v", errLoad3)
	}
	// Initial checkpoint before any translation occurs:
	if errSave3 := tr3.saveProgress(1, tr3.translatedSubtitles, nil); errSave3 != nil {
		t.Fatalf("tr3.saveProgress initial checkpoint failed: %v", errSave3)
	}
	data3, errRead3 := os.ReadFile(outputPath)
	if errRead3 != nil {
		t.Fatalf("failed to read phase 3 output: %v", errRead3)
	}
	parsed3, errParse3 := srt.ParseSRT(string(data3))
	if errParse3 != nil {
		t.Fatalf("phase 3 output is not valid SRT: %v", errParse3)
	}
	if parsed3[0].Content != expectedLine1 {
		t.Errorf("initial checkpoint reverted line 1: got %q, want %q", parsed3[0].Content, expectedLine1)
	}
}

func TestNormalizeSubtitleContentForModel_PreservesWindowsPathsInSRT(t *testing.T) {
	srtText := `Open C:\new\notes.txt`
	normalized := normalizeSubtitleContentForModel(srtText)
	if normalized != srtText {
		t.Errorf("normalizeSubtitleContentForModel corrupted path in SRT: got %q, want %q", normalized, srtText)
	}

	multilineSRT := "First line\r\nSecond line\nThird line"
	normalizedMulti := normalizeSubtitleContentForModel(multilineSRT)
	expectedMulti := "First line    Second line    Third line"
	if normalizedMulti != expectedMulti {
		t.Errorf("normalizeSubtitleContentForModel failed on multiline: got %q, want %q", normalizedMulti, expectedMulti)
	}
}

func TestTranslator_InitialCheckpointPreservesUntranslatedLayout(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
Title: Layout Test

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,Top\NBottom\hWord
`
	inputPath := filepath.Join(tempDir, "input.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write input ASS file: %v", errWrite)
	}

	outputPath := filepath.Join(tempDir, "output.srt")
	cfg := &config.Config{
		InputFile:      inputPath,
		OutputFile:     outputPath,
		TargetLanguage: "Simplified Chinese",
		BatchSize:      10,
		StartLine:      1,
	}

	tr := NewTranslator(cfg)
	if errLoad := tr.loadSubtitles(inputPath); errLoad != nil {
		t.Fatalf("loadSubtitles failed: %v", errLoad)
	}

	// Initial checkpoint before any translation occurs
	if errSave := tr.saveProgress(1, tr.translatedSubtitles, nil); errSave != nil {
		t.Fatalf("saveProgress initial checkpoint failed: %v", errSave)
	}

	data, errRead := os.ReadFile(outputPath)
	if errRead != nil {
		t.Fatalf("failed to read output file: %v", errRead)
	}
	parsedInit, errParseInit := srt.ParseSRT(string(data))
	if errParseInit != nil {
		t.Fatalf("initial output is not valid SRT: %v", errParseInit)
	}
	if len(parsedInit) != 1 {
		t.Fatalf("expected 1 SRT entry, got %d", len(parsedInit))
	}

	// Untranslated line in SRT must retain newline and space (\N -> \n, \h -> space)
	expectedUntrans := "Top\nBottom Word"
	if parsedInit[0].Content != expectedUntrans {
		t.Errorf("initial checkpoint mutated untranslated layout: got %q, want %q", parsedInit[0].Content, expectedUntrans)
	}

	// Now simulate model translating this line:
	batch := []srt.SubtitleObject{{Index: 0, Content: "顶部[[ASSTAG0]]底部[[ASSTAG1]]词"}}
	transSubs := []srt.Subtitle{{Index: 1, Content: "顶部[[ASSTAG0]]底部[[ASSTAG1]]词"}}
	if errProcess := tr.processTranslatedLines(batch, transSubs, batch); errProcess != nil {
		t.Fatalf("processTranslatedLines failed: %v", errProcess)
	}
	if errSaveTrans := tr.saveProgress(1, transSubs, nil); errSaveTrans != nil {
		t.Fatalf("saveProgress translated failed: %v", errSaveTrans)
	}
	dataTrans, errReadTrans := os.ReadFile(outputPath)
	if errReadTrans != nil {
		t.Fatalf("failed to read output file after translation: %v", errReadTrans)
	}
	parsedTrans, errParseTrans := srt.ParseSRT(string(dataTrans))
	if errParseTrans != nil {
		t.Fatalf("translated output is not valid SRT: %v", errParseTrans)
	}
	expectedTrans := "顶部\n底部 词"
	if parsedTrans[0].Content != expectedTrans {
		t.Errorf("translated line lost layout escapes, got %q, want %q", parsedTrans[0].Content, expectedTrans)
	}
}

func TestProcessTranslatedLines_RTLWithASSTags(t *testing.T) {
	tr := &Translator{
		subtitleFormat: video.SubtitleFormatASS,
	}

	translatedBatch := []srt.SubtitleObject{
		{Index: 0, Content: "[[ASSTAG0]]مرحبا[[ASSTAG1]]"},
	}
	transSubs := []srt.Subtitle{
		{Index: 1, Content: ""},
	}

	if errProcess := tr.processTranslatedLines(translatedBatch, transSubs, translatedBatch); errProcess != nil {
		t.Fatalf("processTranslatedLines failed: %v", errProcess)
	}

	// Should be wrapped with RTL markers \u202b and \u202c despite [[ASSTAGn]] containing Latin letters
	expected := "\u202b[[ASSTAG0]]مرحبا[[ASSTAG1]]\u202c"
	if transSubs[0].Content != expected {
		t.Errorf("RTL formatting failed: got %q, want %q", transSubs[0].Content, expected)
	}
}

type assTestMockProvider struct {
	translateBatchFunc func(ctx context.Context, batch []srt.SubtitleObject) (*providers.TranslationResponse, error)
}

func (m *assTestMockProvider) GetModels(ctx context.Context) ([]string, error) {
	return []string{"test-model"}, nil
}

func (m *assTestMockProvider) GetTokenLimit(ctx context.Context, modelName string) (int32, error) {
	return 1000, nil
}

func (m *assTestMockProvider) CountTokens(ctx context.Context, modelName string, content string) (int32, error) {
	return 10, nil
}

func (m *assTestMockProvider) TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []providers.ContextMessage, config *providers.TranslationConfig) (*providers.TranslationResponse, error) {
	if m.translateBatchFunc != nil {
		return m.translateBatchFunc(ctx, batch)
	}
	return &providers.TranslationResponse{TranslatedBatch: batch}, nil
}

func (m *assTestMockProvider) GetName() string {
	return "mock"
}

func TestTranslator_ASSInputGeneratesSRTOutputOnly(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
Title: ASS to SRT Test
WrapStyle: 0

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,{\i1}Hello{\i0} world
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,{\pos(100,200)}
Dialogue: 0,0:00:07.00,0:00:09.00,Default,,0,0,0,,Line 1\NLine 2
`
	inputPath := filepath.Join(tempDir, "movie.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write input ASS file: %v", errWrite)
	}

	cfg := config.NewConfig()
	cfg.InputFile = inputPath
	cfg.TargetLanguage = "Simplified Chinese"
	cfg.BatchSize = 10
	cfg.ModelName = "test-model"

	tr := NewTranslator(cfg)
	tr.provider = &assTestMockProvider{
		translateBatchFunc: func(ctx context.Context, batch []srt.SubtitleObject) (*providers.TranslationResponse, error) {
			translated := make([]srt.SubtitleObject, len(batch))
			for i, item := range batch {
				translated[i] = srt.SubtitleObject{
					Index: item.Index,
					Guard: item.Guard,
				}
				if strings.Contains(item.Content, "Hello") {
					translated[i].Content = "[[ASSTAG0]]你好[[ASSTAG1]] 世界"
				} else if strings.Contains(item.Content, "Line 1") {
					translated[i].Content = "第一行[[ASSTAG0]]第二行"
				} else {
					translated[i].Content = item.Content
				}
			}
			return &providers.TranslationResponse{
				TranslatedBatch: translated,
			}, nil
		},
	}

	if errTranslate := tr.Translate(context.Background()); errTranslate != nil {
		t.Fatalf("Translate() failed: %v", errTranslate)
	}

	// Verify default output file is .srt
	expectedSRTPath := filepath.Join(tempDir, "movie.chs.srt")
	if _, errStat := os.Stat(expectedSRTPath); os.IsNotExist(errStat) {
		t.Errorf("expected SRT output file %s does not exist", expectedSRTPath)
	}

	// Verify no .ass output file was created
	assOutputPath := filepath.Join(tempDir, "movie.chs.ass")
	if _, errStatAss := os.Stat(assOutputPath); errStatAss == nil {
		t.Errorf("unexpected ASS output file %s exists", assOutputPath)
	}

	// Verify output content is valid SRT
	content, errRead := os.ReadFile(expectedSRTPath)
	if errRead != nil {
		t.Fatalf("failed to read SRT output: %v", errRead)
	}
	srtContent := string(content)
	if strings.Contains(srtContent, "[Script Info]") || strings.Contains(srtContent, "Dialogue:") {
		t.Errorf("output file contains ASS structure:\n%s", srtContent)
	}

	parsed, errParse := srt.ParseSRT(srtContent)
	if errParse != nil {
		t.Fatalf("output file is not valid SRT: %v", errParse)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 subtitles in SRT output, got %d", len(parsed))
	}
	if parsed[0].Index != 1 || parsed[0].Content != "你好 世界" {
		t.Errorf("unexpected entry 1: %+v", parsed[0])
	}
	if parsed[1].Index != 2 || parsed[1].Content != "第一行\n第二行" {
		t.Errorf("unexpected entry 2: %+v", parsed[1])
	}
}

func TestTranslator_ASSResume_MismatchedSRTCountRestarts(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
Title: Mismatched SRT Test
WrapStyle: 0

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,Hello
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,World
`
	inputPath := filepath.Join(tempDir, "input.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write input ASS file: %v", errWrite)
	}

	// Create an existing output SRT with only 1 entry (mismatch: input has 2 translatable lines)
	outputPath := filepath.Join(tempDir, "output.srt")
	incompleteSRT := "1\n00:00:01,000 --> 00:00:03,000\n你好\n\n"
	if errWriteSRT := os.WriteFile(outputPath, []byte(incompleteSRT), 0644); errWriteSRT != nil {
		t.Fatalf("Failed to write incomplete SRT file: %v", errWriteSRT)
	}

	cfg := config.NewConfig()
	cfg.InputFile = inputPath
	cfg.OutputFile = outputPath
	cfg.TargetLanguage = "Simplified Chinese"
	cfg.StartLine = 2

	tr := NewTranslator(cfg)
	if errLoad := tr.loadSubtitles(inputPath); errLoad != nil {
		t.Fatalf("loadSubtitles failed: %v", errLoad)
	}

	// Mismatched count should cause restartPending to be true and startLine reset to 1
	if !tr.restartPending {
		t.Errorf("expected restartPending to be true due to mismatched SRT count")
	}
	if tr.config.StartLine != 1 {
		t.Errorf("expected startLine to be reset to 1, got %d", tr.config.StartLine)
	}
}

func TestTranslator_ASSResumePreservesLiteralTagPlaceholders(t *testing.T) {
	tempDir := t.TempDir()

	assContent := `[Script Info]
Title: Literal Tag Placeholder Resume Test
WrapStyle: 0

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:03.00,Default,,0,0,0,,{\i1}Hello{\i0}
Dialogue: 0,0:00:04.00,0:00:06.00,Default,,0,0,0,,Second line
`
	inputPath := filepath.Join(tempDir, "input.ass")
	if errWrite := os.WriteFile(inputPath, []byte(assContent), 0644); errWrite != nil {
		t.Fatalf("Failed to write input ASS file: %v", errWrite)
	}

	outputPath := filepath.Join(tempDir, "output.srt")

	// Phase 1: Translate line 1 where translation contains literal text "[[ASSTAG0]]"
	cfg1 := config.NewConfig()
	cfg1.InputFile = inputPath
	cfg1.OutputFile = outputPath
	cfg1.TargetLanguage = "Simplified Chinese"
	cfg1.BatchSize = 10
	cfg1.StartLine = 1

	tr1 := NewTranslator(cfg1)
	if errLoad1 := tr1.loadSubtitles(inputPath); errLoad1 != nil {
		t.Fatalf("tr1.loadSubtitles failed: %v", errLoad1)
	}

	literalText := "Code: [[ASSTAG0]] preserved"
	trans1 := []srt.Subtitle{
		{Index: 1, Start: 1 * time.Second, End: 3 * time.Second, Content: literalText},
		{Index: 2, Start: 4 * time.Second, End: 6 * time.Second, Content: "Second line"},
	}
	// Note: in Phase 1, line 1 is set directly or translated
	tr1.assDialogues[0].SetPlainText(literalText)
	if errSave1 := tr1.saveProgress(1, trans1, nil); errSave1 != nil {
		t.Fatalf("tr1.saveProgress failed: %v", errSave1)
	}

	// Verify Phase 1 output in SRT contains literalText
	data1, errRead1 := os.ReadFile(outputPath)
	if errRead1 != nil {
		t.Fatalf("failed to read phase 1 output: %v", errRead1)
	}
	parsed1, errParse1 := srt.ParseSRT(string(data1))
	if errParse1 != nil {
		t.Fatalf("failed to parse phase 1 SRT: %v", errParse1)
	}
	if parsed1[0].Content != literalText {
		t.Fatalf("phase 1 output does not contain expected literal text: got %q, want %q", parsed1[0].Content, literalText)
	}

	// Phase 2: Resume translation from line 2
	cfg2 := config.NewConfig()
	cfg2.InputFile = inputPath
	cfg2.OutputFile = outputPath
	cfg2.TargetLanguage = "Simplified Chinese"
	cfg2.BatchSize = 10
	cfg2.StartLine = 2

	tr2 := NewTranslator(cfg2)
	if errLoad2 := tr2.loadSubtitles(inputPath); errLoad2 != nil {
		t.Fatalf("tr2.loadSubtitles failed: %v", errLoad2)
	}

	// Initial checkpoint before translation starts
	if errInitCheck := tr2.saveProgress(1, tr2.translatedSubtitles, nil); errInitCheck != nil {
		t.Fatalf("tr2 initial checkpoint failed: %v", errInitCheck)
	}

	// Translate line 2
	trans2 := []srt.Subtitle{
		{Index: 1, Start: 1 * time.Second, End: 3 * time.Second, Content: tr2.translatedSubtitles[0].Content},
		{Index: 2, Start: 4 * time.Second, End: 6 * time.Second, Content: "第二行"},
	}
	batch2 := []srt.SubtitleObject{{Index: 1, Content: "第二行"}}
	if errProcess2 := tr2.processTranslatedLines(batch2, trans2, batch2); errProcess2 != nil {
		t.Fatalf("tr2.processTranslatedLines failed: %v", errProcess2)
	}
	if errSave2 := tr2.saveProgress(2, trans2, nil); errSave2 != nil {
		t.Fatalf("tr2.saveProgress failed: %v", errSave2)
	}

	// Phase 3: Verify line 1 still has literalText intact, not corrupted or stripped
	data2, errRead2 := os.ReadFile(outputPath)
	if errRead2 != nil {
		t.Fatalf("failed to read phase 2 output: %v", errRead2)
	}
	parsed2, errParse2 := srt.ParseSRT(string(data2))
	if errParse2 != nil {
		t.Fatalf("failed to parse phase 2 SRT: %v", errParse2)
	}
	if len(parsed2) != 2 {
		t.Fatalf("expected 2 SRT entries, got %d", len(parsed2))
	}
	if parsed2[0].Content != literalText {
		t.Errorf("resumed output corrupted literal text on line 1: got %q, want %q", parsed2[0].Content, literalText)
	}
	if parsed2[1].Content != "第二行" {
		t.Errorf("resumed output missing line 2 translation: got %q", parsed2[1].Content)
	}
}
