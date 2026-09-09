package main

import (
	"context"
	stdErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/luispater/gemini-srt-translator-go/internal/logger"
	"github.com/luispater/gemini-srt-translator-go/internal/translator"
	"github.com/luispater/gemini-srt-translator-go/internal/video"
	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/languages"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = newRootCommand()

func newRootCommand() *cobra.Command {
	return newRootCommandWithConfig(config.NewConfig())
}

func newRootCommandWithConfig(commandConfig *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gst [flags] <SUBTITLE_FILE|MKV_FILE|GLOB>...",
		Short: "Translate one or more subtitle files (SRT, ASS) or extract and translate subtitles from MKV files using AI",
		Long: `Gemini SRT Translator is a powerful tool to translate subtitle files using AI providers (Gemini, OpenAI).
Supports both SRT and ASS subtitle files as well as MKV files with embedded subtitles.
Perfect for anyone needing fast, accurate, and customizable translations for videos, movies, and series.`,
		SilenceUsage:  true, // Don't show usage on errors
		SilenceErrors: true, // Don't show errors automatically (we handle them in main)
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runTranslate(commandConfig, args)
		},
	}

	// Root command flags (removed input-file flag)
	rootCmd.Flags().StringVarP(&commandConfig.TargetLanguage, "target-language", "l", "Simplified Chinese", "Target language for translation")
	rootCmd.Flags().StringVarP(&commandConfig.Provider, "provider", "p", commandConfig.Provider, "AI provider (gemini, openai)")
	rootCmd.Flags().StringVar(&commandConfig.OpenAIProtocol, "openai-protocol", commandConfig.OpenAIProtocol, "OpenAI API protocol (chat-completions, responses)")
	rootCmd.Flags().StringVarP(&commandConfig.BaseURL, "base-url", "", commandConfig.BaseURL, "API Base URL (auto-detected based on provider)")

	// Custom handling for comma-separated API keys
	var apiKeysStr string
	rootCmd.Flags().StringVarP(&apiKeysStr, "api-key", "k", "", "API key(s) - comma-separated for multiple keys (auto-detected based on provider)")
	rootCmd.Flags().StringVarP(&commandConfig.OutputFile, "output-file", "o", "", "Output file path")
	rootCmd.Flags().IntVarP(&commandConfig.StartLine, "start-line", "s", 0, "Starting line number")
	rootCmd.Flags().StringVarP(&commandConfig.Description, "description", "d", "", "Description for translation context")
	rootCmd.Flags().StringVarP(&commandConfig.ModelName, "model", "m", commandConfig.ModelName, "Model to use (gemini-2.5-pro, gpt-4o, etc.)")
	rootCmd.Flags().IntVarP(&commandConfig.BatchSize, "batch-size", "b", commandConfig.BatchSize, "Batch size for translation")
	rootCmd.Flags().IntVarP(&commandConfig.RetryCount, "retry-count", "r", commandConfig.RetryCount, "Number of retries for failed requests (default: 3)")

	// Model tuning parameters
	var temperature, topP, topK float32
	rootCmd.Flags().Float32Var(&temperature, "temperature", 1.0, "Temperature (0.0-2.0)")
	rootCmd.Flags().Float32Var(&topP, "top-p", 0.95, "Top P (0.0-1.0)")
	rootCmd.Flags().Float32Var(&topK, "top-k", 0, "Top K (>=0)")
	rootCmd.Flags().StringVar(&commandConfig.ThinkingLevel, "thinking-level", commandConfig.ThinkingLevel, "Thinking level (minimal, low, medium, high)")

	// Boolean flags
	var noStreaming, noThinking, noColors, progressLog, quiet bool
	var paidQuota, interactive, resume, noResume bool

	rootCmd.Flags().BoolVar(&noStreaming, "no-streaming", false, "Disable streaming")
	rootCmd.Flags().BoolVar(&noThinking, "no-thinking", false, "Disable thinking mode")
	rootCmd.Flags().BoolVar(&noColors, "no-colors", false, "Disable colored output")
	rootCmd.Flags().BoolVar(&progressLog, "progress-log", false, "Enable progress logging")
	rootCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress output")
	rootCmd.Flags().BoolVar(&resume, "resume", false, "Resume interrupted translation")
	rootCmd.Flags().BoolVar(&noResume, "no-resume", false, "Start from beginning")
	rootCmd.Flags().BoolVar(&paidQuota, "paid-quota", false, "Remove artificial limits for paid quota users")
	rootCmd.Flags().BoolVar(&interactive, "interactive", false, "Interactive model selection")

	// Set flag processing
	rootCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		// Auto-detect provider based on model name if not explicitly set.
		if !cmd.Flags().Changed("provider") && os.Getenv(config.EnvProvider) == "" {
			modelName := strings.ToLower(commandConfig.ModelName)
			if strings.Contains(modelName, "gpt") {
				commandConfig.Provider = "openai"
			} else if strings.Contains(modelName, "gemini") {
				commandConfig.Provider = "gemini"
			}
		}
		commandConfig.Provider = config.NormalizeProvider(commandConfig.Provider)

		// Reload legacy values for the selected provider unless a higher-priority source was provided.
		loadLegacyAPIKeys := !cmd.Flags().Changed("api-key") && os.Getenv(config.EnvAPIKey) == ""
		loadLegacyBaseURL := !cmd.Flags().Changed("base-url") && os.Getenv(config.EnvBaseURL) == ""
		if loadLegacyAPIKeys {
			commandConfig.APIKeys = nil
			commandConfig.LoadAPIKeysForProvider()
		}
		if loadLegacyBaseURL {
			commandConfig.BaseURL = ""
			commandConfig.LoadBaseURLForProvider()
		}

		// Set default model based on provider.
		if !cmd.Flags().Changed("model") && os.Getenv(config.EnvModel) == "" {
			commandConfig.ModelName = defaultModelForProvider(commandConfig.Provider, commandConfig.OpenAIProtocol)
		}

		if cmd.Flags().Changed("api-key") {
			// Override with command-line values, including an explicit empty value.
			keys := strings.Split(apiKeysStr, ",")
			commandConfig.APIKeys = []string{}
			for _, key := range keys {
				trimmed := strings.TrimSpace(key)
				if trimmed != "" {
					commandConfig.APIKeys = append(commandConfig.APIKeys, trimmed)
				}
			}
		}

		// Handle temperature
		if cmd.Flags().Changed("temperature") {
			commandConfig.Temperature = &temperature
		}
		if cmd.Flags().Changed("top-p") {
			commandConfig.TopP = &topP
		}
		if cmd.Flags().Changed("top-k") {
			commandConfig.TopK = &topK
		}

		// Handle boolean flags.
		if noStreaming {
			commandConfig.Streaming = false
		}
		if noThinking {
			commandConfig.Thinking = false
		}
		if noColors {
			commandConfig.UseColors = false
		}
		if progressLog {
			commandConfig.ProgressLog = true
		}
		if quiet {
			commandConfig.QuietMode = true
		}
		if paidQuota {
			commandConfig.FreeQuota = false
		}
		if resume {
			resumeValue := true
			commandConfig.Resume = &resumeValue
		}
		if noResume {
			resumeValue := false
			commandConfig.Resume = &resumeValue
		}

		// Handle interactive model selection.
		if interactive {
			return selectModelInteractive(commandConfig)
		}

		return nil
	}

	return rootCmd
}

func defaultModelForProvider(provider string, openAIProtocol string) string {
	if config.NormalizeProvider(provider) == "openai" {
		if strings.EqualFold(strings.TrimSpace(openAIProtocol), "responses") {
			return "gpt-5"
		}
		return "gpt-4o"
	}
	return "gemini-3.5-flash"
}

type translationTask struct {
	filename   string
	translator *translator.Translator
	progress   *logger.ProgressBar
	err        error
}

func runTranslate(commandConfig *config.Config, inputPatterns []string) error {
	commandConfig.Provider = config.NormalizeProvider(commandConfig.Provider)
	logger.SetColorMode(commandConfig.UseColors)
	logger.SetQuietMode(commandConfig.QuietMode)

	inputFiles, errExpandInputs := expandInputFiles(inputPatterns)
	if errExpandInputs != nil {
		return errExpandInputs
	}
	if len(inputFiles) > 1 && commandConfig.OutputFile != "" {
		return errors.NewConfigurationError("--output-file can only be used with one input file", nil)
	}
	for _, inputFile := range inputFiles {
		if !validateVideoFilePath(inputFile) {
			return errors.NewFileError("invalid input file", nil).WithContext("file_path", inputFile)
		}
	}

	if len(commandConfig.APIKeys) == 0 {
		var prompt string
		switch commandConfig.Provider {
		case "openai":
			prompt = "Enter your OpenAI API key: "
		case "gemini":
			fallthrough
		default:
			prompt = "Enter your Gemini API key: "
		}
		apiKey := getAPIKeyFromInput(prompt)
		commandConfig.APIKeys = []string{apiKey}
	}
	if commandConfig.TargetLanguage == "" {
		commandConfig.TargetLanguage = strings.TrimSpace(logger.InputPrompt("Enter target language: "))
	}

	tasks, errPrepareTasks := prepareTranslationTasks(commandConfig, inputFiles)
	if errPrepareTasks != nil {
		return errPrepareTasks
	}
	multiProgress := logger.NewMultiProgress()
	for taskIndex := range tasks {
		task := &tasks[taskIndex]
		task.progress = multiProgress.AddBar(filepath.Base(task.filename))
		if task.translator != nil {
			task.translator.SetProgressBar(task.progress)
		}
		if task.err != nil {
			task.progress.Fail(task.err)
			saveTranslationTaskLog(task)
			continue
		}
		task.progress.SetStatus("Ready")
	}

	multiProgress.Start()
	var waitGroup sync.WaitGroup
	for taskIndex := range tasks {
		task := &tasks[taskIndex]
		if task.err != nil {
			continue
		}
		waitGroup.Add(1)
		go func(currentTask *translationTask) {
			defer waitGroup.Done()
			taskContext, cancelTask := context.WithCancel(context.Background())
			defer cancelTask()

			currentTask.progress.SetStatus("Starting")
			if errTranslate := currentTask.translator.Translate(taskContext); errTranslate != nil {
				currentTask.err = errTranslate
				currentTask.progress.Fail(errTranslate)
				saveTranslationTaskLog(currentTask)
				return
			}
			currentTask.progress.Complete()
			saveTranslationTaskLog(currentTask)
		}(task)
	}
	waitGroup.Wait()
	multiProgress.Stop()

	var failureMessages []string
	for _, task := range tasks {
		if task.err != nil {
			failureMessages = append(failureMessages, fmt.Sprintf("%s: %v", task.filename, task.err))
		}
	}
	if len(failureMessages) > 0 {
		return fmt.Errorf("%d translation task(s) failed:\n%s", len(failureMessages), strings.Join(failureMessages, "\n"))
	}
	return nil
}

func saveTranslationTaskLog(task *translationTask) {
	if task.translator == nil {
		return
	}
	if errSaveLog := task.translator.SaveProgressLog(); errSaveLog != nil {
		task.progress.AddMessage(fmt.Sprintf("Failed to save progress log: %v", errSaveLog), logger.Yellow)
		if task.err != nil {
			task.progress.Fail(task.err)
		} else {
			task.progress.Complete()
		}
	}
}

func prepareTranslationTasks(commandConfig *config.Config, inputFiles []string) ([]translationTask, error) {
	tasks := make([]translationTask, 0, len(inputFiles))
	for _, inputFile := range inputFiles {
		taskConfig := commandConfig.Clone()
		taskConfig.InputFile = inputFile

		task := translationTask{filename: inputFile}
		if strings.EqualFold(filepath.Ext(inputFile), ".mkv") {
			subtitleTrack, errSelectTrack := selectSubtitleTrack(inputFile)
			if errSelectTrack != nil {
				task.err = errSelectTrack
				task.translator = translator.NewTranslator(taskConfig)
				tasks = append(tasks, task)
				continue
			}
			taskConfig.SubtitleTrack = subtitleTrack
		}
		task.translator = translator.NewTranslator(taskConfig)
		tasks = append(tasks, task)
	}

	if errValidatePaths := validateTranslationTaskPaths(tasks); errValidatePaths != nil {
		return nil, errValidatePaths
	}
	for taskIndex := range tasks {
		task := &tasks[taskIndex]
		if task.err != nil {
			continue
		}
		if errPrepare := task.translator.Prepare(); errPrepare != nil {
			task.err = errPrepare
		}
	}
	return tasks, nil
}

func validateTranslationTaskPaths(tasks []translationTask) error {
	inputOwners := make(map[string]string)
	for _, task := range tasks {
		canonicalInput, errCanonicalInput := canonicalPath(task.filename)
		if errCanonicalInput != nil {
			return errors.NewFileError("failed to resolve input path", errCanonicalInput).WithContext("file_path", task.filename)
		}
		inputOwners[canonicalInput] = task.filename
	}

	artifactOwners := make(map[string]string)
	for _, task := range tasks {
		if task.translator == nil {
			continue
		}
		for _, artifactPath := range task.translator.ArtifactPaths() {
			canonicalArtifact, errCanonicalArtifact := canonicalPath(artifactPath)
			if errCanonicalArtifact != nil {
				return errors.NewFileError("failed to resolve output path", errCanonicalArtifact).WithContext("file_path", artifactPath)
			}
			if inputOwner, conflictsWithInput := inputOwners[canonicalArtifact]; conflictsWithInput {
				return errors.NewConfigurationError("translation artifact conflicts with an input file", nil).WithContext("artifact_path", artifactPath).WithContext("input_file", inputOwner)
			}
			if artifactOwner, conflictsWithTask := artifactOwners[canonicalArtifact]; conflictsWithTask {
				return errors.NewConfigurationError("translation tasks would write the same artifact", nil).WithContext("artifact_path", artifactPath).WithContext("first_input", artifactOwner).WithContext("second_input", task.filename)
			}
			artifactOwners[canonicalArtifact] = task.filename
		}
	}
	return nil
}

func canonicalPath(filePath string) (string, error) {
	absolutePath, errAbsolutePath := filepath.Abs(filePath)
	if errAbsolutePath != nil {
		return "", errAbsolutePath
	}
	absolutePath = filepath.Clean(absolutePath)

	currentPath := absolutePath
	var unresolvedParts []string
	for {
		resolvedPath, errResolvePath := filepath.EvalSymlinks(currentPath)
		if errResolvePath == nil {
			for unresolvedIndex := len(unresolvedParts) - 1; unresolvedIndex >= 0; unresolvedIndex-- {
				resolvedPath = filepath.Join(resolvedPath, unresolvedParts[unresolvedIndex])
			}
			return filepath.Clean(resolvedPath), nil
		}
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return absolutePath, nil
		}
		unresolvedParts = append(unresolvedParts, filepath.Base(currentPath))
		currentPath = parentPath
	}
}

func expandInputFiles(inputPatterns []string) ([]string, error) {
	seenFiles := make(map[string]struct{})
	var inputFiles []string
	for _, inputPattern := range inputPatterns {
		var matches []string
		if _, errStatLiteral := os.Stat(inputPattern); errStatLiteral == nil {
			matches = []string{inputPattern}
		} else {
			globMatches, errGlob := filepath.Glob(inputPattern)
			if errGlob != nil {
				return nil, errors.NewFileError("invalid input glob", errGlob).WithContext("pattern", inputPattern)
			}
			matches = globMatches
		}
		if len(matches) == 0 {
			if strings.ContainsAny(inputPattern, "*?[") {
				return nil, errors.NewFileError("input glob matched no files", nil).WithContext("pattern", inputPattern)
			}
			matches = []string{inputPattern}
		}
		for _, match := range matches {
			cleanedPath := filepath.Clean(match)
			canonicalInput, errCanonicalInput := canonicalPath(cleanedPath)
			if errCanonicalInput != nil {
				return nil, errors.NewFileError("failed to resolve input path", errCanonicalInput).WithContext("file_path", cleanedPath)
			}
			if _, exists := seenFiles[canonicalInput]; exists {
				continue
			}
			seenFiles[canonicalInput] = struct{}{}
			inputFiles = append(inputFiles, cleanedPath)
		}
	}
	if len(inputFiles) == 0 {
		return nil, errors.NewValidationError("please provide at least one input file", nil)
	}
	return inputFiles, nil
}

func selectSubtitleTrack(mkvPath string) (int, error) {
	tracks, errInspectTracks := video.InspectSubtitleTracks(mkvPath)
	if errInspectTracks != nil {
		return 0, errInspectTracks
	}

	logger.Highlight(fmt.Sprintf("\nSource subtitles for %s", mkvPath))
	if len(tracks) == 1 {
		logger.Info(fmt.Sprintf("Only one subtitle track found; selecting %s automatically.", subtitleTrackDescription(tracks[0])))
		return tracks[0].Number, nil
	}

	logger.Info("Available subtitle tracks:")
	for trackIndex, track := range tracks {
		logger.Info(fmt.Sprintf("[%d] %s", trackIndex+1, subtitleTrackDescription(track)))
	}

	defaultTrack := video.SelectBestSubtitleTrack(tracks)
	defaultIndex := 0
	if defaultTrack != nil {
		for trackIndex, track := range tracks {
			if track.Number == defaultTrack.Number {
				defaultIndex = trackIndex
				break
			}
		}
	}
	for {
		input := strings.TrimSpace(logger.InputPrompt(fmt.Sprintf("Select source subtitle track for %s [%d]: ", filepath.Base(mkvPath), defaultIndex+1)))
		if input == "" {
			return tracks[defaultIndex].Number, nil
		}
		selection, errSelection := strconv.Atoi(input)
		if errSelection == nil && selection >= 1 && selection <= len(tracks) {
			return tracks[selection-1].Number, nil
		}
		logger.Warning("Invalid selection. Enter a valid number.")
	}
}

func subtitleTrackDescription(track video.SubtitleTrack) string {
	language := strings.TrimSpace(track.Language)
	if bcp47 := languages.BCP47FromMKV(language); bcp47 != "" {
		language = bcp47
	}
	if language == "" {
		language = "undetermined"
	}
	if name := strings.TrimSpace(track.Name); name != "" {
		return fmt.Sprintf("Language: %s, Name: %s, Codec: %s", language, name, track.Codec)
	}
	return fmt.Sprintf("Language: %s, Codec: %s", language, track.Codec)
}

func selectModelInteractive(commandConfig *config.Config) error {
	t := translator.NewTranslator(commandConfig)
	ctx := context.Background()

	models, err := t.GetModels(ctx)
	if err != nil {
		return err
	}

	if len(models) == 0 {
		logger.Info("No models available.")
		return nil
	}

	fmt.Println("\nAvailable models:")
	for i, model := range models {
		fmt.Printf("%d. %s\n", i+1, model)
	}

	for {
		input, errPromptModel := logger.InputPromptWithError("\nEnter model number: ")
		if errPromptModel != nil {
			return errors.NewValidationError("cannot select a model without interactive input", errPromptModel)
		}
		choice, errAtoi := strconv.Atoi(strings.TrimSpace(input))
		if errAtoi != nil || choice < 1 || choice > len(models) {
			logger.Error("Invalid choice. Please try again.")
			continue
		}

		commandConfig.ModelName = models[choice-1]
		logger.Success(fmt.Sprintf("Selected model: %s", commandConfig.ModelName))
		break
	}

	return nil
}

func getAPIKeyFromInput(prompt string) string {
	fmt.Print(prompt)
	//goland:noinspection GoRedundantConversion
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		logger.Error(fmt.Sprintf("Error reading API key: %v", err))
		return ""
	}
	fmt.Println() // Add newline after password input
	return strings.TrimSpace(string(bytePassword))
}

func validateVideoFilePath(filePath string) bool {
	fileInfo, errStat := os.Stat(filePath)
	if errStat != nil {
		logger.Error(fmt.Sprintf("Cannot access file %s: %v", filePath, errStat))
		return false
	}
	if !fileInfo.Mode().IsRegular() {
		logger.Error(fmt.Sprintf("Input path is not a regular file: %s", filePath))
		return false
	}

	extension := strings.ToLower(filepath.Ext(filePath))
	supportedExts := []string{".srt", ".ass", ".ssa", ".mkv"}

	for _, ext := range supportedExts {
		if extension == ext {
			return true
		}
	}

	logger.Error(fmt.Sprintf("File must have .srt, .ass, .ssa, or .mkv extension: %s", filePath))
	return false
}

func main() {
	if errExecute := rootCmd.Execute(); errExecute != nil {
		// Handle structured errors with additional context
		var translatorErr *errors.TranslatorError
		if stdErrors.As(errExecute, &translatorErr) {
			logger.Error(fmt.Sprintf("[%s] %s", strings.ToUpper(string(translatorErr.Type)), translatorErr.Message))
			if translatorErr.Cause != nil {
				logger.Error(fmt.Sprintf("Cause: %v", translatorErr.Cause))
			}
			if len(translatorErr.Context) > 0 {
				logger.Error("Context:")
				for key, value := range translatorErr.Context {
					logger.Error(fmt.Sprintf("  %s: %v", key, value))
				}
			}
		} else {
			// Handle non-structured errors
			logger.Error(fmt.Sprintf("Error: %v", errExecute))
		}
		os.Exit(1)
	}
}
