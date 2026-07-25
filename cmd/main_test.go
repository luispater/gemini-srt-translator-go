package main

import "testing"

func TestDefaultModelForProvider(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		openAIProtocol string
		want           string
	}{
		{name: "OpenAI Responses", provider: "openai", openAIProtocol: "responses", want: "gpt-5"},
		{name: "OpenAI Chat Completions", provider: "openai", openAIProtocol: "chat-completions", want: "gpt-4o"},
		{name: "OpenAI legacy empty protocol", provider: "openai", want: "gpt-4o"},
		{name: "Gemini", provider: "gemini", openAIProtocol: "responses", want: "gemini-3.5-flash"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := defaultModelForProvider(testCase.provider, testCase.openAIProtocol)
			if got != testCase.want {
				t.Errorf("defaultModelForProvider() = %q, want %q", got, testCase.want)
			}
		})
	}
}
