package providers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"

	"github.com/luispater/gemini-srt-translator-go/pkg/errors"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

const (
	readTranslationBatchTool = "read_translation_batch"
	submitTranslationsTool   = "submit_translations"
)

type translationBatchToolOutput struct {
	Records []srt.SubtitleObject `json:"records"`
}

type submitTranslationsArguments struct {
	Records []srt.SubtitleObject `json:"records"`
}

// translateBatchWithResponses translates subtitles through the Responses function-calling protocol.
func (o *OpenAIProvider) translateBatchWithResponses(ctx context.Context, batch []srt.SubtitleObject, previousContext []ContextMessage, translationConfig *TranslationConfig) (*TranslationResponse, error) {
	if o.client == nil {
		if err := o.createClient(); err != nil {
			return nil, err
		}
	}

	input, err := responseInputFromContext(previousContext)
	if err != nil {
		return nil, errors.NewTranslationError("failed to restore Responses context", err)
	}

	currentContext := make([]ContextMessage, 0, 6)
	requestText := "Translate the next subtitle batch. Read it with the provided tool, then submit the complete translation with the submission tool."
	requestMessage := responses.ResponseInputItemParamOfMessage(requestText, responses.EasyInputMessageRoleUser)
	input = append(input, requestMessage)
	currentContext = append(currentContext, ContextMessage{Role: "user", Content: requestText})

	if translationConfig.ProgressUpdater != nil {
		translationConfig.ProgressUpdater.SetLoading(true)
		defer translationConfig.ProgressUpdater.SetLoading(false)
	}

	readResponse, err := o.sendResponsesRequest(
		ctx,
		input,
		translationConfig,
		o.getResponsesInstruction(translationConfig),
		readTranslationBatchDefinition(),
		readTranslationBatchTool,
	)
	if err != nil {
		return nil, err
	}

	input, err = appendResponseOutput(input, &currentContext, readResponse.Output)
	if err != nil {
		return nil, errors.NewTranslationError("failed to preserve Responses output context", err)
	}

	readCall, err := requiredFunctionCall(readResponse.Output, readTranslationBatchTool)
	if err != nil {
		return nil, errors.NewTranslationError("failed to read translation batch through tool call", err)
	}
	if err = validateReadToolArguments(readCall.Arguments); err != nil {
		return nil, errors.NewTranslationError("invalid read_translation_batch arguments", err)
	}

	batchOutput, err := json.Marshal(translationBatchToolOutput{Records: batch})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch tool output: %w", err)
	}
	readCallOutput := responses.ResponseInputItemParamOfFunctionCallOutput(readCall.CallID, string(batchOutput))
	input = append(input, readCallOutput)
	if err = appendResponseInputContext(&currentContext, readCallOutput); err != nil {
		return nil, errors.NewTranslationError("failed to preserve batch tool output context", err)
	}

	submitResponse, err := o.sendResponsesRequest(
		ctx,
		input,
		translationConfig,
		o.getResponsesInstruction(translationConfig),
		submitTranslationsDefinition(),
		submitTranslationsTool,
	)
	if err != nil {
		return nil, err
	}

	_, err = appendResponseOutput(nil, &currentContext, submitResponse.Output)
	if err != nil {
		return nil, errors.NewTranslationError("failed to preserve Responses submission context", err)
	}

	submitCall, err := requiredFunctionCall(submitResponse.Output, submitTranslationsTool)
	if err != nil {
		return nil, errors.NewTranslationError("failed to receive translations through tool call", err)
	}

	var submitted submitTranslationsArguments
	if err = json.Unmarshal([]byte(submitCall.Arguments), &submitted); err != nil {
		return nil, errors.NewTranslationError("failed to decode submit_translations arguments", err).WithContext("response_text", submitCall.Arguments)
	}

	submitCallOutput := responses.ResponseInputItemParamOfFunctionCallOutput(submitCall.CallID, `{"accepted":true}`)
	if err = appendResponseInputContext(&currentContext, submitCallOutput); err != nil {
		return nil, errors.NewTranslationError("failed to preserve submission acknowledgement context", err)
	}

	return &TranslationResponse{
		TranslatedBatch: submitted.Records,
		Context:         currentContext,
	}, nil
}

func (o *OpenAIProvider) sendResponsesRequest(ctx context.Context, input responses.ResponseInputParam, translationConfig *TranslationConfig, instruction string, tool responses.ToolUnionParam, toolName string) (*responses.Response, error) {
	promptCacheKey, errPromptCacheKey := o.getOrCreatePromptCacheKey()
	if errPromptCacheKey != nil {
		return nil, errPromptCacheKey
	}

	sanitizedInput := sanitizeResponsesInput(input)

	params := responses.ResponseNewParams{
		Instructions:      openai.String(instruction),
		Input:             responses.ResponseNewParamsInputUnion{OfInputItemList: sanitizedInput},
		Model:             openai.ResponsesModel(translationConfig.ModelName),
		Include:           []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent},
		ParallelToolCalls: openai.Bool(false),
		PromptCacheKey:    openai.String(promptCacheKey),
		Store:             openai.Bool(false),
		Tools:             []responses.ToolUnionParam{tool},
		ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &responses.ToolChoiceFunctionParam{Name: toolName},
		},
	}

	supportsReasoning := supportsResponsesReasoning(translationConfig.ModelName)
	if !supportsReasoning {
		if translationConfig.Temperature != nil {
			params.Temperature = openai.Float(float64(*translationConfig.Temperature))
		}
		if translationConfig.TopP != nil {
			params.TopP = openai.Float(float64(*translationConfig.TopP))
		}
	} else {
		reasoningLevel := translationConfig.ThinkingLevel
		if !translationConfig.Thinking {
			// The Responses API cannot fully disable reasoning for these models, so request the lowest supported effort without a summary.
			reasoningLevel = "minimal"
		}
		params.Reasoning = responses.ReasoningParam{
			Effort: responsesReasoningEffort(translationConfig.ModelName, reasoningLevel),
		}
		if translationConfig.Thinking {
			params.Reasoning.Summary = responses.ReasoningSummaryAuto
		}
	}

	if translationConfig.ProgressUpdater != nil {
		translationConfig.ProgressUpdater.SetThinking(translationConfig.Thinking)
		defer translationConfig.ProgressUpdater.SetThinking(false)
	}

	if !translationConfig.Streaming {
		response, err := o.client.Responses.New(ctx, params)
		if err != nil {
			return nil, errors.NewAPIError("Responses request failed", err)
		}
		if err = validateCompletedResponse(response); err != nil {
			return nil, err
		}
		return response, nil
	}

	return o.sendStreamingResponsesRequest(ctx, params)
}

func (o *OpenAIProvider) getOrCreatePromptCacheKey() (string, error) {
	o.promptCacheKeyLock.Lock()
	defer o.promptCacheKeyLock.Unlock()

	if o.promptCacheKey != "" {
		return o.promptCacheKey, nil
	}
	if o.config.OpenAIPromptCacheKey != "" {
		o.promptCacheKey = o.config.OpenAIPromptCacheKey
		return o.promptCacheKey, nil
	}

	var uuidBytes [16]byte
	if _, errRead := rand.Read(uuidBytes[:]); errRead != nil {
		return "", errors.NewAPIError("failed to generate OpenAI prompt cache key", errRead)
	}
	uuidBytes[6] = uuidBytes[6]&0x0f | 0x40
	uuidBytes[8] = uuidBytes[8]&0x3f | 0x80
	o.promptCacheKey = fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		uuidBytes[0:4],
		uuidBytes[4:6],
		uuidBytes[6:8],
		uuidBytes[8:10],
		uuidBytes[10:16],
	)
	o.config.OpenAIPromptCacheKey = o.promptCacheKey
	return o.promptCacheKey, nil
}

func (o *OpenAIProvider) sendStreamingResponsesRequest(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
	var rawResponse *http.Response
	errPost := o.client.Post(
		ctx,
		"responses",
		params,
		&rawResponse,
		option.WithJSONSet("stream", true),
		option.WithHeader("Accept", "text/event-stream"),
	)
	if errPost != nil {
		return nil, errors.NewAPIError("Responses streaming request failed", errPost)
	}
	if rawResponse == nil || rawResponse.Body == nil {
		return nil, errors.NewAPIError("Responses streaming request returned an empty HTTP response", nil)
	}

	completedResponse, errConsume := consumeResponsesStream(rawResponse.Body)
	errClose := rawResponse.Body.Close()
	if errConsume != nil {
		return nil, errConsume
	}
	if errClose != nil {
		return nil, errors.NewAPIError("failed to close Responses stream", errClose)
	}
	return completedResponse, nil
}

func consumeResponsesStream(reader io.Reader) (*responses.Response, error) {
	const maxStreamEventSize = 64 * 1024 * 1024

	return consumeResponsesStreamWithLimit(reader, maxStreamEventSize)
}

func consumeResponsesStreamWithLimit(reader io.Reader, maxStreamEventSize int) (*responses.Response, error) {
	if maxStreamEventSize <= 0 {
		return nil, errors.NewAPIError("Responses stream event size limit must be positive", nil)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, min(64*1024, maxStreamEventSize)), maxStreamEventSize)

	var eventData []byte
	var completedResponse *responses.Response
	var terminalErr error
	streamDone := false

	processEvent := func(data []byte) error {
		trimmedData := bytes.TrimSpace(data)
		if len(trimmedData) == 0 {
			return nil
		}
		if bytes.Equal(trimmedData, []byte("[DONE]")) {
			streamDone = true
			return nil
		}

		var event responses.ResponseStreamEventUnion
		if errUnmarshal := json.Unmarshal(trimmedData, &event); errUnmarshal != nil {
			return errors.NewAPIError("failed to decode Responses stream event", errUnmarshal)
		}
		switch event.Type {
		case "response.completed":
			completed := event.AsResponseCompleted()
			responseCopy := completed.Response
			completedResponse = &responseCopy
		case "response.failed":
			failed := event.AsResponseFailed()
			terminalErr = validateCompletedResponse(&failed.Response)
		case "response.incomplete":
			incomplete := event.AsResponseIncomplete()
			terminalErr = validateCompletedResponse(&incomplete.Response)
		case "error":
			responseError := event.AsError()
			terminalErr = responseStreamError(responseError)
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if errProcess := processEvent(eventData); errProcess != nil {
				return nil, errProcess
			}
			eventData = eventData[:0]
			if streamDone {
				break
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, hasSeparator := strings.Cut(line, ":")
		if !hasSeparator || field != "data" {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		separatorSize := 0
		if len(eventData) > 0 {
			separatorSize = 1
		}
		if len(value) > maxStreamEventSize-len(eventData)-separatorSize {
			return nil, errors.NewAPIError("Responses stream event exceeds size limit", nil)
		}
		if separatorSize > 0 {
			eventData = append(eventData, '\n')
		}
		eventData = append(eventData, value...)
	}
	if errScan := scanner.Err(); errScan != nil {
		return nil, errors.NewAPIError("failed to read Responses stream", errScan)
	}
	if !streamDone && len(eventData) > 0 {
		if errProcess := processEvent(eventData); errProcess != nil {
			return nil, errProcess
		}
	}
	if terminalErr != nil {
		return nil, terminalErr
	}
	if completedResponse == nil {
		return nil, errors.NewAPIError("Responses stream ended without a completed response", nil)
	}
	if errValidate := validateCompletedResponse(completedResponse); errValidate != nil {
		return nil, errValidate
	}
	return completedResponse, nil
}

func validateCompletedResponse(response *responses.Response) error {
	if response == nil {
		return errors.NewAPIError("Responses API returned an empty response", nil)
	}
	if response.Status == responses.ResponseStatusCompleted {
		return nil
	}
	if response.Error.Message != "" {
		if response.Error.Code != "" {
			return errors.NewAPIError(fmt.Sprintf("Responses API returned status %s (%s): %s", response.Status, response.Error.Code, response.Error.Message), nil)
		}
		return errors.NewAPIError(fmt.Sprintf("Responses API returned status %s: %s", response.Status, response.Error.Message), nil)
	}
	if response.IncompleteDetails.Reason != "" {
		return errors.NewAPIError(fmt.Sprintf("Responses API returned status %s: %s", response.Status, response.IncompleteDetails.Reason), nil)
	}
	return errors.NewAPIError(fmt.Sprintf("Responses API returned status %s", response.Status), nil)
}

func responseStreamError(responseError responses.ResponseErrorEvent) error {
	details := responseError.Message
	if responseError.Code != "" {
		details = fmt.Sprintf("%s: %s", responseError.Code, details)
	}
	if responseError.Param != "" {
		details = fmt.Sprintf("%s (parameter: %s)", details, responseError.Param)
	}
	return errors.NewAPIError("Responses stream error: "+details, nil)
}

func supportsResponsesReasoning(modelName string) bool {
	normalizedModel := normalizeResponsesModelName(modelName)
	if strings.HasPrefix(normalizedModel, "gpt-5") {
		return !strings.Contains(normalizedModel, "-chat")
	}
	return len(normalizedModel) > 1 && normalizedModel[0] == 'o' && normalizedModel[1] >= '0' && normalizedModel[1] <= '9'
}

func responsesReasoningEffort(modelName string, thinkingLevel string) responses.ReasoningEffort {
	normalizedModel := normalizeResponsesModelName(modelName)
	if strings.Contains(normalizedModel, "-pro") {
		return responses.ReasoningEffortHigh
	}

	normalizedLevel := strings.ToLower(strings.TrimSpace(thinkingLevel))
	if normalizedLevel == "minimal" && !strings.HasPrefix(normalizedModel, "gpt-5") {
		normalizedLevel = "low"
	}
	return responses.ReasoningEffort(normalizedLevel)
}

func normalizeResponsesModelName(modelName string) string {
	normalizedModel := strings.ToLower(strings.TrimSpace(modelName))
	separatorIndex := strings.LastIndex(normalizedModel, "/")
	if separatorIndex >= 0 {
		normalizedModel = normalizedModel[separatorIndex+1:]
	}
	return normalizedModel
}

func responseInputFromContext(contextMessages []ContextMessage) (responses.ResponseInputParam, error) {
	input := make(responses.ResponseInputParam, 0, len(contextMessages))
	for contextIndex, message := range contextMessages {
		if len(message.RawItem) > 0 {
			if isEmptyRawResponsesMessage(message.RawItem) {
				continue
			}
			item, errUnmarshal := unmarshalResponseInputItem(message.RawItem)
			if errUnmarshal != nil {
				return nil, fmt.Errorf("invalid raw Responses context item %d (%s): %w", contextIndex, message.RawItem, errUnmarshal)
			}
			input = append(input, item)
			continue
		}

		role := responses.EasyInputMessageRoleUser
		switch message.Role {
		case "assistant", "model":
			role = responses.EasyInputMessageRoleAssistant
		case "system":
			role = responses.EasyInputMessageRoleSystem
		case "developer":
			role = responses.EasyInputMessageRoleDeveloper
		}
		if role == responses.EasyInputMessageRoleAssistant && strings.TrimSpace(message.Content) == "" {
			continue
		}
		input = append(input, responses.ResponseInputItemParamOfMessage(message.Content, role))
	}
	return input, nil
}

func appendResponseOutput(input responses.ResponseInputParam, contextMessages *[]ContextMessage, output []responses.ResponseOutputItemUnion) (responses.ResponseInputParam, error) {
	for _, outputItem := range output {
		if isEmptyResponseOutputMessage(outputItem) {
			continue
		}
		rawItem := json.RawMessage(outputItem.RawJSON())
		if len(rawItem) == 0 {
			marshaledItem, errMarshal := json.Marshal(outputItem)
			if errMarshal != nil {
				return nil, fmt.Errorf("failed to marshal Responses output item: %w", errMarshal)
			}
			rawItem = marshaledItem
		}
		if isEmptyRawResponsesMessage(rawItem) {
			continue
		}

		inputItem, errUnmarshal := unmarshalResponseInputItem(rawItem)
		if errUnmarshal != nil {
			return nil, fmt.Errorf("failed to convert Responses output item to input: %w", errUnmarshal)
		}
		input = append(input, inputItem)
		*contextMessages = append(*contextMessages, ContextMessage{RawItem: append(json.RawMessage(nil), rawItem...)})
	}
	return input, nil
}

func unmarshalResponseInputItem(raw []byte) (responses.ResponseInputItemUnionParam, error) {
	var outMsg responses.ResponseOutputMessageParam
	if errOut := json.Unmarshal(raw, &outMsg); errOut == nil && outMsg.Role == "assistant" && len(outMsg.Content) > 0 {
		return responses.ResponseInputItemUnionParam{
			OfOutputMessage: &outMsg,
		}, nil
	}

	var item responses.ResponseInputItemUnionParam
	if errUnmarshal := json.Unmarshal(raw, &item); errUnmarshal != nil {
		return responses.ResponseInputItemUnionParam{}, errUnmarshal
	}
	return item, nil
}

func sanitizeResponsesInput(input responses.ResponseInputParam) responses.ResponseInputParam {
	sanitized := make(responses.ResponseInputParam, 0, len(input))
	for _, item := range input {
		data, errMarshal := json.Marshal(item)
		if errMarshal != nil {
			continue
		}
		var probe struct {
			Role    string          `json:"role"`
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		if errProbe := json.Unmarshal(data, &probe); errProbe == nil {
			isAssistant := probe.Role == "assistant" || probe.Role == "model"
			if isAssistant && (len(probe.Content) == 0 || string(probe.Content) == "null") {
				continue
			}
		}
		sanitized = append(sanitized, item)
	}
	return sanitized
}

func isEmptyResponseOutputMessage(item responses.ResponseOutputItemUnion) bool {
	if item.Type != "message" {
		return false
	}
	if item.Role != "" && item.Role != "assistant" {
		return false
	}
	if len(item.Content) == 0 {
		return true
	}
	for _, content := range item.Content {
		if content.Type != "" && content.Type != "output_text" && content.Type != "refusal" {
			return false
		}
		if strings.TrimSpace(content.Text) != "" || strings.TrimSpace(content.Refusal) != "" {
			return false
		}
	}
	return true
}

func isEmptyRawResponsesMessage(raw []byte) bool {
	var probe struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if errUnmarshal := json.Unmarshal(raw, &probe); errUnmarshal != nil {
		return false
	}
	isAssistant := probe.Role == "assistant" || probe.Role == "model"
	if !isAssistant && probe.Role != "" {
		return false
	}
	if probe.Type != "" && probe.Type != "message" {
		return false
	}
	trimmed := strings.TrimSpace(string(probe.Content))
	if len(probe.Content) == 0 || trimmed == "" || trimmed == "null" {
		return true
	}
	var textContent string
	if errStr := json.Unmarshal(probe.Content, &textContent); errStr == nil {
		return strings.TrimSpace(textContent) == ""
	}
	var arrayContent []map[string]any
	if errArr := json.Unmarshal(probe.Content, &arrayContent); errArr == nil {
		if len(arrayContent) == 0 {
			return true
		}
		for _, block := range arrayContent {
			if blockType, ok := block["type"].(string); ok && blockType != "output_text" && blockType != "text" && blockType != "refusal" {
				return false
			}
			if textVal, ok := block["text"].(string); ok && strings.TrimSpace(textVal) != "" {
				return false
			}
			if refusalVal, ok := block["refusal"].(string); ok && strings.TrimSpace(refusalVal) != "" {
				return false
			}
		}
		return true
	}
	return false
}

func appendResponseInputContext(contextMessages *[]ContextMessage, inputItem responses.ResponseInputItemUnionParam) error {
	rawItem, err := json.Marshal(inputItem)
	if err != nil {
		return err
	}
	*contextMessages = append(*contextMessages, ContextMessage{RawItem: rawItem})
	return nil
}

func requiredFunctionCall(output []responses.ResponseOutputItemUnion, expectedName string) (*responses.ResponseFunctionToolCall, error) {
	var found *responses.ResponseFunctionToolCall
	for _, item := range output {
		if item.Type != "function_call" {
			continue
		}
		call := item.AsFunctionCall()
		if call.Name != expectedName {
			return nil, fmt.Errorf("unexpected function call %q", call.Name)
		}
		if found != nil {
			return nil, fmt.Errorf("function %q was called more than once", expectedName)
		}
		callCopy := call
		found = &callCopy
	}
	if found == nil {
		return nil, fmt.Errorf("required function %q was not called", expectedName)
	}
	return found, nil
}

func validateReadToolArguments(arguments string) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &values); err != nil {
		return err
	}
	if len(values) != 0 {
		return fmt.Errorf("read_translation_batch does not accept arguments")
	}
	return nil
}

func readTranslationBatchDefinition() responses.ToolUnionParam {
	tool := responses.ToolParamOfFunction(readTranslationBatchTool, map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	}, true)
	tool.OfFunction.Description = openai.String("Read the current batch of subtitle records that must be translated.")
	return tool
}

func submitTranslationsDefinition() responses.ToolUnionParam {
	recordSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"index":   map[string]any{"type": "integer"},
			"content": map[string]any{"type": "string"},
			"guard":   map[string]any{"type": "string"},
		},
		"required":             []string{"index", "content", "guard"},
		"additionalProperties": false,
	}
	tool := responses.ToolParamOfFunction(submitTranslationsTool, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"records": map[string]any{
				"type":  "array",
				"items": recordSchema,
			},
		},
		"required":             []string{"records"},
		"additionalProperties": false,
	}, true)
	tool.OfFunction.Description = openai.String("Submit every translated subtitle record in the original order.")
	return tool
}

func (o *OpenAIProvider) getResponsesInstruction(translationConfig *TranslationConfig) string {
	instruction := fmt.Sprintf(`You are an assistant that translates subtitles from any language to %s.

For every batch, call read_translation_batch to obtain the records. After translating all records, call submit_translations exactly once with the complete result. Never return translations as assistant text.

Each record contains:
- index: an integer translation index
- content: the text to translate
- guard: a line guard token that must be copied unchanged

Translate only the content field. Copy index and guard exactly as received.
If content is empty, leave it unchanged.
Preserve the original meaning, formatting intent, and special characters, but do not emit literal line breaks in content.
Treat each input record as one complete subtitle, including content that appears to contain escaped line separators or multiple visual lines.
Do not move, merge, or split content between records.
Do not add, remove, or reorder records.

If the target language is Simplified Chinese:
Replace every comma, period, exclamation mark, and question mark with four spaces.
Replace every line break, carriage return, and escaped line separator with four spaces.
Trim invisible characters at the beginning and end of content.
Remove tags such as <i></i> while preserving their inner content.
Remove invisible characters after ":" or "：".`, translationConfig.TargetLanguage)

	if translationConfig.Description != "" {
		instruction += fmt.Sprintf("\n\nAdditional user instruction:\n\n%s", translationConfig.Description)
	}
	if translationConfig.RetryInstruction != "" {
		instruction += "\n\nRetry correction instruction:\n\n" + translationConfig.RetryInstruction
		instruction += "\n\nFor this Responses request, any instruction above about returning a JSON array or text means supplying the same structured records through submit_translations. The protocol requirement to return no translation text and use submit_translations takes precedence."
	}
	return instruction
}
