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

	"github.com/luispater/gemini-srt-translator-go/internal/logger"
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
