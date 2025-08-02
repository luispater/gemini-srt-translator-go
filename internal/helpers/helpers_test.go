package helpers

import (
	"context"
	"strings"
	"testing"

	"github.com/luispater/gemini-srt-translator-go/pkg/config"
)

func TestGetInstruction(t *testing.T) {
	tests := []struct {
		name               string
		language           string
		thinking           bool
		thinkingCompatible bool
		description        string
		wantContains       []string
		wantNotContains    []string
	}{
		{
			name:               "basic instruction without thinking",
			language:           "French",
			thinking:           false,
			thinkingCompatible: true,
			description:        "",
			wantContains: []string{
				"translates subtitles from any language to French",
				"Do NOT think or reason",
				"index: a string identifier",
				"content: the text to translate",
			},
			wantNotContains: []string{
				"Think deeply and reason",
				"Additional user instruction",
			},
		},
		{
			name:               "instruction with thinking enabled",
			language:           "Spanish",
			thinking:           true,
			thinkingCompatible: true,
			description:        "",
			wantContains: []string{
				"translates subtitles from any language to Spanish",
				"Think deeply and reason as much as possible",
			},
			wantNotContains: []string{
				"Do NOT think or reason",
				"Additional user instruction",
			},
		},
		{
			name:               "instruction with description",
			language:           "German",
			thinking:           false,
			thinkingCompatible: true,
			description:        "Use formal tone",
			wantContains: []string{
				"translates subtitles from any language to German",
				"Additional user instruction:",
				"Use formal tone",
			},
		},
		{
			name:               "simplified chinese special instructions",
			language:           "Simplified Chinese",
			thinking:           false,
			thinkingCompatible: true,
			description:        "",
			wantContains: []string{
				"translates subtitles from any language to Simplified Chinese",
				"Replace all of the \",\" \".\" \"!\" \"?\" to four spaces",
				"Replace all of the \\n to four spaces",
				"Remove all tags like <i></i>",
			},
		},
		{
			name:               "thinking not compatible",
			language:           "Japanese",
			thinking:           true,
			thinkingCompatible: false,
			description:        "",
			wantContains: []string{
				"translates subtitles from any language to Japanese",
			},
			wantNotContains: []string{
				"Think deeply and reason",
				"Do NOT think or reason",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetInstruction(tt.language, tt.thinking, tt.thinkingCompatible, tt.description)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GetInstruction() result missing expected text: %q", want)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result, notWant) {
					t.Errorf("GetInstruction() result contains unexpected text: %q", notWant)
				}
			}
		})
	}
}

func TestGetResponseSchema(t *testing.T) {
	schema := GetResponseSchema()

	// Check top-level structure
	if schema["type"] != "array" {
		t.Errorf("Expected schema type to be 'array', got %v", schema["type"])
	}

	// Check items structure
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected items to be a map")
	}

	if items["type"] != "object" {
		t.Errorf("Expected items type to be 'object', got %v", items["type"])
	}

	// Check properties
	properties, ok := items["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be a map")
	}

	// Check index property
	indexProp, ok := properties["index"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected index property to exist")
	}
	if indexProp["type"] != "string" {
		t.Errorf("Expected index type to be 'string', got %v", indexProp["type"])
	}

	// Check content property
	contentProp, ok := properties["content"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected content property to exist")
	}
	if contentProp["type"] != "string" {
		t.Errorf("Expected content type to be 'string', got %v", contentProp["type"])
	}

	// Check required fields
	required, ok := items["required"].([]string)
	if !ok {
		t.Fatal("Expected required to be a string slice")
	}
	if len(required) != 2 {
		t.Errorf("Expected 2 required fields, got %d", len(required))
	}
	if !contains(required, "index") {
		t.Error("Expected 'index' to be required")
	}
	if !contains(required, "content") {
		t.Error("Expected 'content' to be required")
	}
}

func TestGetGenerationConfig(t *testing.T) {
	tests := []struct {
		name        string
		temperature *float32
		topP        *float32
		topK        *float32
		wantKeys    []string
	}{
		{
			name:     "all parameters nil",
			wantKeys: []string{"response_mime_type", "response_schema"},
		},
		{
			name:        "with temperature",
			temperature: float32Ptr(0.7),
			wantKeys:    []string{"response_mime_type", "response_schema", "temperature"},
		},
		{
			name:     "with topP",
			topP:     float32Ptr(0.9),
			wantKeys: []string{"response_mime_type", "response_schema", "top_p"},
		},
		{
			name:     "with topK",
			topK:     float32Ptr(40),
			wantKeys: []string{"response_mime_type", "response_schema", "top_k"},
		},
		{
			name:        "with all parameters",
			temperature: float32Ptr(0.8),
			topP:        float32Ptr(0.95),
			topK:        float32Ptr(50),
			wantKeys:    []string{"response_mime_type", "response_schema", "temperature", "top_p", "top_k"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GetGenerationConfig(tt.temperature, tt.topP, tt.topK)

			// Check all expected keys are present
			for _, key := range tt.wantKeys {
				if _, exists := config[key]; !exists {
					t.Errorf("Expected key %q to be present in config", key)
				}
			}

			// Check specific values
			if config["response_mime_type"] != "application/json" {
				t.Errorf("Expected response_mime_type to be 'application/json', got %v", config["response_mime_type"])
			}

			if tt.temperature != nil {
				if config["temperature"] != *tt.temperature {
					t.Errorf("Expected temperature to be %v, got %v", *tt.temperature, config["temperature"])
				}
			}

			if tt.topP != nil {
				if config["top_p"] != *tt.topP {
					t.Errorf("Expected top_p to be %v, got %v", *tt.topP, config["top_p"])
				}
			}

			if tt.topK != nil {
				if config["top_k"] != *tt.topK {
					t.Errorf("Expected top_k to be %v, got %v", *tt.topK, config["top_k"])
				}
			}
		})
	}
}

func TestCreateClient(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		apiKey    string
		wantError bool
	}{
		{
			name: "valid config with base URL",
			cfg: &config.Config{
				BaseURL: "https://custom.api.endpoint",
			},
			apiKey:    "test-api-key",
			wantError: false,
		},
		{
			name:      "valid config without base URL",
			cfg:       &config.Config{},
			apiKey:    "test-api-key",
			wantError: false,
		},
		{
			name:      "empty API key",
			cfg:       &config.Config{},
			apiKey:    "",
			wantError: false, // The client creation might succeed but fail later in actual usage
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client, err := CreateClient(ctx, tt.cfg, tt.apiKey)

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.wantError && client == nil {
				t.Error("Expected client to be non-nil")
			}
		})
	}
}

// Helper functions
func float32Ptr(f float32) *float32 {
	return &f
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
