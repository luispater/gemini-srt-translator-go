package providers

import (
	"context"

	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

// TranslationProvider defines the interface for AI translation providers
type TranslationProvider interface {
	// GetModels returns available models for the provider
	GetModels(ctx context.Context) ([]string, error)

	// GetTokenLimit returns the token limit for a specific model
	GetTokenLimit(ctx context.Context, modelName string) (int32, error)

	// CountTokens counts tokens in the given content for a model
	CountTokens(ctx context.Context, modelName string, content string) (int32, error)

	// TranslateBatch translates a batch of subtitle objects
	TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []ContextMessage, config *TranslationConfig) (*TranslationResponse, error)

	// GetName returns the provider name
	GetName() string
}

// ContextMessage represents a conversation message for context
type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TranslationConfig holds configuration for translation request
type TranslationConfig struct {
	ModelName       string
	TargetLanguage  string
	Description     string
	Temperature     *float32
	TopP            *float32
	TopK            *float32
	Streaming       bool
	Thinking        bool
	ThinkingBudget  int
	ProgressUpdater ProgressUpdater
}

// TranslationResponse holds the response from translation
type TranslationResponse struct {
	TranslatedBatch []srt.SubtitleObject
	Context         []ContextMessage
}

// KeySwitcher interface for providers that support multiple API keys
type KeySwitcher interface {
	SwitchAPIKey() bool
	GetCurrentAPIKeyIndex() int
}

// ProgressUpdater interface for updating translation progress
type ProgressUpdater interface {
	SetLoading(loading bool)
	SetThinking(thinking bool)
}

// ProviderFactory creates providers based on configuration
type ProviderFactory struct{}

// NewProvider creates a new provider instance based on the configuration
func (f *ProviderFactory) NewProvider(cfg *config.Config) (TranslationProvider, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg)
	case "gemini":
		fallthrough
	default:
		return NewGeminiProvider(cfg)
	}
}
