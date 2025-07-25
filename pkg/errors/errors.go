package errors

import (
	"fmt"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrorTypeValidation   ErrorType = "validation"
	ErrorTypeAPI          ErrorType = "api"
	ErrorTypeFile         ErrorType = "file"
	ErrorTypeTranslation  ErrorType = "translation"
	ErrorTypeConfiguration ErrorType = "configuration"
	ErrorTypeNetwork      ErrorType = "network"
)

// TranslatorError represents a structured error with context
type TranslatorError struct {
	Type     ErrorType
	Message  string
	Cause    error
	Context  map[string]interface{}
}

// Error implements the error interface
func (e *TranslatorError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s error: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s error: %s", e.Type, e.Message)
}

// Unwrap returns the underlying cause
func (e *TranslatorError) Unwrap() error {
	return e.Cause
}

// WithContext adds context information to the error
func (e *TranslatorError) WithContext(key string, value interface{}) *TranslatorError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewValidationError creates a new validation error
func NewValidationError(message string, cause error) *TranslatorError {
	return &TranslatorError{
		Type:    ErrorTypeValidation,
		Message: message,
		Cause:   cause,
	}
}

// NewAPIError creates a new API-related error
func NewAPIError(message string, cause error) *TranslatorError {
	return &TranslatorError{
		Type:    ErrorTypeAPI,
		Message: message,
		Cause:   cause,
	}
}

// NewFileError creates a new file-related error
func NewFileError(message string, cause error) *TranslatorError {
	return &TranslatorError{
		Type:    ErrorTypeFile,
		Message: message,
		Cause:   cause,
	}
}

// NewTranslationError creates a new translation-related error
func NewTranslationError(message string, cause error) *TranslatorError {
	return &TranslatorError{
		Type:    ErrorTypeTranslation,
		Message: message,
		Cause:   cause,
	}
}

// NewConfigurationError creates a new configuration-related error
func NewConfigurationError(message string, cause error) *TranslatorError {
	return &TranslatorError{
		Type:    ErrorTypeConfiguration,
		Message: message,
		Cause:   cause,
	}
}

// NewNetworkError creates a new network-related error
func NewNetworkError(message string, cause error) *TranslatorError {
	return &TranslatorError{
		Type:    ErrorTypeNetwork,
		Message: message,
		Cause:   cause,
	}
}