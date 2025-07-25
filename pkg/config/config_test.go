package config

import (
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	// Save original environment
	originalAPIKey := os.Getenv("GEMINI_API_KEY")
	defer func() {
		if originalAPIKey != "" {
			os.Setenv("GEMINI_API_KEY", originalAPIKey)
		} else {
			os.Unsetenv("GEMINI_API_KEY")
		}
	}()

	// Test with no API key
	os.Unsetenv("GEMINI_API_KEY")
	cfg := NewConfig()

	if len(cfg.GeminiAPIKeys) != 0 {
		t.Errorf("Expected empty API keys, got %v", cfg.GeminiAPIKeys)
	}
	if cfg.ModelName != "gemini-2.5-pro" {
		t.Errorf("Expected model name 'gemini-2.5-pro', got %v", cfg.ModelName)
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
	if cfg.ThinkingBudget != 2048 {
		t.Errorf("Expected thinking budget 2048, got %v", cfg.ThinkingBudget)
	}
	if !cfg.FreeQuota {
		t.Error("Expected free quota to be true")
	}
	if !cfg.UseColors {
		t.Error("Expected use colors to be true")
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
				os.Setenv("TEST_API_KEY", tt.envValue)
			} else {
				os.Unsetenv("TEST_API_KEY")
			}
			defer os.Unsetenv("TEST_API_KEY")

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
