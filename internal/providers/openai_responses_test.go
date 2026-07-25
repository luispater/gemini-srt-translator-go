package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/luispater/gemini-srt-translator-go/pkg/config"
	"github.com/luispater/gemini-srt-translator-go/pkg/srt"
)

func TestOpenAIResponsesTranslateBatchUsesToolsAndPreservesReasoningContext(t *testing.T) {
	translatedRecords := []srt.SubtitleObject{
		{Index: 10, Content: "第一句", Guard: "GST_LINE_000010"},
		{Index: 11, Content: "第二句", Guard: "GST_LINE_000011"},
	}
	submitArguments, err := json.Marshal(submitTranslationsArguments{Records: translatedRecords})
	if err != nil {
		t.Fatalf("failed to marshal submission arguments: %v", err)
	}

	responseBodies := []string{
		responsesTestBody(`[
			{"type":"reasoning","id":"rs_read","summary":[{"type":"summary_text","text":"I should read the batch."}],"encrypted_content":"signature-read","status":"completed"},
			{"type":"function_call","id":"fc_read","call_id":"call_read","name":"read_translation_batch","arguments":"{}","status":"completed"}
		]`),
		responsesTestBody(fmt.Sprintf(`[
			{"type":"reasoning","id":"rs_submit","summary":[{"type":"summary_text","text":"I translated every record."}],"encrypted_content":"signature-submit","status":"completed"},
			{"type":"function_call","id":"fc_submit","call_id":"call_submit","name":"submit_translations","arguments":%q,"status":"completed"}
		]`, submitArguments)),
	}

	var requestBodies [][]byte
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		body, errReadAll := io.ReadAll(request.Body)
		if errReadAll != nil {
			http.Error(responseWriter, errReadAll.Error(), http.StatusInternalServerError)
			return
		}

		mutex.Lock()
		requestIndex := len(requestBodies)
		requestBodies = append(requestBodies, body)
		mutex.Unlock()

		if request.URL.Path != "/v1/responses" {
			http.Error(responseWriter, "unexpected path", http.StatusNotFound)
			return
		}
		if requestIndex >= len(responseBodies) {
			http.Error(responseWriter, "unexpected request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(responseBodies[requestIndex]))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(&config.Config{
		Provider:       "openai",
		OpenAIProtocol: "responses",
		APIKeys:        []string{"test-key"},
		BaseURL:        server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	batch := []srt.SubtitleObject{
		{Index: 10, Content: "First source", Guard: "GST_LINE_000010"},
		{Index: 11, Content: "Second source", Guard: "GST_LINE_000011"},
	}
	result, err := provider.TranslateBatch(context.Background(), batch, nil, &TranslationConfig{
		ModelName:      "gpt-test",
		TargetLanguage: "Simplified Chinese",
		Streaming:      false,
		Thinking:       true,
		ThinkingLevel:  "high",
	})
	if err != nil {
		t.Fatalf("TranslateBatch() error = %v", err)
	}
	if len(result.TranslatedBatch) != len(translatedRecords) {
		t.Fatalf("translated record count = %d, want %d", len(result.TranslatedBatch), len(translatedRecords))
	}
	for i, expected := range translatedRecords {
		if result.TranslatedBatch[i] != expected {
			t.Errorf("translated record %d = %+v, want %+v", i, result.TranslatedBatch[i], expected)
		}
	}

	mutex.Lock()
	capturedBodies := append([][]byte(nil), requestBodies...)
	mutex.Unlock()
	if len(capturedBodies) != 2 {
		t.Fatalf("request count = %d, want 2", len(capturedBodies))
	}
	var promptCacheKeys []string
	for requestIndex, requestBody := range capturedBodies {
		var requestData struct {
			PromptCacheKey string `json:"prompt_cache_key"`
		}
		if errUnmarshal := json.Unmarshal(requestBody, &requestData); errUnmarshal != nil {
			t.Fatalf("failed to decode request %d: %v", requestIndex+1, errUnmarshal)
		}
		if !isUUIDv4(requestData.PromptCacheKey) {
			t.Errorf("request %d prompt_cache_key = %q, want UUID v4", requestIndex+1, requestData.PromptCacheKey)
		}
		promptCacheKeys = append(promptCacheKeys, requestData.PromptCacheKey)
	}
	if promptCacheKeys[0] != promptCacheKeys[1] {
		t.Errorf("tool requests used different prompt cache keys: %q and %q", promptCacheKeys[0], promptCacheKeys[1])
	}
	if strings.Contains(string(capturedBodies[0]), "First source") {
		t.Error("first Responses request contained subtitle records before the read tool call")
	}
	if !strings.Contains(string(capturedBodies[0]), `"name":"read_translation_batch"`) {
		t.Error("first Responses request did not force read_translation_batch")
	}
	if !strings.Contains(string(capturedBodies[0]), `"reasoning.encrypted_content"`) {
		t.Error("Responses request did not include encrypted reasoning content")
	}
	if !strings.Contains(string(capturedBodies[0]), `"store":false`) {
		t.Error("Responses request did not use stateless context management")
	}
	if !strings.Contains(string(capturedBodies[1]), "First source") {
		t.Error("second Responses request did not receive subtitle records through function_call_output")
	}
	if !strings.Contains(string(capturedBodies[1]), `"encrypted_content":"signature-read"`) {
		t.Error("second Responses request did not preserve the first reasoning signature")
	}
	if !strings.Contains(string(capturedBodies[1]), `"name":"submit_translations"`) {
		t.Error("second Responses request did not force submit_translations")
	}

	contextJSON, err := json.Marshal(result.Context)
	if err != nil {
		t.Fatalf("failed to marshal returned context: %v", err)
	}
	for _, expectedContext := range []string{
		`"type":"reasoning"`,
		`"summary_text"`,
		`"encrypted_content":"signature-read"`,
		`"encrypted_content":"signature-submit"`,
		`"type":"function_call_output"`,
		`"call_id":"call_submit"`,
		`{\"accepted\":true}`,
	} {
		if !strings.Contains(string(contextJSON), expectedContext) {
			t.Errorf("returned context does not contain %s", expectedContext)
		}
	}

	restoredInput, err := responseInputFromContext(result.Context)
	if err != nil {
		t.Fatalf("responseInputFromContext() error = %v", err)
	}
	restoredJSON, err := json.Marshal(restoredInput)
	if err != nil {
		t.Fatalf("failed to marshal restored context: %v", err)
	}
	if !strings.Contains(string(restoredJSON), `"encrypted_content":"signature-submit"`) {
		t.Error("restored context lost the final reasoning signature")
	}
}

func TestOpenAIPromptCacheKeyIsStableForProviderLifetime(t *testing.T) {
	firstConfig := &config.Config{APIKeys: []string{"first-key", "second-key"}}
	firstProvider, errNewFirst := NewOpenAIProvider(firstConfig)
	if errNewFirst != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", errNewFirst)
	}
	firstKey, errFirstKey := firstProvider.getOrCreatePromptCacheKey()
	if errFirstKey != nil {
		t.Fatalf("getOrCreatePromptCacheKey() error = %v", errFirstKey)
	}
	secondRead, errSecondRead := firstProvider.getOrCreatePromptCacheKey()
	if errSecondRead != nil {
		t.Fatalf("second getOrCreatePromptCacheKey() error = %v", errSecondRead)
	}
	if firstKey != secondRead || firstConfig.OpenAIPromptCacheKey != firstKey {
		t.Errorf("provider did not retain prompt cache key: first=%q second=%q config=%q", firstKey, secondRead, firstConfig.OpenAIPromptCacheKey)
	}
	if !isUUIDv4(firstKey) {
		t.Errorf("prompt cache key = %q, want UUID v4", firstKey)
	}
	if errCreateFirst := firstProvider.createClient(); errCreateFirst != nil {
		t.Fatalf("createClient() error = %v", errCreateFirst)
	}
	if !firstProvider.SwitchAPIKey() {
		t.Fatal("SwitchAPIKey() = false, want true")
	}
	if errCreateSecond := firstProvider.createClient(); errCreateSecond != nil {
		t.Fatalf("createClient() after key switch error = %v", errCreateSecond)
	}
	keyAfterClientRebuild, errKeyAfterClientRebuild := firstProvider.getOrCreatePromptCacheKey()
	if errKeyAfterClientRebuild != nil {
		t.Fatalf("getOrCreatePromptCacheKey() after client rebuild error = %v", errKeyAfterClientRebuild)
	}
	if keyAfterClientRebuild != firstKey {
		t.Errorf("client rebuild changed prompt cache key from %q to %q", firstKey, keyAfterClientRebuild)
	}

	secondProvider, errNewSecond := NewOpenAIProvider(&config.Config{})
	if errNewSecond != nil {
		t.Fatalf("second NewOpenAIProvider() error = %v", errNewSecond)
	}
	secondKey, errSecondKey := secondProvider.getOrCreatePromptCacheKey()
	if errSecondKey != nil {
		t.Fatalf("second provider getOrCreatePromptCacheKey() error = %v", errSecondKey)
	}
	if secondKey == firstKey {
		t.Errorf("different provider tasks reused prompt cache key %q", firstKey)
	}

	restoredKey := "123e4567-e89b-42d3-a456-426614174000"
	restoredProvider, errNewRestored := NewOpenAIProvider(&config.Config{OpenAIPromptCacheKey: restoredKey})
	if errNewRestored != nil {
		t.Fatalf("restored NewOpenAIProvider() error = %v", errNewRestored)
	}
	actualRestoredKey, errRestoredKey := restoredProvider.getOrCreatePromptCacheKey()
	if errRestoredKey != nil {
		t.Fatalf("restored getOrCreatePromptCacheKey() error = %v", errRestoredKey)
	}
	if actualRestoredKey != restoredKey {
		t.Errorf("restored prompt cache key = %q, want %q", actualRestoredKey, restoredKey)
	}
}

func TestOpenAIPromptCacheKeyConcurrentInitializationUsesOneUUID(t *testing.T) {
	const workerCount = 64

	providerConfig := &config.Config{}
	provider, errNewProvider := NewOpenAIProvider(providerConfig)
	if errNewProvider != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", errNewProvider)
	}

	start := make(chan struct{})
	keys := make([]string, workerCount)
	errorsByWorker := make([]error, workerCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)
	for workerIndex := range workerCount {
		go func(index int) {
			defer waitGroup.Done()
			<-start
			keys[index], errorsByWorker[index] = provider.getOrCreatePromptCacheKey()
		}(workerIndex)
	}
	close(start)
	waitGroup.Wait()

	firstKey := keys[0]
	if !isUUIDv4(firstKey) {
		t.Fatalf("prompt cache key = %q, want UUID v4", firstKey)
	}
	for workerIndex := range workerCount {
		if errorsByWorker[workerIndex] != nil {
			t.Errorf("worker %d error = %v", workerIndex, errorsByWorker[workerIndex])
		}
		if keys[workerIndex] != firstKey {
			t.Errorf("worker %d key = %q, want %q", workerIndex, keys[workerIndex], firstKey)
		}
	}
	if providerConfig.OpenAIPromptCacheKey != firstKey {
		t.Errorf("config prompt cache key = %q, want %q", providerConfig.OpenAIPromptCacheKey, firstKey)
	}
}

func TestOpenAIResponsesTranslateBatchRejectsMissingSubmissionCall(t *testing.T) {
	responsesServed := 0
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responsesServed++
		responseWriter.Header().Set("Content-Type", "application/json")
		if responsesServed == 1 {
			_, _ = responseWriter.Write([]byte(responsesTestBody(`[
				{"type":"function_call","id":"fc_read","call_id":"call_read","name":"read_translation_batch","arguments":"{}","status":"completed"}
			]`)))
			return
		}
		_, _ = responseWriter.Write([]byte(responsesTestBody(`[
			{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"not a tool call","annotations":[]}]}
		]`)))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(&config.Config{
		OpenAIProtocol: "responses",
		APIKeys:        []string{"test-key"},
		BaseURL:        server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	_, err = provider.TranslateBatch(context.Background(), []srt.SubtitleObject{
		{Index: 1, Content: "source", Guard: "GST_LINE_000001"},
	}, nil, &TranslationConfig{
		ModelName:      "gpt-test",
		TargetLanguage: "French",
		Streaming:      false,
	})
	if err == nil || !strings.Contains(err.Error(), "submit_translations") {
		t.Fatalf("TranslateBatch() error = %v, want missing submit_translations error", err)
	}
}

func TestOpenAIResponsesStreamingReturnsCompletedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept header = %q, want text/event-stream", request.Header.Get("Accept"))
		}
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		responseBody := responsesTestBody(`[
			{"type":"function_call","id":"fc_read","call_id":"call_read","name":"read_translation_batch","arguments":"{}","status":"completed"}
		]`)
		compactResponse := strings.NewReplacer("\n", "", "\t", "").Replace(responseBody)
		_, _ = fmt.Fprintf(responseWriter, ": keep-alive\n\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":1,\"response\":%s}\n\n\n", compactResponse)
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(&config.Config{
		OpenAIProtocol: "responses",
		APIKeys:        []string{"test-key"},
		BaseURL:        server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}
	if err = provider.createClient(); err != nil {
		t.Fatalf("createClient() error = %v", err)
	}

	response, err := provider.sendResponsesRequest(
		context.Background(),
		nil,
		&TranslationConfig{ModelName: "gpt-test", Streaming: true},
		"Call the tool.",
		readTranslationBatchDefinition(),
		readTranslationBatchTool,
	)
	if err != nil {
		t.Fatalf("sendResponsesRequest() error = %v", err)
	}
	call, err := requiredFunctionCall(response.Output, readTranslationBatchTool)
	if err != nil {
		t.Fatalf("requiredFunctionCall() error = %v", err)
	}
	if call.CallID != "call_read" {
		t.Errorf("call ID = %q, want call_read", call.CallID)
	}
}

func TestConsumeResponsesStreamRejectsOversizedMultilineEvent(t *testing.T) {
	streamData := "data: 1234567890123456\ndata: 1234567890123456\n\n"

	_, err := consumeResponsesStreamWithLimit(strings.NewReader(streamData), 24)
	if err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("consumeResponsesStreamWithLimit() error = %v, want size limit error", err)
	}
}

func TestOpenAIResponsesStreamingPreservesTerminalErrors(t *testing.T) {
	tests := []struct {
		name          string
		event         string
		expectedError string
	}{
		{
			name:          "failed response",
			event:         `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_failed","object":"response","created_at":1,"model":"gpt-5","status":"failed","error":{"code":"rate_limit_exceeded","message":"quota exhausted"},"output":[]}}`,
			expectedError: "rate_limit_exceeded",
		},
		{
			name:          "incomplete response",
			event:         `{"type":"response.incomplete","sequence_number":1,"response":{"id":"resp_incomplete","object":"response","created_at":1,"model":"gpt-5","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}}`,
			expectedError: "max_output_tokens",
		},
		{
			name:          "error event",
			event:         `{"type":"error","sequence_number":1,"code":"invalid_request","message":"bad tool choice","param":"tool_choice"}`,
			expectedError: "bad tool choice",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				responseWriter.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(responseWriter, "data: %s\n\ndata: [DONE]\n\n", testCase.event)
			}))
			defer server.Close()

			provider, errNewProvider := NewOpenAIProvider(&config.Config{
				OpenAIProtocol: "responses",
				APIKeys:        []string{"test-key"},
				BaseURL:        server.URL + "/v1",
			})
			if errNewProvider != nil {
				t.Fatalf("NewOpenAIProvider() error = %v", errNewProvider)
			}
			if errCreateClient := provider.createClient(); errCreateClient != nil {
				t.Fatalf("createClient() error = %v", errCreateClient)
			}

			_, errRequest := provider.sendResponsesRequest(
				context.Background(),
				nil,
				&TranslationConfig{ModelName: "gpt-5", Streaming: true},
				"Call the tool.",
				readTranslationBatchDefinition(),
				readTranslationBatchTool,
			)
			if errRequest == nil || !strings.Contains(errRequest.Error(), testCase.expectedError) {
				t.Fatalf("sendResponsesRequest() error = %v, want error containing %q", errRequest, testCase.expectedError)
			}
		})
	}
}

func TestOpenAIResponsesRequestParameterCompatibility(t *testing.T) {
	temperature := float32(0.7)
	topP := float32(0.8)
	tests := []struct {
		name            string
		modelName       string
		thinking        bool
		thinkingLevel   string
		wantReasoning   bool
		wantEffort      string
		wantSummary     bool
		wantTemperature bool
		wantTopP        bool
	}{
		{
			name:          "gpt-5 keeps minimal reasoning",
			modelName:     "gpt-5-mini",
			thinking:      true,
			thinkingLevel: "minimal",
			wantReasoning: true,
			wantEffort:    "minimal",
			wantSummary:   true,
		},
		{
			name:          "o-series normalizes minimal reasoning",
			modelName:     "openai/o3-mini",
			thinking:      true,
			thinkingLevel: "minimal",
			wantReasoning: true,
			wantEffort:    "low",
			wantSummary:   true,
		},
		{
			name:            "non-reasoning model keeps sampling parameters",
			modelName:       "gpt-4.1",
			thinking:        true,
			thinkingLevel:   "high",
			wantTemperature: true,
			wantTopP:        true,
		},
		{
			name:            "GPT-5 chat variant keeps sampling parameters",
			modelName:       "openai/gpt-5-chat-latest",
			thinking:        true,
			thinkingLevel:   "high",
			wantTemperature: true,
			wantTopP:        true,
		},
		{
			name:          "GPT-5 pro only uses high reasoning",
			modelName:     "gpt-5-pro",
			thinking:      true,
			thinkingLevel: "minimal",
			wantReasoning: true,
			wantEffort:    "high",
			wantSummary:   true,
		},
		{
			name:          "disabled GPT-5 thinking uses API minimum without summary",
			modelName:     "gpt-5",
			thinking:      false,
			thinkingLevel: "high",
			wantReasoning: true,
			wantEffort:    "minimal",
		},
		{
			name:          "disabled o-series thinking uses API minimum without summary",
			modelName:     "o3-mini",
			thinking:      false,
			thinkingLevel: "high",
			wantReasoning: true,
			wantEffort:    "low",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var requestBody map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				body, errReadAll := io.ReadAll(request.Body)
				if errReadAll != nil {
					http.Error(responseWriter, errReadAll.Error(), http.StatusInternalServerError)
					return
				}
				if errUnmarshal := json.Unmarshal(body, &requestBody); errUnmarshal != nil {
					http.Error(responseWriter, errUnmarshal.Error(), http.StatusBadRequest)
					return
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				_, _ = responseWriter.Write([]byte(responsesTestBody(`[]`)))
			}))
			defer server.Close()

			provider, errNewProvider := NewOpenAIProvider(&config.Config{
				OpenAIProtocol: "responses",
				APIKeys:        []string{"test-key"},
				BaseURL:        server.URL + "/v1",
			})
			if errNewProvider != nil {
				t.Fatalf("NewOpenAIProvider() error = %v", errNewProvider)
			}
			if errCreateClient := provider.createClient(); errCreateClient != nil {
				t.Fatalf("createClient() error = %v", errCreateClient)
			}

			translationConfig := &TranslationConfig{
				ModelName:     testCase.modelName,
				Thinking:      testCase.thinking,
				ThinkingLevel: testCase.thinkingLevel,
				Temperature:   &temperature,
				TopP:          &topP,
			}
			_, errRequest := provider.sendResponsesRequest(
				context.Background(),
				nil,
				translationConfig,
				"Call the tool.",
				readTranslationBatchDefinition(),
				readTranslationBatchTool,
			)
			if errRequest != nil {
				t.Fatalf("sendResponsesRequest() error = %v", errRequest)
			}

			reasoningJSON, hasReasoning := requestBody["reasoning"]
			if hasReasoning != testCase.wantReasoning {
				t.Errorf("reasoning parameter present = %v, want %v; body = %v", hasReasoning, testCase.wantReasoning, requestBody)
			}
			if hasReasoning {
				var reasoning map[string]string
				if errUnmarshal := json.Unmarshal(reasoningJSON, &reasoning); errUnmarshal != nil {
					t.Fatalf("failed to decode reasoning parameters: %v", errUnmarshal)
				}
				if reasoning["effort"] != testCase.wantEffort {
					t.Errorf("reasoning effort = %q, want %q", reasoning["effort"], testCase.wantEffort)
				}
				_, hasSummary := reasoning["summary"]
				if hasSummary != testCase.wantSummary {
					t.Errorf("reasoning summary present = %v, want %v", hasSummary, testCase.wantSummary)
				}
			}
			if !testCase.thinking {
				encodedRequestBody, errMarshal := json.Marshal(requestBody)
				if errMarshal != nil {
					t.Fatalf("failed to encode captured request body: %v", errMarshal)
				}
				if strings.Contains(string(encodedRequestBody), "summary") {
					t.Error("reasoning summary was requested while thinking was disabled")
				}
			}

			_, hasTemperature := requestBody["temperature"]
			if hasTemperature != testCase.wantTemperature {
				t.Errorf("temperature present = %v, want %v", hasTemperature, testCase.wantTemperature)
			}
			_, hasTopP := requestBody["top_p"]
			if hasTopP != testCase.wantTopP {
				t.Errorf("top_p present = %v, want %v", hasTopP, testCase.wantTopP)
			}
			if !strings.Contains(string(requestBody["include"]), "reasoning.encrypted_content") {
				t.Error("Responses request did not include encrypted reasoning content")
			}
		})
	}
}

func TestSupportsResponsesReasoningExcludesGPT5ChatVariants(t *testing.T) {
	tests := []struct {
		modelName string
		want      bool
	}{
		{modelName: "gpt-5", want: true},
		{modelName: "openai/gpt-5-mini", want: true},
		{modelName: "gpt-5-chat-latest", want: false},
		{modelName: "openai/gpt-5.1-chat", want: false},
		{modelName: "o3-mini", want: true},
		{modelName: "gpt-4.1", want: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.modelName, func(t *testing.T) {
			if got := supportsResponsesReasoning(testCase.modelName); got != testCase.want {
				t.Errorf("supportsResponsesReasoning(%q) = %v, want %v", testCase.modelName, got, testCase.want)
			}
		})
	}
}

func TestResponsesReasoningEffortUsesModelSupportedMinimums(t *testing.T) {
	// Thinking cannot be fully disabled through the Responses API, so minimal requests must use each model's lowest supported effort.
	tests := []struct {
		modelName     string
		thinkingLevel string
		want          string
	}{
		{modelName: "gpt-5-mini", thinkingLevel: "minimal", want: "minimal"},
		{modelName: "o3-mini", thinkingLevel: "minimal", want: "low"},
		{modelName: "gpt-5-pro", thinkingLevel: "low", want: "high"},
		{modelName: "openai/o3-pro", thinkingLevel: "medium", want: "high"},
	}

	for _, testCase := range tests {
		t.Run(testCase.modelName, func(t *testing.T) {
			got := responsesReasoningEffort(testCase.modelName, testCase.thinkingLevel)
			if string(got) != testCase.want {
				t.Errorf("responsesReasoningEffort(%q, %q) = %q, want %q", testCase.modelName, testCase.thinkingLevel, got, testCase.want)
			}
		})
	}
}

func TestOpenAISwitchAPIKeyRebuildsClient(t *testing.T) {
	var authorizationHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		authorizationHeaders = append(authorizationHeaders, request.Header.Get("Authorization"))
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{
			"id":"chatcmpl_test",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-4o",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"[{\"index\":1,\"content\":\"traduit\",\"guard\":\"GST_LINE_000001\"}]"}}]
		}`))
	}))
	defer server.Close()

	provider, errNewProvider := NewOpenAIProvider(&config.Config{
		APIKeys: []string{"first-key", "second-key"},
		BaseURL: server.URL + "/v1",
	})
	if errNewProvider != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", errNewProvider)
	}
	batch := []srt.SubtitleObject{{Index: 1, Content: "source", Guard: "GST_LINE_000001"}}
	translationConfig := &TranslationConfig{ModelName: "gpt-4o", TargetLanguage: "French"}

	if _, errTranslate := provider.TranslateBatch(context.Background(), batch, nil, translationConfig); errTranslate != nil {
		t.Fatalf("first TranslateBatch() error = %v", errTranslate)
	}
	if !provider.SwitchAPIKey() {
		t.Fatal("SwitchAPIKey() = false, want true")
	}
	if provider.client != nil {
		t.Fatal("SwitchAPIKey() did not invalidate the existing client")
	}
	if _, errTranslate := provider.TranslateBatch(context.Background(), batch, nil, translationConfig); errTranslate != nil {
		t.Fatalf("second TranslateBatch() error = %v", errTranslate)
	}

	wantHeaders := []string{"Bearer first-key", "Bearer second-key"}
	if len(authorizationHeaders) != len(wantHeaders) {
		t.Fatalf("authorization header count = %d, want %d", len(authorizationHeaders), len(wantHeaders))
	}
	for headerIndex, wantHeader := range wantHeaders {
		if authorizationHeaders[headerIndex] != wantHeader {
			t.Errorf("authorization header %d = %q, want %q", headerIndex, authorizationHeaders[headerIndex], wantHeader)
		}
	}
}

func TestOpenAIChatCompletionsProtocolRemainsDefault(t *testing.T) {
	requestedPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.Path
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{
			"id":"chatcmpl_test",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-test",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"[{\"index\":1,\"content\":\"traduit\",\"guard\":\"GST_LINE_000001\"}]"}}]
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(&config.Config{
		APIKeys: []string{"test-key"},
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	result, err := provider.TranslateBatch(context.Background(), []srt.SubtitleObject{
		{Index: 1, Content: "source", Guard: "GST_LINE_000001"},
	}, nil, &TranslationConfig{
		ModelName:      "gpt-test",
		TargetLanguage: "French",
		Streaming:      false,
	})
	if err != nil {
		t.Fatalf("TranslateBatch() error = %v", err)
	}
	if requestedPath != "/v1/chat/completions" {
		t.Errorf("request path = %q, want Chat Completions path", requestedPath)
	}
	if len(result.TranslatedBatch) != 1 || result.TranslatedBatch[0].Content != "traduit" {
		t.Errorf("unexpected Chat Completions result: %+v", result.TranslatedBatch)
	}
}

func isUUIDv4(value string) bool {
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(value)
}

func responsesTestBody(output string) string {
	return fmt.Sprintf(`{
		"id":"resp_test",
		"object":"response",
		"created_at":1,
		"model":"gpt-test",
		"status":"completed",
		"output":%s
	}`, output)
}
