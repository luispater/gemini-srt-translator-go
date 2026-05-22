package providers

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

var missingContentKeyPattern = regexp.MustCompile(`\{\s*"index"\s*:\s*(-?\d+)\s*(?:,|=)?\s*("(?:(?:\\.)|[^"\\])*")\s*\}`)
var missingContentKeyBeforeGuardPattern = regexp.MustCompile(`\{\s*"index"\s*:\s*(-?\d+)\s*(?:,|=)?\s*("(?:(?:\\.)|[^"\\])*")\s*,\s*"guard"\s*:`)
var strayContentKeyBeforeContentPattern = regexp.MustCompile(`\{\s*"index"\s*:\s*(-?\d+)\s*,\s*"(?:(?:\\.)|[^"\\])*"\s*,?\s*"content"\s*:`)
var indexCommentBeforeContentPattern = regexp.MustCompile(`\{\s*"index"\s*:\s*(-?\d+)\s*://[^\r\n]*(?:\r?\n|\r)\s*"content"\s*:`)
var indexCommentBeforeContentValuePattern = regexp.MustCompile(`\{\s*"index"\s*:\s*(-?\d+)\s*://[^\r\n]*(?:\r?\n|\r)\s*("(?:(?:\\.)|[^"\\])*")\s*,\s*"guard"\s*:`)

func parseTranslatedBatch(responseText string) ([]srt.SubtitleObject, string, error) {
	var translatedBatch []srt.SubtitleObject
	if err := json.Unmarshal([]byte(responseText), &translatedBatch); err != nil {
		repairedResponseText := repairMissingContentKeys(responseText)
		if repairedResponseText == responseText {
			if decodedBatch, firstArrayText, ok := decodeFirstRepeatedArray(responseText); ok {
				return decodedBatch, firstArrayText, nil
			}
			return nil, responseText, err
		}
		if errRepair := json.Unmarshal([]byte(repairedResponseText), &translatedBatch); errRepair != nil {
			if decodedBatch, firstArrayText, ok := decodeFirstRepeatedArray(repairedResponseText); ok {
				return decodedBatch, firstArrayText, nil
			}
			return nil, responseText, err
		}
		return translatedBatch, repairedResponseText, nil
	}

	return translatedBatch, responseText, nil
}

func decodeFirstRepeatedArray(responseText string) ([]srt.SubtitleObject, string, bool) {
	decoder := json.NewDecoder(strings.NewReader(responseText))
	var translatedBatch []srt.SubtitleObject
	if err := decoder.Decode(&translatedBatch); err != nil {
		return nil, "", false
	}

	offset := decoder.InputOffset()
	trailingText := strings.TrimSpace(responseText[offset:])
	if trailingText == "" {
		return translatedBatch, responseText, true
	}
	if !strings.HasPrefix(trailingText, "[") {
		return nil, "", false
	}

	return translatedBatch, strings.TrimSpace(responseText[:offset]), true
}

func repairMissingContentKeys(responseText string) string {
	responseText = repairIndexCommentBeforeContentValue(responseText)
	responseText = repairIndexCommentBeforeContent(responseText)
	responseText = repairStrayContentKeyBeforeContent(responseText)
	responseText = repairMissingContentKeyBeforeGuard(responseText)
	matches := missingContentKeyPattern.FindAllStringSubmatchIndex(responseText, -1)
	if len(matches) == 0 {
		return responseText
	}

	var repaired strings.Builder
	repaired.Grow(len(responseText) + len(matches)*len(`"content":`))
	lastIndex := 0
	for _, match := range matches {
		repaired.WriteString(responseText[lastIndex:match[0]])
		repaired.WriteString(`{"index":`)
		repaired.WriteString(responseText[match[2]:match[3]])
		repaired.WriteString(`,"content":`)
		repaired.WriteString(responseText[match[4]:match[5]])
		repaired.WriteByte('}')
		lastIndex = match[1]
	}
	repaired.WriteString(responseText[lastIndex:])

	return repaired.String()
}

func repairIndexCommentBeforeContentValue(responseText string) string {
	matches := indexCommentBeforeContentValuePattern.FindAllStringSubmatchIndex(responseText, -1)
	if len(matches) == 0 {
		return responseText
	}

	var repaired strings.Builder
	repaired.Grow(len(responseText) + len(matches)*len(`"content":`))
	lastIndex := 0
	for _, match := range matches {
		repaired.WriteString(responseText[lastIndex:match[0]])
		repaired.WriteString(`{"index":`)
		repaired.WriteString(responseText[match[2]:match[3]])
		repaired.WriteString(`,"content":`)
		repaired.WriteString(responseText[match[4]:match[5]])
		repaired.WriteString(`,"guard":`)
		lastIndex = match[1]
	}
	repaired.WriteString(responseText[lastIndex:])

	return repaired.String()
}

func repairIndexCommentBeforeContent(responseText string) string {
	matches := indexCommentBeforeContentPattern.FindAllStringSubmatchIndex(responseText, -1)
	if len(matches) == 0 {
		return responseText
	}

	var repaired strings.Builder
	repaired.Grow(len(responseText))
	lastIndex := 0
	for _, match := range matches {
		repaired.WriteString(responseText[lastIndex:match[0]])
		repaired.WriteString(`{"index":`)
		repaired.WriteString(responseText[match[2]:match[3]])
		repaired.WriteString(`,"content":`)
		lastIndex = match[1]
	}
	repaired.WriteString(responseText[lastIndex:])

	return repaired.String()
}

func repairMissingContentKeyBeforeGuard(responseText string) string {
	matches := missingContentKeyBeforeGuardPattern.FindAllStringSubmatchIndex(responseText, -1)
	if len(matches) == 0 {
		return responseText
	}

	var repaired strings.Builder
	repaired.Grow(len(responseText) + len(matches)*len(`"content":`))
	lastIndex := 0
	for _, match := range matches {
		repaired.WriteString(responseText[lastIndex:match[0]])
		repaired.WriteString(`{"index":`)
		repaired.WriteString(responseText[match[2]:match[3]])
		repaired.WriteString(`,"content":`)
		repaired.WriteString(responseText[match[4]:match[5]])
		repaired.WriteString(`,"guard":`)
		lastIndex = match[1]
	}
	repaired.WriteString(responseText[lastIndex:])

	return repaired.String()
}

func repairStrayContentKeyBeforeContent(responseText string) string {
	matches := strayContentKeyBeforeContentPattern.FindAllStringSubmatchIndex(responseText, -1)
	if len(matches) == 0 {
		return responseText
	}

	var repaired strings.Builder
	repaired.Grow(len(responseText))
	lastIndex := 0
	for _, match := range matches {
		repaired.WriteString(responseText[lastIndex:match[0]])
		repaired.WriteString(`{"index":`)
		repaired.WriteString(responseText[match[2]:match[3]])
		repaired.WriteString(`,"content":`)
		lastIndex = match[1]
	}
	repaired.WriteString(responseText[lastIndex:])

	return repaired.String()
}
