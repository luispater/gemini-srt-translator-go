package config

import (
	"os"
	"strings"
)

// Config holds all configuration for the translator
type Config struct {
	// Provider selection
	Provider string

	// API configuration (unified for all providers)
	BaseURL        string
	APIKeys        []string
	TargetLanguage string

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
		Provider:       "gemini",                            // Default to Gemini for backward compatibility
		APIKeys:        parseAPIKeys("GEMINI_API_KEY"),      // Default to Gemini env var
		BaseURL:        os.Getenv("GOOGLE_GEMINI_BASE_URL"), // Default to Gemini base URL
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

// LoadEnvironmentForProvider loads environment variables based on the provider
func (c *Config) LoadEnvironmentForProvider() {
	switch c.Provider {
	case "openai":
		// Load OpenAI environment variables
		if len(c.APIKeys) == 0 {
			c.APIKeys = parseAPIKeys("OPENAI_API_KEY")
		}
		if c.BaseURL == "" {
			c.BaseURL = os.Getenv("OPENAI_BASE_URL")
		}
	case "gemini":
		fallthrough
	default:
		// Load Gemini environment variables
		if len(c.APIKeys) == 0 {
			c.APIKeys = parseAPIKeys("GEMINI_API_KEY")
		}
		if c.BaseURL == "" {
			c.BaseURL = os.Getenv("GOOGLE_GEMINI_BASE_URL")
		}
	}
}
