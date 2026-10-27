package providers

import (
	"testing"

	"google.golang.org/genai"
)

func TestParseThinkingLevel(t *testing.T) {
	tests := []struct {
		level string
		want  genai.ThinkingLevel
	}{
		{level: "minimal", want: genai.ThinkingLevelMinimal},
		{level: "low", want: genai.ThinkingLevelLow},
		{level: "medium", want: genai.ThinkingLevelMedium},
		{level: "high", want: genai.ThinkingLevelHigh},
		{level: "xhigh", want: genai.ThinkingLevelHigh},
		{level: "max", want: genai.ThinkingLevelHigh},
		{level: " XHIGH ", want: genai.ThinkingLevelHigh},
		{level: " MAX ", want: genai.ThinkingLevelHigh},
	}

	for _, testCase := range tests {
		t.Run(testCase.level, func(t *testing.T) {
			if got := parseThinkingLevel(testCase.level); got != testCase.want {
				t.Errorf("parseThinkingLevel(%q) = %q, want %q", testCase.level, got, testCase.want)
			}
		})
	}
}
