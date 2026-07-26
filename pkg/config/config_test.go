package config

import (
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	t.Setenv(EnvProvider, "")
	t.Setenv(EnvOpenAIProtocol, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvModel, "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "")

	cfg := NewConfig()

	if cfg.Provider != "gemini" {
		t.Errorf("Expected provider 'gemini', got %q", cfg.Provider)
	}
	if len(cfg.APIKeys) != 0 {
		t.Errorf("Expected empty API keys, got %v", cfg.APIKeys)
	}
	if cfg.BaseURL != "" {
		t.Errorf("Expected empty base URL, got %q", cfg.BaseURL)
	}
	if cfg.ModelName != "gemini-3.5-flash" {
		t.Errorf("Expected model name 'gemini-3.5-flash', got %v", cfg.ModelName)
	}
	if cfg.OpenAIProtocol != "chat-completions" {
		t.Errorf("Expected OpenAI protocol 'chat-completions', got %v", cfg.OpenAIProtocol)
	}
	if cfg.BatchSize != 300 {
		t.Errorf("Expected batch size 300, got %v", cfg.BatchSize)
	}
	if !cfg.Streaming {
		t.Error("Expected streaming to be true")
	}
	if !cfg.Thinking {
		t.Error("Expected thinking to be true")
	}
	if cfg.ThinkingLevel != "high" {
		t.Errorf("Expected thinking level high, got %v", cfg.ThinkingLevel)
	}
	if !cfg.FreeQuota {
		t.Error("Expected free quota to be true")
	}
	if !cfg.UseColors {
		t.Error("Expected use colors to be true")
	}
}

func TestNewConfigFromSRTEnvironment(t *testing.T) {
	t.Setenv(EnvProvider, "openai")
	t.Setenv(EnvOpenAIProtocol, "responses")
	t.Setenv(EnvBaseURL, "https://example.com/v1")
	t.Setenv(EnvAPIKey, "key1, key2")
	t.Setenv(EnvModel, "gpt-test")
	t.Setenv("GEMINI_API_KEY", "legacy-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://legacy.example.com")

	cfg := NewConfig()

	if cfg.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got %q", cfg.Provider)
	}
	if cfg.OpenAIProtocol != "responses" {
		t.Errorf("Expected OpenAI protocol 'responses', got %q", cfg.OpenAIProtocol)
	}
	if cfg.BaseURL != "https://example.com/v1" {
		t.Errorf("Expected SRT translator base URL, got %q", cfg.BaseURL)
	}
	if cfg.ModelName != "gpt-test" {
		t.Errorf("Expected model name 'gpt-test', got %q", cfg.ModelName)
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "key1" || cfg.APIKeys[1] != "key2" {
		t.Errorf("Expected SRT translator API keys, got %v", cfg.APIKeys)
	}
}

func TestNewConfigDoesNotUseLegacyAPIKeysWhenUnifiedValueHasNoKeys(t *testing.T) {
	t.Setenv(EnvProvider, "openai")
	t.Setenv(EnvAPIKey, " , ")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("OPENAI_API_KEY", "openai-key")

	cfg := NewConfig()

	if len(cfg.APIKeys) != 0 {
		t.Errorf("API keys = %v, want no legacy API keys", cfg.APIKeys)
	}
}

func TestNewConfigUsesProviderDefaultModel(t *testing.T) {
	t.Setenv(EnvModel, "")

	tests := []struct {
		name           string
		provider       string
		openAIProtocol string
		wantModel      string
	}{
		{
			name:           "OpenAI Responses",
			provider:       "openai",
			openAIProtocol: "responses",
			wantModel:      "gpt-5",
		},
		{
			name:           "OpenAI Chat Completions",
			provider:       "openai",
			openAIProtocol: "chat-completions",
			wantModel:      "gpt-4o",
		},
		{
			name:           "OpenAI other protocol",
			provider:       "openai",
			openAIProtocol: "other",
			wantModel:      "gpt-4o",
		},
		{
			name:           "Gemini",
			provider:       "gemini",
			openAIProtocol: "responses",
			wantModel:      "gemini-3.5-flash",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(EnvProvider, testCase.provider)
			t.Setenv(EnvOpenAIProtocol, testCase.openAIProtocol)

			cfg := NewConfig()

			if cfg.ModelName != testCase.wantModel {
				t.Errorf("Model name = %q, want %q", cfg.ModelName, testCase.wantModel)
			}
		})
	}
}

func TestNewConfigUsesProviderSpecificLegacyEnvironment(t *testing.T) {
	t.Setenv(EnvOpenAIProtocol, "")
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvModel, "")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://gemini.example.com")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENAI_BASE_URL", "https://openai.example.com")

	tests := []struct {
		name        string
		provider    string
		wantAPIKey  string
		wantBaseURL string
	}{
		{
			name:        "Gemini legacy environment",
			provider:    "gemini",
			wantAPIKey:  "gemini-key",
			wantBaseURL: "https://gemini.example.com",
		},
		{
			name:        "OpenAI legacy environment",
			provider:    "openai",
			wantAPIKey:  "openai-key",
			wantBaseURL: "https://openai.example.com",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(EnvProvider, testCase.provider)
			cfg := NewConfig()

			if len(cfg.APIKeys) != 1 || cfg.APIKeys[0] != testCase.wantAPIKey {
				t.Errorf("API keys = %v, want [%q]", cfg.APIKeys, testCase.wantAPIKey)
			}
			if cfg.BaseURL != testCase.wantBaseURL {
				t.Errorf("Base URL = %q, want %q", cfg.BaseURL, testCase.wantBaseURL)
			}
		})
	}
}

func TestConfigCloneCreatesIndependentTaskConfiguration(t *testing.T) {
	temperature := float32(0.7)
	topP := float32(0.8)
	topK := float32(20)
	resume := true
	original := &Config{
		APIKeys:     []string{"key-1", "key-2"},
		InputFile:   "first.srt",
		Temperature: &temperature,
		TopP:        &topP,
		TopK:        &topK,
		Resume:      &resume,
	}

	cloned := original.Clone()
	cloned.APIKeys[0] = "changed"
	cloned.InputFile = "second.srt"
	*cloned.Temperature = 0.1
	*cloned.TopP = 0.2
	*cloned.TopK = 1
	*cloned.Resume = false

	if original.APIKeys[0] != "key-1" {
		t.Errorf("Original API keys changed to %v", original.APIKeys)
	}
	if original.InputFile != "first.srt" {
		t.Errorf("Original input file changed to %q", original.InputFile)
	}
	if *original.Temperature != 0.7 || *original.TopP != 0.8 || *original.TopK != 20 || !*original.Resume {
		t.Error("Clone shares pointer fields with the original configuration")
	}
}

func TestParseAPIKeys(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "empty environment variable",
			envValue: "",
			expected: []string{},
		},
		{
			name:     "single API key",
			envValue: "key1",
			expected: []string{"key1"},
		},
		{
			name:     "multiple API keys",
			envValue: "key1,key2,key3",
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name:     "API keys with spaces",
			envValue: " key1 , key2 , key3 ",
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name:     "API keys with empty entries",
			envValue: "key1,,key2,",
			expected: []string{"key1", "key2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				_ = os.Setenv("TEST_API_KEY", tt.envValue)
			} else {
				_ = os.Unsetenv("TEST_API_KEY")
			}
			defer func() {
				_ = os.Unsetenv("TEST_API_KEY")
			}()

			result := parseAPIKeys("TEST_API_KEY")

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected key %d to be '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}
