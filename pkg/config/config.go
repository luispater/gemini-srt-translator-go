package config

import (
	"os"
	"strings"
)

// Config holds all configuration for the translator
type Config struct {
	// API configuration
	GoogleGeminiBaseURL string
	GeminiAPIKeys       []string
	TargetLanguage      string

	// File paths
	InputFile  string
	OutputFile string

	// Processing options
	StartLine   int
	Description string
	BatchSize   int
	RetryCount  int

	// Model configuration
	ModelName      string
	Streaming      bool
	Thinking       bool
	ThinkingBudget int
	Temperature    *float32
	TopP           *float32
	TopK           *float32

	// User options
	FreeQuota   bool
	UseColors   bool
	ProgressLog bool
	QuietMode   bool
	Resume      *bool
}

// parseAPIKeys parses comma-separated API keys from environment variable
func parseAPIKeys(envKey string) []string {
	value := os.Getenv(envKey)
	if value == "" {
		return []string{}
	}

	keys := strings.Split(value, ",")
	var result []string
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		GeminiAPIKeys:  parseAPIKeys("GEMINI_API_KEY"),
		ModelName:      "gemini-2.5-pro",
		BatchSize:      300,
		RetryCount:     3,
		Streaming:      true,
		Thinking:       true,
		ThinkingBudget: 2048,
		FreeQuota:      true,
		UseColors:      true,
		ProgressLog:    false,
		QuietMode:      false,
	}
}
