package errors

import (
	"errors"
	"testing"
)

func TestTranslatorError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *TranslatorError
		expected string
	}{
		{
			name: "error without cause",
			err: &TranslatorError{
				Type:    ErrorTypeValidation,
				Message: "test validation error",
			},
			expected: "validation error: test validation error",
		},
		{
			name: "error with cause",
			err: &TranslatorError{
				Type:    ErrorTypeAPI,
				Message: "test API error",
				Cause:   errors.New("underlying error"),
			},
			expected: "api error: test API error (caused by: underlying error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("TranslatorError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTranslatorError_WithContext(t *testing.T) {
	err := NewValidationError("test error", nil)
	_ = err.WithContext("key1", "value1").WithContext("key2", 42)

	if err.Context["key1"] != "value1" {
		t.Errorf("Expected context key1 to be 'value1', got %v", err.Context["key1"])
	}
	if err.Context["key2"] != 42 {
		t.Errorf("Expected context key2 to be 42, got %v", err.Context["key2"])
	}
}

func TestNewValidationError(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewValidationError("validation failed", cause)

	if err.Type != ErrorTypeValidation {
		t.Errorf("Expected type %v, got %v", ErrorTypeValidation, err.Type)
	}
	if err.Message != "validation failed" {
		t.Errorf("Expected message 'validation failed', got %v", err.Message)
	}
	if !errors.Is(cause, err.Cause) {
		t.Errorf("Expected cause to be %v, got %v", cause, err.Cause)
	}
}

func TestNewAPIError(t *testing.T) {
	err := NewAPIError("API request failed", nil)
	if err.Type != ErrorTypeAPI {
		t.Errorf("Expected type %v, got %v", ErrorTypeAPI, err.Type)
	}
}

func TestNewFileError(t *testing.T) {
	err := NewFileError("file not found", nil)
	if err.Type != ErrorTypeFile {
		t.Errorf("Expected type %v, got %v", ErrorTypeFile, err.Type)
	}
}

func TestNewTranslationError(t *testing.T) {
	err := NewTranslationError("translation failed", nil)
	if err.Type != ErrorTypeTranslation {
		t.Errorf("Expected type %v, got %v", ErrorTypeTranslation, err.Type)
	}
}

func TestNewConfigurationError(t *testing.T) {
	err := NewConfigurationError("invalid configuration", nil)
	if err.Type != ErrorTypeConfiguration {
		t.Errorf("Expected type %v, got %v", ErrorTypeConfiguration, err.Type)
	}
}

func TestNewNetworkError(t *testing.T) {
	err := NewNetworkError("network timeout", nil)
	if err.Type != ErrorTypeNetwork {
		t.Errorf("Expected type %v, got %v", ErrorTypeNetwork, err.Type)
	}
}
