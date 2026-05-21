package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/luispater/gemini-srt-translator-go/internal/helpers"
	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

// GeminiProvider implements TranslationProvider for Google Gemini
type GeminiProvider struct {
	config          *config.Config
	client          *genai.Client
	apiKeys         []string
	currentAPIIndex int
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(cfg *config.Config) (*GeminiProvider, error) {
	return &GeminiProvider{
		config:          cfg,
		apiKeys:         cfg.APIKeys,
		currentAPIIndex: 0,
	}, nil
}

// GetName returns the provider name
func (g *GeminiProvider) GetName() string {
	return "gemini"
}

// parseThinkingLevel converts configuration values to Gemini SDK thinking levels.
func parseThinkingLevel(level string) genai.ThinkingLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "minimal":
		return genai.ThinkingLevelMinimal
	case "low":
		return genai.ThinkingLevelLow
	case "medium":
		return genai.ThinkingLevelMedium
	case "high":
		return genai.ThinkingLevelHigh
	default:
		return genai.ThinkingLevelHigh
	}
}

// getCurrentAPIKey returns the current API key if available
func (g *GeminiProvider) getCurrentAPIKey() string {
	if len(g.apiKeys) == 0 || g.currentAPIIndex >= len(g.apiKeys) {
		return ""
	}
	return g.apiKeys[g.currentAPIIndex]
}

// GetModels returns available Gemini models
func (g *GeminiProvider) GetModels(ctx context.Context) ([]string, error) {
	if len(g.apiKeys) == 0 {
		return nil, errors.NewValidationError("please provide a valid Gemini API key", nil)
	}

	client, err := helpers.CreateClient(ctx, g.config, g.getCurrentAPIKey())
	if err != nil {
		return nil, errors.NewAPIError("failed to create Gemini client", err)
	}
	g.client = client

	return helpers.ListModels(ctx, client)
}

// GetTokenLimit gets the token limit for a specific model
func (g *GeminiProvider) GetTokenLimit(ctx context.Context, modelName string) (int32, error) {
	if g.client == nil {
		client, err := helpers.CreateClient(ctx, g.config, g.getCurrentAPIKey())
		if err != nil {
			return 0, err
		}
		g.client = client
	}

	return helpers.GetTokenLimit(ctx, g.client, modelName)
}

// CountTokens counts tokens in the given content
func (g *GeminiProvider) CountTokens(ctx context.Context, modelName string, content string) (int32, error) {
	if g.client == nil {
		client, err := helpers.CreateClient(ctx, g.config, g.getCurrentAPIKey())
		if err != nil {
			return 0, err
		}
		g.client = client
	}

	return helpers.CountTokens(ctx, g.client, modelName, content)
}

// TranslateBatch translates a batch of subtitle objects using Gemini
func (g *GeminiProvider) TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []ContextMessage, config *TranslationConfig) (*TranslationResponse, error) {
	if g.client == nil {
		client, err := helpers.CreateClient(ctx, g.config, g.getCurrentAPIKey())
		if err != nil {
			return nil, err
		}
		g.client = client
	}

	// Create generation config
	thinkingCompatible := strings.Contains(config.ModelName, "2.5") || strings.Contains(config.ModelName, "gemini-3")
	instruction := helpers.GetInstruction(
		config.TargetLanguage,
		config.Thinking,
		thinkingCompatible,
		config.Description,
	)
	if config.RetryInstruction != "" {
		instruction += "\n\nRetry correction instruction:\n\n" + config.RetryInstruction
	}

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

	// Build conversation history
	var contents []*genai.Content

	// Add previous context
	if len(previousContext) > 0 {
		for _, msg := range previousContext {
			genaiContent := &genai.Content{
				Parts: []*genai.Part{{Text: msg.Content}},
				Role:  msg.Role,
			}
			contents = append(contents, genaiContent)
		}
	}
	contents = append(contents, currentMessage)

	// Generate response
	if config.ProgressUpdater != nil {
		config.ProgressUpdater.SetLoading(true)
		defer config.ProgressUpdater.SetLoading(false)
	}

	// Use the new API for content generation
	genContentConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: instruction}},
			Role:  "model",
		},
	}

	// Set generation parameters
	genConfig := helpers.GetGenerationConfigForBatch(config.Temperature, config.TopP, config.TopK, len(batch))
	if mimeType, ok := genConfig["response_mime_type"].(string); ok {
		genContentConfig.ResponseMIMEType = mimeType
	}
	if schema, hasSchema := genConfig["response_schema"].(*genai.Schema); hasSchema {
		genContentConfig.ResponseSchema = schema
	}

	genContentConfig.MaxOutputTokens = 65536
	thinkingLevel := parseThinkingLevel(config.ThinkingLevel)
	if !config.Thinking {
		thinkingLevel = genai.ThinkingLevelMinimal
	}
	genContentConfig.ThinkingConfig = &genai.ThinkingConfig{
		ThinkingLevel:   thinkingLevel,
		IncludeThoughts: config.Thinking,
	}

	var responseText string

	if config.Streaming {
		stream := g.client.Models.GenerateContentStream(ctx, config.ModelName, contents, genContentConfig)

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
							if config.ProgressUpdater != nil {
								config.ProgressUpdater.SetThinking(true)
							}
						} else {
							if config.ProgressUpdater != nil {
								config.ProgressUpdater.SetThinking(false)
							}
							if part.Text != "" {
								responseText += part.Text
							}
						}
					}
				}
			}
		}
	} else {
		// Non-streaming mode
		result, errGenerateContent := g.client.Models.GenerateContent(ctx, config.ModelName, contents, genContentConfig)
		if errGenerateContent != nil {
			return nil, fmt.Errorf("generation failed: %v", errGenerateContent)
		}

		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			for _, part := range result.Candidates[0].Content.Parts {
				if !part.Thought && part.Text != "" {
					responseText += part.Text
				}
			}
		}
	}

	// Parse response
	translatedBatch, parsedResponseText, errParse := parseTranslatedBatch(responseText)
	if errParse != nil {
		return nil, errors.NewTranslationError("failed to parse response", errParse).WithContext("response_text", responseText)
	}
	responseText = parsedResponseText

	// Build context for next request
	newContext := []ContextMessage{
		{Role: "user", Content: string(batchData)},
		{Role: "model", Content: responseText},
	}

	return &TranslationResponse{
		TranslatedBatch: translatedBatch,
		Context:         newContext,
	}, nil
}

// SwitchAPIKey switches to the next available API key
func (g *GeminiProvider) SwitchAPIKey() bool {
	if len(g.apiKeys) <= 1 {
		return false
	}
	g.currentAPIIndex = (g.currentAPIIndex + 1) % len(g.apiKeys)
	return true
}

// GetCurrentAPIKeyIndex returns the current API key index
func (g *GeminiProvider) GetCurrentAPIKeyIndex() int {
	return g.currentAPIIndex
}
