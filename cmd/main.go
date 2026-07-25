package main

import (
	"context"
	stdErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/luispater/gemini-srt-translator-go/internal/logger"
	"github.com/luispater/gemini-srt-translator-go/internal/translator"
	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = newRootCommand()

func newRootCommand() *cobra.Command {
	return newRootCommandWithConfig(config.NewConfig())
}

func newRootCommandWithConfig(commandConfig *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gst [flags] <SRT_FILE|MKV_FILE>",
		Short: "Translate SRT subtitle files or extract and translate subtitles from MKV files using AI",
		Long: `Gemini SRT Translator is a powerful tool to translate subtitle files using AI providers (Gemini, OpenAI).
Supports both SRT files and MKV files with embedded subtitles.
Perfect for anyone needing fast, accurate, and customizable translations for videos, movies, and series.`,
		SilenceUsage:  true, // Don't show usage on errors
		SilenceErrors: true, // Don't show errors automatically (we handle them in main)
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if no arguments provided, show help
			if len(args) == 0 {
				return cmd.Help()
			}
			// Set input file from positional argument
			commandConfig.InputFile = args[0]
			return runTranslate(commandConfig)
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

func runTranslate(commandConfig *config.Config) error {
	commandConfig.Provider = config.NormalizeProvider(commandConfig.Provider)

	// Set logger modes.
	logger.SetColorMode(commandConfig.UseColors)
	logger.SetQuietMode(commandConfig.QuietMode)

	// Validate required fields based on provider.
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

	// Validate file paths.
	if commandConfig.InputFile != "" {
		if !validateVideoFilePath(commandConfig.InputFile) {
			return errors.NewFileError("invalid input file", nil).WithContext("file_path", commandConfig.InputFile)
		}
	}

	// Create translator and perform translation.
	t := translator.NewTranslator(commandConfig)

	ctx := context.Background()
	return t.Translate(ctx)
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
		input := logger.InputPrompt("\nEnter model number: ")
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
	if _, errStat := os.Stat(filePath); os.IsNotExist(errStat) {
		logger.Error(fmt.Sprintf("File does not exist: %s", filePath))
		return false
	}

	extension := strings.ToLower(filepath.Ext(filePath))
	supportedExts := []string{".srt", ".mkv"}

	for _, ext := range supportedExts {
		if extension == ext {
			return true
		}
	}

	logger.Error(fmt.Sprintf("File must have .srt or .mkv extension: %s", filePath))
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
