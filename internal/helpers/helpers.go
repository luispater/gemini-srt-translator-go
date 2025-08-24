package helpers

import (
	"context"
	"fmt"
	"slices"

	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"google.golang.org/genai"
)

// GetInstruction generates the system instruction for the translation model
func GetInstruction(language string, thinking bool, thinkingCompatible bool, description string) string {
	thinkingInstruction := ""
	if thinking {
		thinkingInstruction = "\nThink deeply and reason as much as possible before returning the response."
	} else {
		thinkingInstruction = "\nDo NOT think or reason."
	}

	fields := "- index: a string identifier\n- content: the text to translate\n"

	instruction := fmt.Sprintf(`You are an assistant that translates subtitles from any language to %s.
You will receive a list of objects, each with these fields:

%s

Translate the 'content' field of each object.
You *MUST* return all of translated objects in the response, *MUST NOT* skip any objects. If I send 300 objects, you *MUST* return 300 objects.
If the 'content' field is empty, leave it as is.
Preserve line breaks, formatting, and special characters.
You *MUST NOT* move or merge 'content' between objects.
You *MUST NOT* add or remove any objects.
You *MUST NOT* alter the 'index' field.

If the target language is *Simplified Chinese*, please forward these instruction:
You *MUST* Replace all of the "," "." "!" "?" to four spaces.
You *MUST* Replace all of the \n to four spaces.
You *MUST* Trim all the invisible characters at the beginning and end of the 'content' field.
You *MUST* Remove all tags like <i></i>, but keep their content.
You *MUST* Remove all invisible characters after ":" or "：" in the 'content' field.
`, language, fields)

	if thinkingCompatible {
		instruction += thinkingInstruction
	}

	if description != "" {
		instruction += fmt.Sprintf("\n\nAdditional user instruction:\n\n%s", description)
	}

	return instruction
}

// GetResponseSchema returns the JSON schema for the translation response
func GetResponseSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"index": map[string]interface{}{
					"type": "string",
				},
				"content": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"index", "content"},
		},
	}
}

// GetGenerationConfig creates the generation configuration
func GetGenerationConfig(temperature *float32, topP *float32, topK *float32) map[string]interface{} {
	cfg := map[string]interface{}{
		"response_mime_type": "application/json",
		"response_schema":    GetResponseSchema(),
	}

	if temperature != nil {
		cfg["temperature"] = *temperature
	}
	if topP != nil {
		cfg["top_p"] = *topP
	}
	if topK != nil {
		cfg["top_k"] = *topK
	}

	return cfg
}

// CreateClient creates a new Gemini client
func CreateClient(ctx context.Context, cfg *config.Config, apiKey string) (*genai.Client, error) {
	// logger.Info(fmt.Sprintf("Creating Gemini client with API key: %s", apiKey))
	clientConfig := &genai.ClientConfig{
		APIKey: apiKey,
	}
	if cfg.BaseURL != "" {
		clientConfig.HTTPOptions.BaseURL = cfg.BaseURL
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	return client, nil
}

// ListModels lists available Gemini models that support content generation
func ListModels(ctx context.Context, client *genai.Client) ([]string, error) {
	m := make([]string, 0)
	models, err := client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, err
	}
	for _, model := range models.Items {
		if slices.Contains(model.SupportedActions, "generateContent") {
			m = append(m, model.Name)
		}
	}
	return m, nil
}

// GetTokenLimit gets the token limit for a specific model
func GetTokenLimit(ctx context.Context, client *genai.Client, modelName string) (int32, error) {
	// Return reasonable defaults based on the model name
	model, err := client.Models.Get(ctx, modelName, &genai.GetModelConfig{})
	if err != nil {
		return 0, err
	}
	return model.OutputTokenLimit, nil
}

// CountTokens counts tokens in the given content
func CountTokens(ctx context.Context, client *genai.Client, modelName string, content string) (int32, error) {
	contents := []*genai.Content{
		genai.NewContentFromText(content, genai.RoleUser),
	}
	resp, err := client.Models.CountTokens(ctx, modelName, contents, &genai.CountTokensConfig{})
	if err != nil {
		return 0, err
	}
	return resp.TotalTokens, nil
}
