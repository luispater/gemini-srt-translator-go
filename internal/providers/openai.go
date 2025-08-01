package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

// OpenAIProvider implements TranslationProvider for OpenAI
type OpenAIProvider struct {
	config          *config.Config
	client          *openai.Client
	apiKeys         []string
	currentAPIIndex int
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(cfg *config.Config) (*OpenAIProvider, error) {
	return &OpenAIProvider{
		config:          cfg,
		apiKeys:         cfg.APIKeys,
		currentAPIIndex: 0,
	}, nil
}

// GetName returns the provider name
func (o *OpenAIProvider) GetName() string {
	return "openai"
}

// getCurrentAPIKey returns the current API key if available
func (o *OpenAIProvider) getCurrentAPIKey() string {
	if len(o.apiKeys) == 0 || o.currentAPIIndex >= len(o.apiKeys) {
		return ""
	}
	return o.apiKeys[o.currentAPIIndex]
}

// createClient creates OpenAI client with current API key
func (o *OpenAIProvider) createClient() error {
	apiKey := o.getCurrentAPIKey()
	if apiKey == "" {
		return errors.NewValidationError("no OpenAI API key available", nil)
	}

	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if o.config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(o.config.BaseURL))
	}

	client := openai.NewClient(opts...)
	o.client = &client
	return nil
}

// GetModels returns available OpenAI models
func (o *OpenAIProvider) GetModels(ctx context.Context) ([]string, error) {
	if len(o.apiKeys) == 0 {
		return nil, errors.NewValidationError("please provide a valid OpenAI API key", nil)
	}

	if err := o.createClient(); err != nil {
		return nil, err
	}

	modelsResp, err := o.client.Models.List(ctx)
	if err != nil {
		return nil, errors.NewAPIError("failed to list OpenAI models", err)
	}

	var models []string
	for _, model := range modelsResp.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

// GetTokenLimit returns the token limit for a specific OpenAI model
func (o *OpenAIProvider) GetTokenLimit(ctx context.Context, modelName string) (int32, error) {
	// Return known token limits for OpenAI models
	switch {
	case strings.Contains(modelName, "gpt-4o"):
		return 128000, nil
	case strings.Contains(modelName, "gpt-4-turbo"):
		return 128000, nil
	case strings.Contains(modelName, "gpt-4"):
		return 8192, nil
	case strings.Contains(modelName, "gpt-3.5-turbo"):
		return 16385, nil
	default:
		return 128000, nil // Conservative default
	}
}

// CountTokens estimates token count for OpenAI models
func (o *OpenAIProvider) CountTokens(ctx context.Context, modelName string, content string) (int32, error) {
	// Simple estimation: ~4 characters per token for most languages
	// This is a rough approximation; for precise counting, you'd need tiktoken
	estimatedTokens := int32(len(content) / 4)
	return estimatedTokens, nil
}

// TranslateBatch translates a batch of subtitle objects using OpenAI
func (o *OpenAIProvider) TranslateBatch(ctx context.Context, batch []srt.SubtitleObject, previousContext []ContextMessage, config *TranslationConfig) (*TranslationResponse, error) {
	if o.client == nil {
		if err := o.createClient(); err != nil {
			return nil, err
		}
	}

	// Build system instruction
	instruction := o.getInstruction(config.TargetLanguage, config.Description)

	// Build messages
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(instruction),
	}

	// Add previous context
	for _, msg := range previousContext {
		switch msg.Role {
		case "user":
			messages = append(messages, openai.UserMessage(msg.Content))
		case "assistant", "model":
			messages = append(messages, openai.AssistantMessage(msg.Content))
		}
	}

	// Add current batch
	batchData, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch: %w", err)
	}
	messages = append(messages, openai.UserMessage(string(batchData)))

	// Prepare request parameters
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModelGPT4o, // Default model
		Messages: messages,
	}

	if config.ModelName != "" {
		params.Model = config.ModelName
	}

	// Set optional parameters
	if config.Temperature != nil {
		params.Temperature = openai.Float(float64(*config.Temperature))
	}
	if config.TopP != nil {
		params.TopP = openai.Float(float64(*config.TopP))
	}

	// Update progress
	if config.ProgressUpdater != nil {
		config.ProgressUpdater.SetLoading(true)
		defer config.ProgressUpdater.SetLoading(false)
	}

	var responseText string

	if config.Streaming {
		// Streaming mode
		stream := o.client.Chat.Completions.NewStreaming(ctx, params)

		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.Content) > 0 {
				responseText += chunk.Choices[0].Delta.Content
			}
		}

		if err = stream.Err(); err != nil {
			return nil, fmt.Errorf("streaming failed: %w", err)
		}
	} else {
		// Non-streaming mode
		completion, errNew := o.client.Chat.Completions.New(ctx, params)
		if errNew != nil {
			return nil, fmt.Errorf("completion failed: %w", errNew)
		}

		if len(completion.Choices) > 0 {
			responseText = completion.Choices[0].Message.Content
		}
	}

	// Parse response
	var translatedBatch []srt.SubtitleObject
	if err = json.Unmarshal([]byte(responseText), &translatedBatch); err != nil {
		return nil, errors.NewTranslationError("failed to parse response", err).WithContext("response_text", responseText)
	}

	// Build context for next request
	newContext := []ContextMessage{
		{Role: "user", Content: string(batchData)},
		{Role: "assistant", Content: responseText},
	}

	return &TranslationResponse{
		TranslatedBatch: translatedBatch,
		Context:         newContext,
	}, nil
}

// getInstruction generates the system instruction for OpenAI translation
func (o *OpenAIProvider) getInstruction(language string, description string) string {
	fields := "- index: a string identifier\n- content: the text to translate\n"

	instruction := fmt.Sprintf(`You are an assistant that translates subtitles from any language to %s.
You will receive a list of objects, each with these fields:

%s

Translate the 'content' field of each object.
If the 'content' field is empty, leave it as is.
Preserve line breaks, formatting, and special characters.
Do NOT move or merge 'content' between objects.
Do NOT add or remove any objects.
Do NOT alter the 'index' field.

Return the result as a JSON array with the same structure.

If the target language is *Simplified Chinese*, please follow these instructions:
Replace all of the "," "." "!" "?" to four spaces.
Replace all of the \n to four spaces.
Trim all the invisible characters at the beginning and end of the 'content' field.
Remove all tags like <i></i>, but keep their content.
Remove all invisible characters after ":" or "：" in the 'content' field.
`, language, fields)

	if description != "" {
		instruction += fmt.Sprintf("\n\nAdditional user instruction:\n\n%s", description)
	}

	return instruction
}

// SwitchAPIKey switches to the next available API key
func (o *OpenAIProvider) SwitchAPIKey() bool {
	if len(o.apiKeys) <= 1 {
		return false
	}
	o.currentAPIIndex = (o.currentAPIIndex + 1) % len(o.apiKeys)
	return true
}

// GetCurrentAPIKeyIndex returns the current API key index
func (o *OpenAIProvider) GetCurrentAPIKeyIndex() int {
	return o.currentAPIIndex
}
