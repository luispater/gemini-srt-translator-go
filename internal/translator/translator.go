package translator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/luispater/gemini-srt-translator-go/internal/logger"
	"github.com/luispater/gemini-srt-translator-go/internal/providers"
	"github.com/luispater/gemini-srt-translator-go/internal/video"
	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/languages"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

// ProgressInfo stores information about translation progress
type ProgressInfo struct {
	Line                   int                        `json:"line"`
	InputFile              string                     `json:"input_file"`
	Provider               string                     `json:"provider,omitempty"`
	Protocol               string                     `json:"protocol,omitempty"`
	Model                  string                     `json:"model,omitempty"`
	TranslationFingerprint string                     `json:"translation_fingerprint,omitempty"`
	PromptCacheKey         string                     `json:"prompt_cache_key,omitempty"`
	SubtitleTrack          int                        `json:"subtitle_track,omitempty"`
	Context                []providers.ContextMessage `json:"context,omitempty"`
}

type translationMetadata struct {
	InputFile         string `json:"input_file"`
	SubtitleTrack     int    `json:"subtitle_track"`
	SourceFingerprint string `json:"source_fingerprint"`
}

type progressTranslationConfig struct {
	BaseURL        string   `json:"base_url"`
	TargetLanguage string   `json:"target_language"`
	Description    string   `json:"description"`
	Thinking       bool     `json:"thinking"`
	ThinkingLevel  string   `json:"thinking_level"`
	Temperature    *float32 `json:"temperature"`
	TopP           *float32 `json:"top_p"`
	TopK           *float32 `json:"top_k"`
}

// ProgressBar wrapper to implement ProgressUpdater interface
type ProgressBarWrapper struct {
	bar *logger.ProgressBar
}

func (p *ProgressBarWrapper) SetLoading(loading bool) {
	if p.bar != nil {
		p.bar.SetLoading(loading)
	}
}

func (p *ProgressBarWrapper) SetThinking(thinking bool) {
	if p.bar != nil {
		p.bar.SetThinking(thinking)
	}
}

// Translator handles the subtitle translation process
type Translator struct {
	config              *config.Config
	provider            providers.TranslationProvider
	batchNumber         int
	tokenLimit          int32
	tokenCount          int32
	translatedBatch     []srt.SubtitleObject
	outputFile          string
	progressFile        string
	logFilePath         string
	thoughtsFilePath    string
	metadataFile        string
	sourceFingerprint   string
	context             []providers.ContextMessage
	progressBar         *logger.ProgressBar
	originalSubtitles   []srt.Subtitle
	translatedSubtitles []srt.Subtitle
	prepared            bool
	restartPending      bool
	extractedSRTFile    string   // Path to SRT file extracted from MKV
	cleanupFiles        []string // Files to clean up after translation
}

// NewTranslator creates a new translator instance
func NewTranslator(cfg *config.Config) *Translator {
	if strings.EqualFold(strings.TrimSpace(cfg.Provider), "openai") &&
		strings.EqualFold(strings.TrimSpace(cfg.OpenAIProtocol), "responses") {
		// A newly constructed translator represents a new task unless progress restores the previous key.
		cfg.OpenAIPromptCacheKey = ""
	}

	baseFile := cfg.InputFile

	var baseName, dirPath string
	if baseFile != "" {
		baseName = strings.TrimSuffix(filepath.Base(baseFile), filepath.Ext(baseFile))
		dirPath = filepath.Dir(baseFile)
	} else {
		baseName = "translated"
		dirPath = ""
	}

	// Set output file path
	outputFile := cfg.OutputFile
	if outputFile == "" {
		suffix := "_translated.srt"

		tl := strings.ToLower(cfg.TargetLanguage)
		if langCode, ok := languages.GetLanguageCode(tl); ok {
			suffix = "." + langCode + ".srt"
		}

		if cfg.InputFile == "" {
			suffix = ".srt"
		}
		if dirPath != "" {
			outputFile = filepath.Join(dirPath, baseName+suffix)
		} else {
			outputFile = baseName + suffix
		}
	}

	// Set progress and log file paths
	var progressFile, logFilePath, thoughtsFilePath string
	if dirPath != "" {
		progressFile = filepath.Join(dirPath, baseName+".progress")
		logFilePath = filepath.Join(dirPath, baseName+".progress.log")
		thoughtsFilePath = filepath.Join(dirPath, baseName+".thoughts.log")
	} else {
		progressFile = baseName + ".progress"
		logFilePath = baseName + ".progress.log"
		thoughtsFilePath = baseName + ".thoughts.log"
	}

	// Create provider
	factory := &providers.ProviderFactory{}
	provider, err := factory.NewProvider(cfg)
	if err != nil {
		// Log error but don't fail - will be handled during translation
		logger.Warning(fmt.Sprintf("Failed to create provider: %v", err))
	}

	return &Translator{
		config:           cfg,
		provider:         provider,
		batchNumber:      1,
		outputFile:       outputFile,
		progressFile:     progressFile,
		logFilePath:      logFilePath,
		thoughtsFilePath: thoughtsFilePath,
		metadataFile:     outputFile + ".gst-meta.json",
		context:          []providers.ContextMessage{},
	}
}

// SetProgressBar assigns a centrally managed progress bar to this task.
func (t *Translator) SetProgressBar(progressBar *logger.ProgressBar) {
	t.progressBar = progressBar
}

// ArtifactPaths returns files that may be written by this translation task.
func (t *Translator) ArtifactPaths() []string {
	paths := []string{t.outputFile, t.progressFile}
	if t.config.ProgressLog {
		paths = append(paths, t.logFilePath)
	}
	if strings.EqualFold(filepath.Ext(t.config.InputFile), ".mkv") {
		paths = append(paths, t.metadataFile)
	}
	return paths
}

// SaveProgressLog writes this task's isolated progress log when enabled.
func (t *Translator) SaveProgressLog() error {
	if !t.config.ProgressLog || t.progressBar == nil {
		return nil
	}
	return t.progressBar.SaveTaskLogsToFile(t.logFilePath)
}

// GetModels returns available models from the provider
func (t *Translator) GetModels(ctx context.Context) ([]string, error) {
	if t.provider == nil {
		return nil, errors.NewValidationError("no provider configured", nil)
	}
	return t.provider.GetModels(ctx)
}

// Prepare performs all file operations and interactive prompts before translation starts.
func (t *Translator) Prepare() (returnErr error) {
	if t.prepared {
		return nil
	}
	defer func() {
		if returnErr != nil {
			t.cleanup()
		}
	}()

	if errValidatePrerequisites := t.validatePrerequisites(); errValidatePrerequisites != nil {
		return errValidatePrerequisites
	}
	if errValidateConfig := t.validateConfig(); errValidateConfig != nil {
		return errValidateConfig
	}

	t.checkSavedProgress()

	srtFile, errPrepareSRT := t.prepareSRTFile()
	if errPrepareSRT != nil {
		return errPrepareSRT
	}
	if errLoadSubtitles := t.loadSubtitles(srtFile); errLoadSubtitles != nil {
		return errLoadSubtitles
	}

	t.prepared = true
	return nil
}

// Translate performs the main translation process.
func (t *Translator) Translate(ctx context.Context) error {
	if errPrepare := t.Prepare(); errPrepare != nil {
		return errPrepare
	}
	defer t.cleanup()

	t.setProgressStatus("Initializing provider")
	if errInitialize := t.initializeTranslationTask(); errInitialize != nil {
		return errInitialize
	}

	t.setProgressStatus("Validating model")
	if errValidateModel := t.validateModel(ctx); errValidateModel != nil {
		return errValidateModel
	}

	t.setProgressStatus("Reading model limits")
	if errGetTokenLimit := t.getTokenLimit(ctx); errGetTokenLimit != nil {
		return errGetTokenLimit
	}

	if t.config.InputFile == "" {
		return fmt.Errorf("no input file provided")
	}
	return t.performTranslation(ctx)
}

func (t *Translator) loadSubtitles(srtFile string) error {
	originalData, errReadOriginal := os.ReadFile(srtFile)
	if errReadOriginal != nil {
		return errors.NewFileError("failed to read input file", errReadOriginal).WithContext("file_path", srtFile)
	}

	sourceHash := sha256.Sum256(originalData)
	t.sourceFingerprint = hex.EncodeToString(sourceHash[:])
	originalSubtitles, errParseOriginal := srt.ParseSRT(string(originalData))
	if errParseOriginal != nil {
		return errors.NewFileError("failed to parse SRT file", errParseOriginal).WithContext("file_path", srtFile)
	}

	loadExistingOutput := false
	if _, errStatOutput := os.Stat(t.outputFile); errStatOutput == nil {
		loadExistingOutput = true
		if strings.EqualFold(filepath.Ext(t.config.InputFile), ".mkv") {
			if matchesSource, mismatchReason := t.outputMetadataMatchesSource(); !matchesSource {
				logger.Warning(fmt.Sprintf("[%s] Existing translation source cannot be verified: %s", filepath.Base(t.config.InputFile), mismatchReason))
				reuseAnswer := strings.ToLower(strings.TrimSpace(logger.InputPrompt("Reuse the existing translation anyway? (y/n): ")))
				loadExistingOutput = reuseAnswer == "y" || reuseAnswer == "yes"
				if !loadExistingOutput {
					t.restartFromBeginning()
				}
			}
		}
	}

	if !loadExistingOutput && t.config.StartLine > 1 {
		logger.Warning(fmt.Sprintf("[%s] Saved progress has no matching output file. Starting from the beginning.", filepath.Base(t.config.InputFile)))
		t.restartFromBeginning()
	}

	var translatedSubtitles []srt.Subtitle
	if loadExistingOutput {
		translatedData, errReadTranslated := os.ReadFile(t.outputFile)
		if errReadTranslated != nil {
			logger.Warning(fmt.Sprintf("Failed to read existing translation. Starting from the beginning: %v", errReadTranslated))
			t.restartFromBeginning()
		} else {
			translatedSubtitles, errReadTranslated = srt.ParseSRT(string(translatedData))
			if errReadTranslated != nil {
				logger.Warning(fmt.Sprintf("Failed to parse existing translation. Starting from the beginning: %v", errReadTranslated))
				t.restartFromBeginning()
			} else {
				logger.Info(fmt.Sprintf("Translated file %s already exists. Loading existing translation...\n", t.outputFile))
				if t.config.StartLine == 0 {
					for {
						input, errPromptLine := logger.InputPromptWithError(fmt.Sprintf("[%s] Enter the line number to start from (1 to %d): ", filepath.Base(t.config.InputFile), len(originalSubtitles)))
						if errPromptLine != nil {
							return errors.NewValidationError("cannot select a start line without interactive input; use --start-line, --resume, or --no-resume", errPromptLine).WithContext("file_path", t.config.InputFile)
						}
						startLine, errParseLine := strconv.Atoi(strings.TrimSpace(input))
						if errParseLine != nil || startLine < 1 || startLine > len(originalSubtitles) {
							logger.Warning(fmt.Sprintf("Line number must be between 1 and %d. Please try again.", len(originalSubtitles)))
							continue
						}
						t.config.StartLine = startLine
						break
					}
				}
			}
		}
	}

	if len(translatedSubtitles) == 0 {
		translatedSubtitles = make([]srt.Subtitle, len(originalSubtitles))
		copy(translatedSubtitles, originalSubtitles)
		t.config.StartLine = 1
	}
	if len(originalSubtitles) != len(translatedSubtitles) {
		return errors.NewValidationError("number of lines of existing translated file does not match the number of lines in the original file", nil).WithContext("original_count", len(originalSubtitles)).WithContext("translated_count", len(translatedSubtitles))
	}
	if t.config.StartLine > len(originalSubtitles) || t.config.StartLine < 1 {
		return errors.NewValidationError(fmt.Sprintf("start line must be between 1 and %d", len(originalSubtitles)), nil).WithContext("start_line", t.config.StartLine).WithContext("max_lines", len(originalSubtitles))
	}
	if len(originalSubtitles) < t.config.BatchSize {
		t.config.BatchSize = len(originalSubtitles)
	}

	t.originalSubtitles = originalSubtitles
	t.translatedSubtitles = translatedSubtitles
	return nil
}

func (t *Translator) outputMetadataMatchesSource() (bool, string) {
	metadataData, errReadMetadata := os.ReadFile(t.metadataFile)
	if errReadMetadata != nil {
		return false, "metadata is missing"
	}
	var metadata translationMetadata
	if errUnmarshalMetadata := json.Unmarshal(metadataData, &metadata); errUnmarshalMetadata != nil {
		return false, "metadata is invalid"
	}
	if filepath.Clean(metadata.InputFile) != filepath.Clean(t.config.InputFile) {
		return false, "input file does not match"
	}
	if metadata.SubtitleTrack != t.config.SubtitleTrack {
		return false, fmt.Sprintf("subtitle track %d does not match selected track %d", metadata.SubtitleTrack, t.config.SubtitleTrack)
	}
	if metadata.SourceFingerprint != t.sourceFingerprint {
		return false, "source subtitles have changed"
	}
	return true, ""
}

func (t *Translator) setProgressStatus(status string) {
	if t.progressBar != nil {
		t.progressBar.SetStatus(status)
	}
}

func (t *Translator) reportProgressMessage(message, color string) {
	if t.progressBar != nil {
		t.progressBar.AddMessage(message, color)
		return
	}
	logger.Warning(message)
}

// validatePrerequisites checks if all prerequisites are met
func (t *Translator) validatePrerequisites() error {
	if t.provider == nil {
		return errors.NewValidationError("no provider configured", nil)
	}

	if t.config.TargetLanguage == "" {
		return errors.NewValidationError("please provide a target language", nil)
	}

	return nil
}

// validateConfig validates the configuration parameters
func (t *Translator) validateConfig() error {
	if t.config.InputFile == "" {
		return errors.NewValidationError("please provide a subtitle file", nil)
	}
	if _, errStatInput := os.Stat(t.config.InputFile); os.IsNotExist(errStatInput) {
		return errors.NewFileError(fmt.Sprintf("input file %s does not exist", t.config.InputFile), errStatInput).WithContext("file_path", t.config.InputFile)
	}

	if t.config.Provider == "openai" {
		switch strings.ToLower(strings.TrimSpace(t.config.OpenAIProtocol)) {
		case "", "chat-completions", "responses":
		default:
			return errors.NewConfigurationError("OpenAI protocol must be one of chat-completions, responses", nil).WithContext("openai_protocol", t.config.OpenAIProtocol)
		}
	}

	switch strings.ToLower(strings.TrimSpace(t.config.ThinkingLevel)) {
	case "minimal", "low", "medium", "high":
	default:
		return errors.NewConfigurationError("thinking level must be one of minimal, low, medium, high", nil).WithContext("thinking_level", t.config.ThinkingLevel)
	}

	if t.config.Temperature != nil && (*t.config.Temperature < 0 || *t.config.Temperature > 2) {
		return errors.NewConfigurationError("temperature must be between 0.0 and 2.0", nil).WithContext("temperature", *t.config.Temperature)
	}

	if t.config.TopP != nil && (*t.config.TopP < 0 || *t.config.TopP > 1) {
		return errors.NewConfigurationError("top P must be between 0.0 and 1.0", nil).WithContext("top_p", *t.config.TopP)
	}

	if t.config.TopK != nil && *t.config.TopK < 0 {
		return errors.NewConfigurationError("top K must be a non-negative integer", nil).WithContext("top_k", *t.config.TopK)
	}

	return nil
}

// checkSavedProgress checks for saved progress and asks user to resume.
func (t *Translator) checkSavedProgress() {
	if t.progressFile == "" || t.config.StartLine != 0 {
		return
	}

	data, errRead := os.ReadFile(t.progressFile)
	if errRead != nil {
		return
	}

	var progress ProgressInfo
	if errUnmarshal := json.Unmarshal(data, &progress); errUnmarshal != nil {
		logger.Warning(fmt.Sprintf("Error reading progress file: %v", errUnmarshal))
		return
	}

	// Verify the progress file matches our current input file.
	if progress.InputFile != t.config.InputFile {
		logger.Warning(fmt.Sprintf("Found progress file for different subtitle: %s", progress.InputFile))
		logger.Warning("Ignoring saved progress.")
		return
	}
	if progress.Line < 1 {
		logger.Warning("Ignoring saved progress with an invalid line number.")
		return
	}
	if progress.SubtitleTrack != 0 && t.config.SubtitleTrack != 0 && progress.SubtitleTrack != t.config.SubtitleTrack {
		logger.Warning(fmt.Sprintf("Saved progress uses subtitle track %d, but track %d was selected. Starting from the beginning.", progress.SubtitleTrack, t.config.SubtitleTrack))
		t.restartFromBeginning()
		return
	}

	shouldResume := true
	if progress.Line > 1 {
		var resume string
		if t.config.Resume == nil {
			resume = strings.ToLower(strings.TrimSpace(logger.InputPrompt(fmt.Sprintf("[%s] Found saved progress at line %d. Resume? (y/n): ", filepath.Base(t.config.InputFile), progress.Line))))
		} else if *t.config.Resume {
			resume = "y"
		} else {
			resume = "n"
		}
		shouldResume = resume == "y" || resume == "yes"
	} else if t.config.Resume != nil && !*t.config.Resume {
		shouldResume = false
	}

	if shouldResume {
		if progress.Line > 1 {
			logger.Info(fmt.Sprintf("Resuming from line %d", progress.Line))
		}
		t.config.StartLine = progress.Line
		if t.usesOpenAIResponses() && t.progressContextMatches(progress) {
			t.context = append([]providers.ContextMessage(nil), progress.Context...)
			t.config.OpenAIPromptCacheKey = progress.PromptCacheKey
		} else if t.usesOpenAIResponses() && (progress.PromptCacheKey != "" || len(progress.Context) > 0) {
			logger.Warning("Saved Responses progress does not match the current provider, protocol, model, or translation configuration. Ignoring saved context and prompt cache key.")
		}
		return
	}

	logger.Info("Starting from the beginning")
	t.restartFromBeginning()
}

func (t *Translator) restartFromBeginning() {
	t.config.StartLine = 0
	t.context = nil
	t.restartPending = true
	if t.usesOpenAIResponses() {
		t.config.OpenAIPromptCacheKey = ""
	}
}

func (t *Translator) initializeTranslationTask() error {
	initializer, supportsInitialization := t.provider.(providers.TranslationTaskInitializer)
	if !supportsInitialization {
		return nil
	}
	return initializer.InitializeTranslationTask()
}

type artifactBackup struct {
	path       string
	backupPath string
}

// saveProgress writes translated output before recording the matching progress and context.
func (t *Translator) saveProgress(line int, translatedSubtitles []srt.Subtitle, contextMessages []providers.ContextMessage) (returnErr error) {
	var backups []artifactBackup
	if t.restartPending {
		var errBackup error
		backups, errBackup = t.backupRestartArtifacts()
		if errBackup != nil {
			return errBackup
		}
		defer func() {
			if returnErr != nil {
				returnErr = stdErrors.Join(returnErr, restoreArtifactBackups(backups))
				return
			}
			t.restartPending = false
			returnErr = stdErrors.Join(returnErr, removeArtifactBackups(backups))
		}()
	}

	translatedContent := srt.ComposeSRT(translatedSubtitles)
	if errWriteOutput := writeFileAtomically(t.outputFile, []byte(translatedContent), 0644); errWriteOutput != nil {
		return fmt.Errorf("failed to write output file %s: %w", t.outputFile, errWriteOutput)
	}
	if strings.EqualFold(filepath.Ext(t.config.InputFile), ".mkv") {
		if errSaveMetadata := t.saveTranslationMetadata(); errSaveMetadata != nil {
			return errSaveMetadata
		}
	}
	if t.progressFile == "" {
		return nil
	}

	progress := ProgressInfo{
		Line:          line,
		InputFile:     t.config.InputFile,
		SubtitleTrack: t.config.SubtitleTrack,
	}
	if t.usesOpenAIResponses() {
		fingerprint, errFingerprint := t.translationConfigFingerprint()
		if errFingerprint != nil {
			return fmt.Errorf("failed to fingerprint translation configuration: %w", errFingerprint)
		}
		progress.Provider = strings.ToLower(strings.TrimSpace(t.config.Provider))
		progress.Protocol = strings.ToLower(strings.TrimSpace(t.config.OpenAIProtocol))
		progress.Model = t.config.ModelName
		progress.TranslationFingerprint = fingerprint
		progress.PromptCacheKey = t.config.OpenAIPromptCacheKey
		progress.Context = append([]providers.ContextMessage(nil), contextMessages...)
	}

	data, errMarshal := json.Marshal(progress)
	if errMarshal != nil {
		return fmt.Errorf("failed to marshal progress: %w", errMarshal)
	}
	if errWriteProgress := writeFileAtomically(t.progressFile, data, 0644); errWriteProgress != nil {
		return fmt.Errorf("failed to write progress file %s: %w", t.progressFile, errWriteProgress)
	}
	return nil
}

func (t *Translator) backupRestartArtifacts() ([]artifactBackup, error) {
	artifactPaths := []string{t.outputFile, t.progressFile}
	if strings.EqualFold(filepath.Ext(t.config.InputFile), ".mkv") {
		artifactPaths = append(artifactPaths, t.metadataFile)
	}

	var backups []artifactBackup
	for _, artifactPath := range artifactPaths {
		fileInfo, errStatArtifact := os.Stat(artifactPath)
		if os.IsNotExist(errStatArtifact) {
			backups = append(backups, artifactBackup{path: artifactPath})
			continue
		}
		if errStatArtifact != nil {
			return nil, stdErrors.Join(errStatArtifact, restoreArtifactBackups(backups))
		}
		if !fileInfo.Mode().IsRegular() {
			return nil, stdErrors.Join(fmt.Errorf("cannot back up non-regular artifact %s", artifactPath), restoreArtifactBackups(backups))
		}

		backupFile, errCreateBackup := os.CreateTemp(filepath.Dir(artifactPath), "."+filepath.Base(artifactPath)+".backup-*")
		if errCreateBackup != nil {
			return nil, stdErrors.Join(errCreateBackup, restoreArtifactBackups(backups))
		}
		backupPath := backupFile.Name()
		if errCloseBackup := backupFile.Close(); errCloseBackup != nil {
			_ = os.Remove(backupPath)
			return nil, stdErrors.Join(errCloseBackup, restoreArtifactBackups(backups))
		}
		if errRemovePlaceholder := os.Remove(backupPath); errRemovePlaceholder != nil {
			return nil, stdErrors.Join(errRemovePlaceholder, restoreArtifactBackups(backups))
		}
		if errRenameArtifact := os.Rename(artifactPath, backupPath); errRenameArtifact != nil {
			return nil, stdErrors.Join(errRenameArtifact, restoreArtifactBackups(backups))
		}
		backups = append(backups, artifactBackup{path: artifactPath, backupPath: backupPath})
	}
	return backups, nil
}

func restoreArtifactBackups(backups []artifactBackup) error {
	var restoreErr error
	for backupIndex := len(backups) - 1; backupIndex >= 0; backupIndex-- {
		backup := backups[backupIndex]
		if errRemoveCurrent := os.Remove(backup.path); errRemoveCurrent != nil && !os.IsNotExist(errRemoveCurrent) {
			restoreErr = stdErrors.Join(restoreErr, fmt.Errorf("failed to remove incomplete artifact %s: %w", backup.path, errRemoveCurrent))
			continue
		}
		if backup.backupPath == "" {
			continue
		}
		if errRestore := os.Rename(backup.backupPath, backup.path); errRestore != nil {
			restoreErr = stdErrors.Join(restoreErr, fmt.Errorf("failed to restore artifact %s: %w", backup.path, errRestore))
		}
	}
	return restoreErr
}

func removeArtifactBackups(backups []artifactBackup) error {
	var removeErr error
	for _, backup := range backups {
		if backup.backupPath == "" {
			continue
		}
		if errRemove := os.Remove(backup.backupPath); errRemove != nil && !os.IsNotExist(errRemove) {
			removeErr = stdErrors.Join(removeErr, fmt.Errorf("failed to remove artifact backup %s: %w", backup.backupPath, errRemove))
		}
	}
	return removeErr
}

func (t *Translator) saveTranslationMetadata() error {
	metadata := translationMetadata{
		InputFile:         t.config.InputFile,
		SubtitleTrack:     t.config.SubtitleTrack,
		SourceFingerprint: t.sourceFingerprint,
	}
	metadataData, errMarshalMetadata := json.Marshal(metadata)
	if errMarshalMetadata != nil {
		return fmt.Errorf("failed to marshal translation metadata: %w", errMarshalMetadata)
	}
	if errWriteMetadata := writeFileAtomically(t.metadataFile, metadataData, 0644); errWriteMetadata != nil {
		return fmt.Errorf("failed to write translation metadata %s: %w", t.metadataFile, errWriteMetadata)
	}
	return nil
}

func writeFileAtomically(path string, data []byte, permissions os.FileMode) (returnErr error) {
	directory := filepath.Dir(path)
	tempFile, errCreate := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if errCreate != nil {
		return errCreate
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if tempFile != nil {
			if errClose := tempFile.Close(); errClose != nil {
				returnErr = stdErrors.Join(returnErr, fmt.Errorf("failed to close temporary file %s: %w", tempPath, errClose))
			}
		}
		if removeTemp {
			if errRemove := os.Remove(tempPath); errRemove != nil && !os.IsNotExist(errRemove) {
				returnErr = stdErrors.Join(returnErr, fmt.Errorf("failed to remove temporary file %s: %w", tempPath, errRemove))
			}
		}
	}()

	if errChmod := tempFile.Chmod(permissions); errChmod != nil {
		return fmt.Errorf("failed to set temporary file permissions: %w", errChmod)
	}
	bytesWritten, errWrite := tempFile.Write(data)
	if errWrite != nil {
		return fmt.Errorf("failed to write temporary file: %w", errWrite)
	}
	if bytesWritten != len(data) {
		return fmt.Errorf("failed to write complete temporary file: wrote %d of %d bytes", bytesWritten, len(data))
	}
	if errClose := tempFile.Close(); errClose != nil {
		tempFile = nil
		return fmt.Errorf("failed to close temporary file %s: %w", tempPath, errClose)
	}
	tempFile = nil

	if errRename := os.Rename(tempPath, path); errRename != nil {
		return fmt.Errorf("failed to atomically replace %s: %w", path, errRename)
	}
	removeTemp = false
	return nil
}

func (t *Translator) usesOpenAIResponses() bool {
	return strings.EqualFold(strings.TrimSpace(t.config.Provider), "openai") &&
		strings.EqualFold(strings.TrimSpace(t.config.OpenAIProtocol), "responses")
}

func (t *Translator) progressContextMatches(progress ProgressInfo) bool {
	fingerprint, errFingerprint := t.translationConfigFingerprint()
	if errFingerprint != nil {
		return false
	}
	return progress.InputFile == t.config.InputFile &&
		strings.EqualFold(strings.TrimSpace(progress.Provider), strings.TrimSpace(t.config.Provider)) &&
		strings.EqualFold(strings.TrimSpace(progress.Protocol), strings.TrimSpace(t.config.OpenAIProtocol)) &&
		progress.Model == t.config.ModelName &&
		progress.TranslationFingerprint == fingerprint
}

func (t *Translator) translationConfigFingerprint() (string, error) {
	fingerprintConfig := progressTranslationConfig{
		BaseURL:        normalizeProgressBaseURL(t.config.BaseURL),
		TargetLanguage: t.config.TargetLanguage,
		Description:    t.config.Description,
		Thinking:       t.config.Thinking,
		ThinkingLevel:  t.config.ThinkingLevel,
		Temperature:    t.config.Temperature,
		TopP:           t.config.TopP,
		TopK:           t.config.TopK,
	}
	data, errMarshal := json.Marshal(fingerprintConfig)
	if errMarshal != nil {
		return "", errMarshal
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeProgressBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// validateModel checks if the specified model is available
func (t *Translator) validateModel(ctx context.Context) error {
	models, err := t.GetModels(ctx)
	if err != nil {
		return err
	}

	for _, model := range models {
		if strings.Contains(model, t.config.ModelName) {
			return nil
		}
	}

	return fmt.Errorf("model %s is not available. Please choose a different model", t.config.ModelName)
}

// getTokenLimit retrieves the token limit for the current model
func (t *Translator) getTokenLimit(ctx context.Context) error {
	tokenLimit, err := t.provider.GetTokenLimit(ctx, t.config.ModelName)
	if err != nil {
		return err
	}

	t.tokenLimit = tokenLimit
	return nil
}

// performTranslation performs the main translation process
func (t *Translator) performTranslation(ctx context.Context) error {
	if len(t.originalSubtitles) == 0 {
		srtFile, errPrepareSRT := t.prepareSRTFile()
		if errPrepareSRT != nil {
			return errPrepareSRT
		}
		if errLoadSubtitles := t.loadSubtitles(srtFile); errLoadSubtitles != nil {
			return errLoadSubtitles
		}
	}

	originalSubtitles := t.originalSubtitles
	translatedSubtitles := t.translatedSubtitles
	var err error

	// Setup delay for pro models with free quota (only for Gemini)
	delay := false
	delayTime := 30 * time.Second

	if t.provider.GetName() == "gemini" && strings.Contains(t.config.ModelName, "pro") && t.config.FreeQuota {
		delay = true
		delayTime = 15 * time.Second
	}

	progressBar := t.progressBar
	if progressBar == nil {
		progressBar = logger.NewProgressBar(len(originalSubtitles), "Translating:")
	} else {
		progressBar.SetTotal(len(originalSubtitles))
	}
	defer progressBar.Stop()

	progressBar.SetStatus("Translating")
	progressBar.SetSuffix(t.config.ModelName)
	progressBar.SetSending(true)

	i := t.config.StartLine - 1

	total := len(originalSubtitles)
	var batch []srt.SubtitleObject

	// Build compatibility context from previous translations when legacy progress has no saved context.
	if t.config.StartLine > 1 && len(t.context) == 0 {
		startIdx := max(0, t.config.StartLine-2-t.config.BatchSize)
		var userBatch []srt.SubtitleObject
		var modelBatch []srt.SubtitleObject

		for j := startIdx; j < t.config.StartLine-1; j++ {
			objUser := srt.SubtitleObject{
				Index:   j,
				Content: normalizeSubtitleContentForModel(originalSubtitles[j].Content),
				Guard:   t.lineGuard(j),
			}

			objModel := srt.SubtitleObject{
				Index:   j,
				Content: normalizeSubtitleContentForModel(translatedSubtitles[j].Content),
				Guard:   t.lineGuard(j),
			}

			userBatch = append(userBatch, objUser)
			modelBatch = append(modelBatch, objModel)
		}

		userData, _ := json.Marshal(userBatch)
		modelData, _ := json.Marshal(modelBatch)

		t.context = []providers.ContextMessage{
			{Role: "user", Content: string(userData)},
			{Role: "model", Content: string(modelData)},
		}
	}

	progressBar.Update(i)

	// Add first subtitle to batch
	obj := srt.SubtitleObject{
		Index:   i,
		Content: originalSubtitles[i].Content,
	}
	batch = append(batch, obj)
	i++

	// Preserve previous artifacts until the first translated checkpoint succeeds.
	if !t.restartPending {
		if errSaveProgress := t.saveProgress(i, translatedSubtitles, t.context); errSaveProgress != nil {
			return errSaveProgress
		}
	}

	// Main translation loop
	for i < total || len(batch) > 0 {
		// Build batch
		for i < total && len(batch) < t.config.BatchSize {
			subtitleObj := srt.SubtitleObject{
				Index:   i,
				Content: originalSubtitles[i].Content,
			}
			batch = append(batch, subtitleObj)
			i++
		}

		// Validate token size
		guardedBatch := t.withLineGuards(batch)
		if err = t.validateTokenSize(ctx, guardedBatch); err != nil {
			return err
		}

		if i == total && len(batch) < t.config.BatchSize {
			t.config.BatchSize = len(batch)
		}

		// Process batch
		startTime := time.Now()
		newContext, errProcessBatch := t.processBatch(ctx, guardedBatch, translatedSubtitles, progressBar)
		if errProcessBatch != nil {
			return errProcessBatch
		}
		endTime := time.Now()

		// Persist output and progress before advancing the in-memory context.
		progressBar.Update(i)
		nextProgressLine := min(i+1, total)
		if errSaveProgress := t.saveProgress(nextProgressLine, translatedSubtitles, newContext); errSaveProgress != nil {
			return errSaveProgress
		}
		t.context = newContext

		// Apply delay if needed
		if delay {
			elapsed := endTime.Sub(startTime)
			if elapsed < delayTime && i < total {
				time.Sleep(delayTime - elapsed)
			}
		}

		// Clear batch for next iteration
		batch = nil
	}

	progressBar.Update(len(originalSubtitles))

	// Stop the progress bar rendering goroutine
	progressBar.Stop()

	t.removeCompletedTaskFiles()
	return nil
}

func (t *Translator) removeCompletedTaskFiles() {
	if t.progressFile != "" {
		if errRemoveProgress := os.Remove(t.progressFile); errRemoveProgress != nil && !os.IsNotExist(errRemoveProgress) {
			t.reportProgressMessage(fmt.Sprintf("Failed to remove progress file: %v", errRemoveProgress), logger.Yellow)
		}
	}
	if t.metadataFile != "" {
		if errRemoveMetadata := os.Remove(t.metadataFile); errRemoveMetadata != nil && !os.IsNotExist(errRemoveMetadata) {
			t.reportProgressMessage(fmt.Sprintf("Failed to remove translation metadata: %v", errRemoveMetadata), logger.Yellow)
		}
	}
}

// validateTokenSize validates that the batch doesn't exceed token limits
func (t *Translator) validateTokenSize(ctx context.Context, batch []srt.SubtitleObject) error {
	batchData, err := json.Marshal(batch)
	if err != nil {
		return errors.NewTranslationError("failed to marshal batch", err)
	}

	tokenCount, err := t.provider.CountTokens(ctx, t.config.ModelName, string(batchData))
	if err != nil {
		return errors.NewAPIError("failed to count tokens", err)
	}

	t.tokenCount = tokenCount

	// Do not prompt while concurrent tasks own the terminal; fail this task with actionable context.
	if t.tokenLimit != 0 && float64(tokenCount) > float64(t.tokenLimit)*0.9 {
		return errors.NewValidationError("batch size too large, please retry with a smaller --batch-size", nil).WithContext("current_batch_size", t.config.BatchSize).WithContext("token_count", tokenCount).WithContext("token_limit", t.tokenLimit)
	}

	return nil
}

// withLineGuards returns a copy of the batch prepared for model input.
func (t *Translator) withLineGuards(batch []srt.SubtitleObject) []srt.SubtitleObject {
	guardedBatch := make([]srt.SubtitleObject, len(batch))
	for i, item := range batch {
		item.Content = normalizeSubtitleContentForModel(item.Content)
		item.Guard = t.lineGuard(item.Index)
		guardedBatch[i] = item
	}
	return guardedBatch
}

// normalizeSubtitleContentForModel keeps one subtitle object as one text unit.
func normalizeSubtitleContentForModel(content string) string {
	replacer := strings.NewReplacer(
		"\r\n", "    ",
		"\n", "    ",
		"\r", "    ",
	)
	return replacer.Replace(content)
}

// lineGuard returns the expected guard token for a subtitle index.
func (t *Translator) lineGuard(index int) string {
	return fmt.Sprintf("GST_LINE_%06d", index)
}

// processBatch processes a single batch of subtitles with retry logic
func (t *Translator) processBatch(ctx context.Context, batch []srt.SubtitleObject, translatedSubtitles []srt.Subtitle, progressBar *logger.ProgressBar) ([]providers.ContextMessage, error) {
	var lastErr error
	retryInstruction := ""
	progressWrapper := &ProgressBarWrapper{bar: progressBar}

	for attempt := 0; attempt <= t.config.RetryCount; attempt++ {
		if attempt > 0 {
			progressBar.PrintErrorAbove(fmt.Sprintf("Retry attempt %d/%d", attempt, t.config.RetryCount), logger.Yellow)
			progressBar.AddRetry()
			retryInstruction = t.buildRetryInstruction(lastErr)

			// Try to switch API key if provider supports it
			if keySwitcher, ok := t.provider.(providers.KeySwitcher); ok {
				if keySwitcher.SwitchAPIKey() {
					progressBar.PrintErrorAbove(fmt.Sprintf("Switching to API Key %d", keySwitcher.GetCurrentAPIKeyIndex()+1), logger.Yellow)
				}
			}

			// Add small delay between retries
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		result, errProcess := t.processBatchAttempt(ctx, batch, translatedSubtitles, progressWrapper, retryInstruction)
		if errProcess == nil {
			// No need to clear messages anymore - errors stay in terminal history
			return result, nil
		}

		lastErr = errProcess
		progressBar.PrintErrorAbove(fmt.Sprintf("Batch processing failed (attempt %d/%d): %v", attempt+1, t.config.RetryCount+1, errProcess), logger.Red)
	}

	return nil, fmt.Errorf("batch processing failed after %d retries: %w", t.config.RetryCount, lastErr)
}

// buildRetryInstruction creates correction instructions for the next retry.
func (t *Translator) buildRetryInstruction(err error) string {
	if err == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("The previous translation attempt failed. Regenerate the complete response from scratch; do not continue, reuse, or patch the previous invalid response.\n")
	builder.WriteString(fmt.Sprintf("Previous failure reason: %v\n", err))
	builder.WriteString("This attempt must return exactly one valid JSON array only; do not return multiple arrays and do not use Markdown code fences.\n")
	builder.WriteString("The JSON must be directly parseable by Go's json.Unmarshal.\n")
	builder.WriteString("No content string may contain unescaped literal line breaks, carriage returns, or control characters; replace any line break, carriage return, or source subtitle \\n, \\r, or \\r\\n with four spaces.\n")
	builder.WriteString("Treat each input object as one complete subtitle unit. Do not split one content value into multiple output objects because it contains escaped line separators, visual line breaks, or multiple short phrases.\n")
	builder.WriteString("The output object count, order, index values, and guard values must exactly match the current input array.\n")
	builder.WriteString("Copy each guard value unchanged. Do not translate, remove, rename, or move guard values between objects.\n")

	var translatorErr *errors.TranslatorError
	if stdErrors.As(err, &translatorErr) {
		if responseText, ok := translatorErr.Context["response_text"].(string); ok && responseText != "" {
			builder.WriteString("\nA relevant excerpt from the previous invalid response is shown below. Do not repeat its formatting errors:\n")
			builder.WriteString(t.responseErrorExcerpt(err, responseText))
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// responseErrorExcerpt returns a small escaped excerpt around a JSON parse error.
func (t *Translator) responseErrorExcerpt(err error, responseText string) string {
	const excerptRadius = 240

	offset := 0
	var syntaxErr *json.SyntaxError
	if stdErrors.As(err, &syntaxErr) && syntaxErr.Offset > 0 {
		offset = int(syntaxErr.Offset) - 1
	}
	if offset < 0 || offset >= len(responseText) {
		offset = 0
	}

	start := offset - excerptRadius
	if start < 0 {
		start = 0
	}
	end := offset + excerptRadius
	if end > len(responseText) {
		end = len(responseText)
	}

	return strconv.Quote(responseText[start:end])
}

// processBatchAttempt performs a single attempt to process a batch
func (t *Translator) processBatchAttempt(ctx context.Context, batch []srt.SubtitleObject, translatedSubtitles []srt.Subtitle, progressWrapper *ProgressBarWrapper, retryInstruction string) ([]providers.ContextMessage, error) {
	// Create translation config
	translationConfig := &providers.TranslationConfig{
		ModelName:        t.config.ModelName,
		TargetLanguage:   t.config.TargetLanguage,
		Description:      t.config.Description,
		RetryInstruction: retryInstruction,
		Temperature:      t.config.Temperature,
		TopP:             t.config.TopP,
		TopK:             t.config.TopK,
		Streaming:        t.config.Streaming,
		Thinking:         t.config.Thinking,
		ThinkingLevel:    t.config.ThinkingLevel,
		ProgressUpdater:  progressWrapper,
	}

	// Call provider to translate batch
	response, err := t.provider.TranslateBatch(ctx, batch, t.context, translationConfig)
	if err != nil {
		return nil, err
	}

	// Validate response content
	if errValidate := t.validateTranslatedResponse(response.TranslatedBatch, batch); errValidate != nil {
		return nil, errValidate
	}

	// Store successful translation
	t.translatedBatch = response.TranslatedBatch

	// Process translated lines
	if err = t.processTranslatedLines(t.translatedBatch, translatedSubtitles, batch); err != nil {
		return nil, err
	}

	t.batchNumber++

	// Preserve every successful turn so subsequent batches receive the full conversation.
	nextContext := make([]providers.ContextMessage, 0, len(t.context)+len(response.Context))
	nextContext = append(nextContext, t.context...)
	nextContext = append(nextContext, response.Context...)

	return nextContext, nil
}

// processTranslatedLines processes the translated subtitle lines
func (t *Translator) processTranslatedLines(translatedLines []srt.SubtitleObject, translatedSubtitles []srt.Subtitle, batch []srt.SubtitleObject) error {
	// Create index map from batch
	indexMap := make(map[int]int)
	for i, item := range batch {
		indexMap[item.Index] = i
	}

	// Process each translated line
	for _, line := range translatedLines {
		index := line.Index

		// Apply RTL detection and formatting
		if t.isDominantRTL(line.Content) {
			translatedSubtitles[index].Content = "\u202b" + line.Content + "\u202c"
		} else if len(line.Content) == 0 {
			translatedSubtitles[index].Content = " "
		} else {
			translatedSubtitles[index].Content = line.Content
		}
	}

	return nil
}

// validateTranslatedResponse validates the translated response content
func (t *Translator) validateTranslatedResponse(translatedBatch []srt.SubtitleObject, originalBatch []srt.SubtitleObject) error {
	if len(translatedBatch) != len(originalBatch) {
		return errors.NewTranslationError(fmt.Sprintf("provider returned unexpected response. Expected %d lines, got %d", len(originalBatch), len(translatedBatch)), nil).WithContext("expected_count", len(originalBatch)).WithContext("actual_count", len(translatedBatch))
	}

	for i, translated := range translatedBatch {
		original := originalBatch[i]
		if translated.Index != original.Index {
			return errors.NewTranslationError(fmt.Sprintf("provider returned mismatched index at position %d. Expected %d, got %d", i, original.Index, translated.Index), nil).WithContext("position", i).WithContext("expected_index", original.Index).WithContext("actual_index", translated.Index)
		}
		if original.Guard != "" && translated.Guard != original.Guard {
			return errors.NewTranslationError(fmt.Sprintf("provider returned mismatched guard for line %d", original.Index), nil).WithContext("line_index", original.Index).WithContext("expected_guard", original.Guard).WithContext("actual_guard", translated.Guard)
		}
		if translated.Content == "" && original.Content != "" && !t.isOnlyPunctuation(original.Content) {
			return errors.NewTranslationError(fmt.Sprintf("provider returned an empty translation for line %d", translated.Index), nil).WithContext("line_index", translated.Index)
		}
	}

	return nil
}

// isOnlyPunctuation checks if the text contains only punctuation marks and whitespace
func (t *Translator) isOnlyPunctuation(text string) bool {
	if text == "" {
		return false
	}

	for _, r := range text {
		if !unicode.IsPunct(r) && !unicode.IsSpace(r) {
			return false
		}
	}

	return true
}

// isDominantRTL determines if text is predominantly right-to-left
func (t *Translator) isDominantRTL(text string) bool {
	rtlCount := 0
	ltrCount := 0

	for _, r := range text {
		switch unicode.In(r, unicode.Arabic, unicode.Hebrew) {
		case true:
			rtlCount++
		default:
			if unicode.In(r, unicode.Latin) {
				ltrCount++
			}
		}
	}

	return rtlCount > ltrCount
}

// prepareSRTFile prepares the SRT file for translation (extracts from MKV if needed)
func (t *Translator) prepareSRTFile() (string, error) {
	inputFile := t.config.InputFile

	// Check if input is an MKV file
	if strings.HasSuffix(strings.ToLower(inputFile), ".mkv") {
		logger.Info(fmt.Sprintf("[%s] Extracting subtitle track %d...", filepath.Base(inputFile), t.config.SubtitleTrack))
		newExtractedPath, errExtract := video.ExtractSubtitlesFromMKV(inputFile, t.config.SubtitleTrack)
		if errExtract != nil {
			return "", errors.NewFileError("failed to extract subtitles from MKV file", errExtract).WithContext("mkv_path", inputFile)
		}

		t.extractedSRTFile = newExtractedPath
		t.cleanupFiles = append(t.cleanupFiles, newExtractedPath)
		logger.Success(fmt.Sprintf("[%s] Subtitles extracted to: %s", filepath.Base(inputFile), newExtractedPath))
		return newExtractedPath, nil
	}

	// For SRT files, return the original path
	return inputFile, nil
}

// cleanup removes temporary files created during translation
func (t *Translator) cleanup() {
	for _, file := range t.cleanupFiles {
		if errRemove := os.Remove(file); errRemove != nil && !os.IsNotExist(errRemove) {
			t.reportProgressMessage(fmt.Sprintf("Failed to remove temporary file %s: %v", file, errRemove), logger.Yellow)
		}
	}
}
