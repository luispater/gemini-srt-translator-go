package config

import (
	"os"
	"strings"
)

const (
	// EnvProvider configures the AI provider.
	EnvProvider = "SRT_TRANSLATOR_PROVIDER"
	// EnvOpenAIProtocol configures the OpenAI API protocol.
	EnvOpenAIProtocol = "SRT_TRANSLATOR_OPENAI_PROTOCOL"
	// EnvBaseURL configures the API base URL.
	EnvBaseURL = "SRT_TRANSLATOR_BASE_URL"
	// EnvAPIKey configures one or more comma-separated API keys.
	EnvAPIKey = "SRT_TRANSLATOR_API_KEY"
	// EnvModel configures the model name.
	EnvModel = "SRT_TRANSLATOR_MODEL"
)

// Config holds all configuration for the translator
type Config struct {
	// Provider selection
	Provider       string
	OpenAIProtocol string

	// API configuration (unified for all providers)
	BaseURL        string
	APIKeys        []string
	TargetLanguage string

	// File paths
	InputFile     string
	OutputFile    string
	SubtitleTrack int

	// Processing options
	StartLine   int
	Description string
	BatchSize   int
	RetryCount  int

	// Model configuration
	ModelName            string
	OpenAIPromptCacheKey string
	Streaming            bool
	Thinking             bool
	ThinkingLevel        string
	Temperature          *float32
	TopP                 *float32
	TopK                 *float32

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

func environmentOrDefault(envKey string, defaultValue string) string {
	if valueGetenv := os.Getenv(envKey); valueGetenv != "" {
		return valueGetenv
	}
	return defaultValue
}

// NormalizeProvider returns the canonical provider name.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func legacyAPIKeyEnvironment(provider string) string {
	if NormalizeProvider(provider) == "openai" {
		return "OPENAI_API_KEY"
	}
	return "GEMINI_API_KEY"
}

func legacyBaseURLEnvironment(provider string) string {
	if NormalizeProvider(provider) == "openai" {
		return "OPENAI_BASE_URL"
	}
	return "GOOGLE_GEMINI_BASE_URL"
}

func apiKeysFromEnvironment(provider string) []string {
	if os.Getenv(EnvAPIKey) != "" {
		return parseAPIKeys(EnvAPIKey)
	}
	return parseAPIKeys(legacyAPIKeyEnvironment(provider))
}

func baseURLFromEnvironment(provider string) string {
	return environmentOrDefault(EnvBaseURL, os.Getenv(legacyBaseURLEnvironment(provider)))
}

func defaultModelForProvider(provider string, openAIProtocol string) string {
	if NormalizeProvider(provider) == "openai" {
		if strings.EqualFold(strings.TrimSpace(openAIProtocol), "responses") {
			return "gpt-5"
		}
		return "gpt-4o"
	}
	return "gemini-3.5-flash"
}

// NewConfig creates a new configuration with default values.
func NewConfig() *Config {
	provider := NormalizeProvider(environmentOrDefault(EnvProvider, "gemini"))
	openAIProtocol := environmentOrDefault(EnvOpenAIProtocol, "chat-completions")
	return &Config{
		Provider:       provider,
		OpenAIProtocol: openAIProtocol,
		APIKeys:        apiKeysFromEnvironment(provider),
		BaseURL:        baseURLFromEnvironment(provider),
		ModelName:      environmentOrDefault(EnvModel, defaultModelForProvider(provider, openAIProtocol)),
		BatchSize:      300,
		RetryCount:     3,
		Streaming:      true,
		Thinking:       true,
		ThinkingLevel:  "high",
		FreeQuota:      true,
		UseColors:      true,
		ProgressLog:    false,
		QuietMode:      false,
	}
}

// Clone returns an independent copy suitable for one translation task.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}

	cloned := *c
	cloned.APIKeys = append([]string(nil), c.APIKeys...)
	if c.Temperature != nil {
		temperature := *c.Temperature
		cloned.Temperature = &temperature
	}
	if c.TopP != nil {
		topP := *c.TopP
		cloned.TopP = &topP
	}
	if c.TopK != nil {
		topK := *c.TopK
		cloned.TopK = &topK
	}
	if c.Resume != nil {
		resume := *c.Resume
		cloned.Resume = &resume
	}
	return &cloned
}

// LoadEnvironmentForProvider loads legacy environment variables for the selected provider.
func (c *Config) LoadEnvironmentForProvider() {
	c.LoadAPIKeysForProvider()
	c.LoadBaseURLForProvider()
}

// LoadAPIKeysForProvider loads legacy API keys for the selected provider when none are configured.
func (c *Config) LoadAPIKeysForProvider() {
	if len(c.APIKeys) == 0 && os.Getenv(EnvAPIKey) == "" {
		c.APIKeys = parseAPIKeys(legacyAPIKeyEnvironment(c.Provider))
	}
}

// LoadBaseURLForProvider loads the legacy base URL for the selected provider when none is configured.
func (c *Config) LoadBaseURLForProvider() {
	if c.BaseURL == "" {
		c.BaseURL = os.Getenv(legacyBaseURLEnvironment(c.Provider))
	}
}
