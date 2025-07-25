package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"google.golang.org/genai"

	"github.com/luispater/gemini-srt-translator-go/internal/helpers"
	"github.com/luispater/gemini-srt-translator-go/internal/logger"
	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

// ProgressInfo stores information about translation progress
type ProgressInfo struct {
	Line      int    `json:"line"`
	InputFile string `json:"input_file"`
}

// Translator handles the subtitle translation process
type Translator struct {
	config           *config.Config
	client           *genai.Client
	apiKeys          []string
	currentAPIIndex  int
	batchNumber      int
	tokenLimit       int32
	tokenCount       int32
	translatedBatch  []srt.SubtitleObject
	outputFile       string
	progressFile     string
	logFilePath      string
	thoughtsFilePath string
}

// NewTranslator creates a new translator instance
func NewTranslator(cfg *config.Config) *Translator {
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

	return &Translator{
		config:           cfg,
		apiKeys:          cfg.GeminiAPIKeys,
		currentAPIIndex:  0,
		batchNumber:      1,
		outputFile:       outputFile,
		progressFile:     progressFile,
		logFilePath:      logFilePath,
		thoughtsFilePath: thoughtsFilePath,
	}
}

// getCurrentAPIKey returns the current API key if available
func (t *Translator) getCurrentAPIKey() string {
	if len(t.apiKeys) == 0 || t.currentAPIIndex >= len(t.apiKeys) {
		return ""
	}
	return t.apiKeys[t.currentAPIIndex]
}

// hasAPIKeys returns true if there are API keys available
func (t *Translator) hasAPIKeys() bool {
	return len(t.apiKeys) > 0
}

// GetModels returns available Gemini models
func (t *Translator) GetModels(ctx context.Context) ([]string, error) {
	if !t.hasAPIKeys() {
		return nil, errors.NewValidationError("please provide a valid Gemini API key", nil)
	}

	client, err := helpers.CreateClient(ctx, t.config, t.getCurrentAPIKey())
	if err != nil {
		return nil, errors.NewAPIError("failed to create Gemini client", err)
	}
	defer func() {
		// Client does not need explicit closing in new API
	}()

	return helpers.ListModels(ctx, client)
}

// ListModels prints available models to console

// Translate performs the main translation process
func (t *Translator) Translate(ctx context.Context) error {
	// Validate prerequisites
	if err := t.validatePrerequisites(); err != nil {
		return err
	}

	// Validate configuration
	if err := t.validateConfig(); err != nil {
		return err
	}

	// Check saved progress
	t.checkSavedProgress()

	// Validate model availability
	if err := t.validateModel(ctx); err != nil {
		return err
	}

	// Get token limit
	if err := t.getTokenLimit(ctx); err != nil {
		return err
	}

	// Perform translation
	if t.config.InputFile != "" {
		return t.performTranslation(ctx)
	}
	return fmt.Errorf("no input file provided")
}

// validatePrerequisites checks if all prerequisites are met
func (t *Translator) validatePrerequisites() error {
	if !t.hasAPIKeys() {
		return errors.NewValidationError("please provide a valid Gemini API key", nil)
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
	if _, err := os.Stat(t.config.InputFile); os.IsNotExist(err) {
		return errors.NewFileError(fmt.Sprintf("input file %s does not exist", t.config.InputFile), err).WithContext("file_path", t.config.InputFile)
	}

	if t.config.ThinkingBudget < 0 || t.config.ThinkingBudget > 24576 {
		return errors.NewConfigurationError("thinking budget must be between 0 and 24576. 0 disables thinking", nil).WithContext("thinking_budget", t.config.ThinkingBudget)
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

// checkSavedProgress checks for saved progress and asks user to resume
func (t *Translator) checkSavedProgress() {
	if t.progressFile == "" || t.config.StartLine != 0 {
		return
	}

	data, err := os.ReadFile(t.progressFile)
	if err != nil {
		return
	}

	var progress ProgressInfo
	if err = json.Unmarshal(data, &progress); err != nil {
		logger.Warning(fmt.Sprintf("Error reading progress file: %v", err))
		return
	}

	// Verify the progress file matches our current input file
	if progress.InputFile != t.config.InputFile {
		logger.Warning(fmt.Sprintf("Found progress file for different subtitle: %s", progress.InputFile))
		logger.Warning("Ignoring saved progress.")
		return
	}

	if progress.Line > 1 {
		var resume string
		if t.config.Resume == nil {
			resume = strings.ToLower(strings.TrimSpace(logger.InputPrompt("Found saved progress. Resume? (y/n): ")))
		} else if *t.config.Resume {
			resume = "y"
		} else {
			resume = "n"
		}

		if resume == "y" || resume == "yes" {
			logger.Info(fmt.Sprintf("Resuming from line %d", progress.Line))
			t.config.StartLine = progress.Line
		} else {
			logger.Info("Starting from the beginning")
			// Remove the existing output file
			if err = os.Remove(t.outputFile); err != nil && !os.IsNotExist(err) {
				logger.Warning(fmt.Sprintf("Failed to remove output file: %v", err))
			}
		}
	}
}

// saveProgress saves current progress to file
func (t *Translator) saveProgress(line int, translatedSubtitles []srt.Subtitle) {
	if t.progressFile == "" {
		return
	}

	progress := ProgressInfo{
		Line:      line,
		InputFile: t.config.InputFile,
	}

	data, err := json.Marshal(progress)
	if err != nil {
		logger.Warning(fmt.Sprintf("Failed to marshal progress: %v", err))
		return
	}

	// Write translated subtitles to the file
	translatedContent := srt.ComposeSRT(translatedSubtitles)
	if err = os.WriteFile(t.outputFile, []byte(translatedContent), 0644); err != nil {
		logger.Warning(fmt.Sprintf("failed to write output file: %v", err))
	}

	if err = os.WriteFile(t.progressFile, data, 0644); err != nil {
		logger.Warning(fmt.Sprintf("Failed to save progress: %v", err))
	}
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
	client, err := helpers.CreateClient(ctx, t.config, t.getCurrentAPIKey())
	if err != nil {
		return err
	}
	defer func() {
		// Client does not need explicit closing in new API
	}()

	tokenLimit, err := helpers.GetTokenLimit(ctx, client, t.config.ModelName)
	if err != nil {
		return err
	}

	t.tokenLimit = tokenLimit
	return nil
}

// performTranslation performs the main translation process
func (t *Translator) performTranslation(ctx context.Context) error {
	// Read original subtitle file
	originalData, err := os.ReadFile(t.config.InputFile)
	if err != nil {
		return errors.NewFileError("failed to read input file", err).WithContext("file_path", t.config.InputFile)
	}

	originalSubtitles, err := srt.ParseSRT(string(originalData))
	if err != nil {
		return errors.NewFileError("failed to parse SRT file", err).WithContext("file_path", t.config.InputFile)
	}

	// Load or create translated subtitles
	var translatedSubtitles []srt.Subtitle
	if _, err = os.Stat(t.outputFile); err == nil {
		translatedData, errRead := os.ReadFile(t.outputFile)
		if errRead == nil {
			translatedSubtitles, errRead = srt.ParseSRT(string(translatedData))
			if errRead == nil {
				logger.Info(fmt.Sprintf("Translated file %s already exists. Loading existing translation...\n", t.outputFile))

				// Prompt for start line if not set
				if t.config.StartLine == 0 {
					for {
						input := logger.InputPrompt(fmt.Sprintf("Enter the line number to start from (1 to %d): ", len(originalSubtitles)))
						startLine, errParse := strconv.Atoi(strings.TrimSpace(input))
						if errParse != nil || startLine < 1 || startLine > len(originalSubtitles) {
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
		// Copy original subtitles as template
		translatedSubtitles = make([]srt.Subtitle, len(originalSubtitles))
		copy(translatedSubtitles, originalSubtitles)
		t.config.StartLine = 1
	}

	// Validate subtitle count consistency
	if len(originalSubtitles) != len(translatedSubtitles) {
		return errors.NewValidationError("number of lines of existing translated file does not match the number of lines in the original file", nil).WithContext("original_count", len(originalSubtitles)).WithContext("translated_count", len(translatedSubtitles))
	}

	// Validate start line
	if t.config.StartLine > len(originalSubtitles) || t.config.StartLine < 1 {
		return errors.NewValidationError(fmt.Sprintf("start line must be between 1 and %d", len(originalSubtitles)), nil).WithContext("start_line", t.config.StartLine).WithContext("max_lines", len(originalSubtitles))
	}

	// Adjust batch size if needed
	if len(originalSubtitles) < t.config.BatchSize {
		t.config.BatchSize = len(originalSubtitles)
	}

	// Setup delay for pro models with free quota
	delay := false
	delayTime := 30 * time.Second

	if strings.Contains(t.config.ModelName, "pro") && t.config.FreeQuota {
		delay = true
		if len(t.apiKeys) > 1 {
			delayTime = 15 * time.Second
			logger.Info("Pro model and free user quota detected, using multiple API keys if needed.\n")
		} else {
			logger.Info("Pro model and free user quota detected.\n")
		}
	}

	// Create client
	client, err := helpers.CreateClient(ctx, t.config, t.getCurrentAPIKey())
	if err != nil {
		return err
	}
	t.client = client

	// Start translation
	logger.Highlight(fmt.Sprintf("Starting translation of %d lines...\n", len(originalSubtitles)-t.config.StartLine+1))

	progressBar := logger.NewProgressBar(len(originalSubtitles), "Translating:")
	defer progressBar.Stop() // Ensure cleanup in case of early returns

	progressBar.SetSuffix(t.config.ModelName)
	progressBar.SetSending(true)

	i := t.config.StartLine - 1

	total := len(originalSubtitles)
	var batch []srt.SubtitleObject
	var previousMessage []*genai.Content

	// Build context from previous translations if resuming
	if t.config.StartLine > 1 {
		startIdx := max(0, t.config.StartLine-2-t.config.BatchSize)
		var userBatch []srt.SubtitleObject
		var modelBatch []srt.SubtitleObject

		for j := startIdx; j < t.config.StartLine-1; j++ {
			objUser := srt.SubtitleObject{
				Index:   strconv.Itoa(j),
				Content: originalSubtitles[j].Content,
			}

			objModel := srt.SubtitleObject{
				Index:   strconv.Itoa(j),
				Content: translatedSubtitles[j].Content,
			}

			userBatch = append(userBatch, objUser)
			modelBatch = append(modelBatch, objModel)
		}

		userData, _ := json.Marshal(userBatch)
		modelData, _ := json.Marshal(modelBatch)

		previousMessage = []*genai.Content{
			{
				Parts: []*genai.Part{{Text: string(userData)}},
				Role:  "user",
			},
			{
				Parts: []*genai.Part{{Text: string(modelData)}},
				Role:  "model",
			},
		}
	}

	progressBar.Update(i)

	if len(t.apiKeys) > 1 {
		progressBar.AddMessage(fmt.Sprintf("Starting with API Key %d", t.currentAPIIndex+1), logger.Cyan)
	}

	// Add first subtitle to batch
	obj := srt.SubtitleObject{
		Index:   strconv.Itoa(i),
		Content: originalSubtitles[i].Content,
	}
	batch = append(batch, obj)
	i++

	// Save initial progress
	t.saveProgress(i, translatedSubtitles)

	// Main translation loop
	for i < total || len(batch) > 0 {
		// Build batch
		for i < total && len(batch) < t.config.BatchSize {
			subtitleObj := srt.SubtitleObject{
				Index:   strconv.Itoa(i),
				Content: originalSubtitles[i].Content,
			}
			batch = append(batch, subtitleObj)
			i++
		}

		// Validate token size
		if err = t.validateTokenSize(ctx, batch); err != nil {
			return err
		}

		if i == total && len(batch) < t.config.BatchSize {
			t.config.BatchSize = len(batch)
		}

		// Process batch
		startTime := time.Now()
		newPreviousMessage, errProcessBatch := t.processBatch(ctx, batch, previousMessage, translatedSubtitles, progressBar)
		if errProcessBatch != nil {
			return errProcessBatch
		}
		endTime := time.Now()

		previousMessage = newPreviousMessage

		// Update progress
		progressBar.Update(i)
		t.saveProgress(i+1, translatedSubtitles)

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

	// Save final result
	logger.Success("Translation completed successfully!")
	if t.config.ProgressLog {
		if err = logger.SaveLogsToFile(t.logFilePath); err != nil {
			logger.Warning(fmt.Sprintf("Failed to save logs: %v", err))
		}
	}

	// Cleanup
	if t.progressFile != "" {
		if err = os.Remove(t.progressFile); err != nil && !os.IsNotExist(err) {
			logger.Warning(fmt.Sprintf("Failed to remove progress file: %v", err))
		}
	}

	return nil
}

// validateTokenSize validates that the batch doesn't exceed token limits
func (t *Translator) validateTokenSize(ctx context.Context, batch []srt.SubtitleObject) error {
	batchData, err := json.Marshal(batch)
	if err != nil {
		return errors.NewTranslationError("failed to marshal batch", err)
	}

	tokenCount, err := helpers.CountTokens(ctx, t.client, "gemini-2.5-flash", string(batchData))
	if err != nil {
		return errors.NewAPIError("failed to count tokens", err)
	}

	t.tokenCount = tokenCount

	// Check if token count exceeds 90% of limit
	if float64(tokenCount) > float64(t.tokenLimit)*0.9 {
		logger.Error(fmt.Sprintf("Token size (%d) exceeds limit (%d) for %s", int(float64(tokenCount)/0.9), t.tokenLimit, t.config.ModelName))

		// Ask user for new batch size
		for {
			input := logger.InputPrompt(fmt.Sprintf("Please enter a new batch size (current: %d): ", t.config.BatchSize))
			newBatchSize, errAtoi := strconv.Atoi(strings.TrimSpace(input))
			if errAtoi != nil || newBatchSize <= 0 {
				logger.Warning("Invalid input. Batch size must be a positive integer.")
				continue
			}

			t.config.BatchSize = newBatchSize
			logger.Info(fmt.Sprintf("Batch size updated to %d.", t.config.BatchSize))
			break
		}

		return errors.NewValidationError("batch size too large, please retry with smaller batch", nil).WithContext("current_batch_size", t.config.BatchSize).WithContext("token_count", tokenCount).WithContext("token_limit", t.tokenLimit)
	}

	return nil
}

// processBatch processes a single batch of subtitles with retry logic
func (t *Translator) processBatch(ctx context.Context, batch []srt.SubtitleObject, previousMessage []*genai.Content, translatedSubtitles []srt.Subtitle, progressBar *logger.ProgressBar) ([]*genai.Content, error) {
	var lastErr error

	for attempt := 0; attempt <= t.config.RetryCount; attempt++ {
		if attempt > 0 {
			logger.Warning(fmt.Sprintf("Retry attempt %d/%d", attempt, t.config.RetryCount))
			progressBar.AddRetry()

			// Switch to next API key if available
			if len(t.apiKeys) > 1 {
				t.currentAPIIndex = (t.currentAPIIndex + 1) % len(t.apiKeys)
				progressBar.AddMessage(fmt.Sprintf("Switching to API Key %d", t.currentAPIIndex+1), logger.Yellow)

				// Create new client with different API key
				client, errCreateClient := helpers.CreateClient(ctx, t.config, t.getCurrentAPIKey())
				if errCreateClient != nil {
					lastErr = errCreateClient
					continue
				}
				t.client = client
			}

			// Add small delay between retries
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		result, errProcess := t.processBatchAttempt(ctx, batch, previousMessage, translatedSubtitles, progressBar)
		if errProcess == nil {
			return result, nil
		}

		lastErr = errProcess
		logger.Error(fmt.Sprintf("Batch processing failed (attempt %d/%d): %v", attempt+1, t.config.RetryCount+1, errProcess))
	}

	return nil, fmt.Errorf("batch processing failed after %d retries: %w", t.config.RetryCount, lastErr)
}

// processBatchAttempt performs a single attempt to process a batch
func (t *Translator) processBatchAttempt(ctx context.Context, batch []srt.SubtitleObject, previousMessage []*genai.Content, translatedSubtitles []srt.Subtitle, progressBar *logger.ProgressBar) ([]*genai.Content, error) {
	// Create generation config
	thinkingCompatible := strings.Contains(t.config.ModelName, "2.5")
	instruction := helpers.GetInstruction(
		t.config.TargetLanguage,
		t.config.Thinking,
		thinkingCompatible,
		t.config.Description,
	)

	// Build content parts
	batchData, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch: %w", err)
	}

	var parts []*genai.Part
	parts = append(parts, &genai.Part{Text: string(batchData)})

	currentMessage := &genai.Content{
		Parts: parts,
		Role:  "user",
	}

	// Build full conversation history
	var contents []*genai.Content

	// Add system instruction as first message
	systemContent := &genai.Content{
		Parts: []*genai.Part{{Text: instruction}},
		Role:  "model",
	}
	contents = append(contents, systemContent)

	if len(previousMessage) > 0 {
		contents = append(contents, previousMessage...)
	}
	contents = append(contents, currentMessage)

	// Generate response
	progressBar.SetLoading(true)

	var responseText string

	// Use the new API for content generation
	genContentConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: instruction}},
			Role:  "model",
		},
	}

	// Set generation parameters
	genConfig := helpers.GetGenerationConfig(t.config.Temperature, t.config.TopP, t.config.TopK)
	if temp, ok := genConfig["temperature"].(float32); ok {
		genContentConfig.Temperature = &temp
	}
	if topP, ok := genConfig["top_p"].(float32); ok {
		genContentConfig.TopP = &topP
	}
	if topK, ok := genConfig["top_k"].(float32); ok {
		genContentConfig.TopK = &topK
	}
	if mimeType, ok := genConfig["response_mime_type"].(string); ok {
		genContentConfig.ResponseMIMEType = mimeType
	}
	if schema, hasSchema := genConfig["response_schema"].(map[string]interface{}); hasSchema {
		// Convert schema to genai.Schema
		jsonSchema := &genai.Schema{
			Type: genai.Type(schema["type"].(string)),
		}
		if items, hasItems := schema["items"].(map[string]interface{}); hasItems {
			jsonSchema.Items = &genai.Schema{
				Type: genai.Type(items["type"].(string)),
			}
			if props, hasProps := items["properties"].(map[string]interface{}); hasProps {
				jsonSchema.Items.Properties = make(map[string]*genai.Schema)
				for key, prop := range props {
					if propMap, isPropMap := prop.(map[string]interface{}); isPropMap {
						jsonSchema.Items.Properties[key] = &genai.Schema{
							Type: genai.Type(propMap["type"].(string)),
						}
					}
				}
			}
			if required, hasRequired := items["required"].([]string); hasRequired {
				jsonSchema.Items.Required = required
			}
		}
		genContentConfig.ResponseSchema = jsonSchema
	}

	var thinkingBudget int32
	if t.config.Thinking {
		thinkingBudget = int32(t.config.ThinkingBudget)
	} else {
		if strings.Contains(t.config.ModelName, "gemini-2.5-pro") {
			thinkingBudget = 128
		} else {
			thinkingBudget = 0
		}
	}
	genContentConfig.MaxOutputTokens = 65536
	genContentConfig.ThinkingConfig = &genai.ThinkingConfig{}
	genContentConfig.ThinkingConfig.ThinkingBudget = &thinkingBudget
	genContentConfig.ThinkingConfig.IncludeThoughts = true

	stream := t.client.Models.GenerateContentStream(ctx, t.config.ModelName, contents, genContentConfig)

	for chunk, errRange := range stream {
		if errRange != nil {
			return nil, fmt.Errorf("stream receive failed: %v", errRange)
		}

		if len(chunk.Candidates) == 0 {
			continue
		}

		for _, candidate := range chunk.Candidates {
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					if part.Thought {
						progressBar.SetThinking(true)
					} else {
						progressBar.SetThinking(false)
						if part.Text != "" {
							responseText += part.Text
						}
					}
				}
			}
		}
	}

	progressBar.SetLoading(false)

	// Parse final response
	var translatedBatch []srt.SubtitleObject
	if err = json.Unmarshal([]byte(responseText), &translatedBatch); err != nil {
		return nil, errors.NewTranslationError("failed to parse response", err).WithContext("response_text", responseText)
	}

	// Validate response content
	if errValidate := t.validateTranslatedResponse(translatedBatch, batch); errValidate != nil {
		return nil, errValidate
	}

	// Store successful translation
	t.translatedBatch = translatedBatch

	// Process translated lines
	if err = t.processTranslatedLines(t.translatedBatch, translatedSubtitles, batch); err != nil {
		return nil, err
	}

	t.batchNumber++

	// Build response message for context
	responseParts := []*genai.Part{{Text: responseText}}
	return []*genai.Content{
		{
			Parts: []*genai.Part{{Text: string(batchData)}},
			Role:  "user",
		},
		{
			Parts: responseParts,
			Role:  "model",
		},
	}, nil
}

// processTranslatedLines processes the translated subtitle lines
func (t *Translator) processTranslatedLines(translatedLines []srt.SubtitleObject, translatedSubtitles []srt.Subtitle, batch []srt.SubtitleObject) error {
	// Create index map from batch
	indexMap := make(map[string]int)
	for i, item := range batch {
		indexMap[item.Index] = i
	}

	// Process each translated line
	for _, line := range translatedLines {
		// Parse index
		index, err := strconv.Atoi(line.Index)
		if err != nil {
			return errors.NewValidationError(fmt.Sprintf("invalid index: %s", line.Index), err).WithContext("line_index", line.Index)
		}

		// Apply RTL detection and formatting
		if t.isDominantRTL(line.Content) {
			translatedSubtitles[index].Content = "\u202b" + line.Content + "\u202c"
		} else {
			translatedSubtitles[index].Content = line.Content
		}
	}

	return nil
}

// validateTranslatedResponse validates the translated response content
func (t *Translator) validateTranslatedResponse(translatedBatch []srt.SubtitleObject, originalBatch []srt.SubtitleObject) error {
	if len(translatedBatch) != len(originalBatch) {
		return errors.NewTranslationError(fmt.Sprintf("gemini returned unexpected response. Expected %d lines, got %d", len(originalBatch), len(translatedBatch)), nil).WithContext("expected_count", len(originalBatch)).WithContext("actual_count", len(translatedBatch))
	}

	// Check for empty translations
	for i, translated := range translatedBatch {
		if translated.Content == "" && originalBatch[i].Content != "" {
			return errors.NewTranslationError(fmt.Sprintf("gemini returned an empty translation for line %s", translated.Index), nil).WithContext("line_index", translated.Index)
		}
	}

	return nil
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
