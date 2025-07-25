package main

import (
	"context"
	stdErrors "errors"
	"fmt"
	"os"
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

var cfg *config.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gst [flags] <SRT_FILE>",
	Short: "Translate SRT subtitle files using Google Gemini AI",
	Long: `Gemini SRT Translator is a powerful tool to translate subtitle files using Google Gemini AI.
Perfect for anyone needing fast, accurate, and customizable translations for videos, movies, and series.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if no arguments provided, show help
		if len(args) == 0 {
			return cmd.Help()
		}
		// Set input file from positional argument
		cfg.InputFile = args[0]
		return runTranslate(cmd, args)
	},
}

func init() {
	cfg = config.NewConfig()

	// Root command flags (removed input-file flag)
	rootCmd.Flags().StringVarP(&cfg.TargetLanguage, "target-language", "l", "Simplified Chinese", "Target language for translation")
	rootCmd.Flags().StringVarP(&cfg.GoogleGeminiBaseURL, "base-url", "", os.Getenv("GOOGLE_GEMINI_BASE_URL"), "Gemini Base URL")

	// Custom handling for comma-separated API keys
	var apiKeysStr string
	rootCmd.Flags().StringVarP(&apiKeysStr, "api-key", "k", os.Getenv("GEMINI_API_KEY"), "Gemini API key(s) - comma-separated for multiple keys")
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if apiKeysStr != "" {
			keys := strings.Split(apiKeysStr, ",")
			cfg.GeminiAPIKeys = []string{}
			for _, key := range keys {
				trimmed := strings.TrimSpace(key)
				if trimmed != "" {
					cfg.GeminiAPIKeys = append(cfg.GeminiAPIKeys, trimmed)
				}
			}
		}
	}
	rootCmd.Flags().StringVarP(&cfg.OutputFile, "output-file", "o", "", "Output file path")
	rootCmd.Flags().IntVarP(&cfg.StartLine, "start-line", "s", 0, "Starting line number")
	rootCmd.Flags().StringVarP(&cfg.Description, "description", "d", "", "Description for translation context")
	rootCmd.Flags().StringVarP(&cfg.ModelName, "model", "m", cfg.ModelName, "Gemini model to use")
	rootCmd.Flags().IntVarP(&cfg.BatchSize, "batch-size", "b", cfg.BatchSize, "Batch size for translation")
	rootCmd.Flags().IntVarP(&cfg.RetryCount, "retry-count", "r", cfg.RetryCount, "Number of retries for failed requests (default: 3)")

	// Model tuning parameters
	var temperature, topP, topK float32
	rootCmd.Flags().Float32Var(&temperature, "temperature", 1.0, "Temperature (0.0-2.0)")
	rootCmd.Flags().Float32Var(&topP, "top-p", 0.95, "Top P (0.0-1.0)")
	rootCmd.Flags().Float32Var(&topK, "top-k", 0, "Top K (>=0)")
	rootCmd.Flags().IntVar(&cfg.ThinkingBudget, "thinking-budget", cfg.ThinkingBudget, "Thinking budget (0-24576)")

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
		// Handle temperature
		if cmd.Flags().Changed("temperature") {
			cfg.Temperature = &temperature
		}
		if cmd.Flags().Changed("top-p") {
			cfg.TopP = &topP
		}
		if cmd.Flags().Changed("top-k") {
			cfg.TopK = &topK
		}

		// Handle boolean flags
		if noStreaming {
			cfg.Streaming = false
		}
		if noThinking {
			cfg.Thinking = false
		}
		if noColors {
			cfg.UseColors = false
		}
		if progressLog {
			cfg.ProgressLog = true
		}
		if quiet {
			cfg.QuietMode = true
		}
		if paidQuota {
			cfg.FreeQuota = false
		}
		if resume {
			resumeValue := true
			cfg.Resume = &resumeValue
		}
		if noResume {
			resumeValue := false
			cfg.Resume = &resumeValue
		}

		// Handle interactive model selection
		if interactive {
			return selectModelInteractive()
		}

		return nil
	}

}

func runTranslate(_ *cobra.Command, _ []string) error {
	// Set logger modes
	logger.SetColorMode(cfg.UseColors)
	logger.SetQuietMode(cfg.QuietMode)

	// Validate required fields
	if len(cfg.GeminiAPIKeys) == 0 {
		apiKey := getAPIKeyFromInput("Enter your Gemini API key: ")
		cfg.GeminiAPIKeys = []string{apiKey}
	}

	if cfg.TargetLanguage == "" {
		cfg.TargetLanguage = strings.TrimSpace(logger.InputPrompt("Enter target language: "))
	}

	// Validate file paths
	if cfg.InputFile != "" {
		if !validateFilePath(cfg.InputFile, ".srt") {
			return errors.NewFileError("invalid input file", nil).WithContext("file_path", cfg.InputFile)
		}
	}

	// Create translator and perform translation
	t := translator.NewTranslator(cfg)

	ctx := context.Background()
	return t.Translate(ctx)
}

func selectModelInteractive() error {
	t := translator.NewTranslator(cfg)
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

		cfg.ModelName = models[choice-1]
		logger.Success(fmt.Sprintf("Selected model: %s", cfg.ModelName))
		break
	}

	return nil
}

func getAPIKeyFromInput(prompt string) string {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		logger.Error(fmt.Sprintf("Error reading API key: %v", err))
		return ""
	}
	fmt.Println() // Add newline after password input
	return strings.TrimSpace(string(bytePassword))
}

func validateFilePath(filePath, extension string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Error(fmt.Sprintf("File does not exist: %s", filePath))
		return false
	}

	if extension != "" && !strings.HasSuffix(strings.ToLower(filePath), extension) {
		logger.Error(fmt.Sprintf("File must have %s extension: %s", extension, filePath))
		return false
	}

	return true
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Handle structured errors with additional context
		var translatorErr *errors.TranslatorError
		if stdErrors.As(err, &translatorErr) {
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
		}
		os.Exit(1)
	}
}
