package proxyshared

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClaudeMessagesProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		input, ok := req.Input.([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("unexpected input: %#v", req.Input)
		}
		first, ok := input[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected first input item: %#v", input[0])
		}
		if first["type"] != "message" || first["role"] != "user" || first["content"] != "hi" {
			t.Fatalf("unexpected message mapping: %#v", first)
		}
		if req.Instructions != "you are helpful" {
			t.Fatalf("unexpected instructions: %q", req.Instructions)
		}

		writeJSON(w, http.StatusOK, openAIResponsesResponse{
			ID:        "resp_msg_1",
			Model:     "gpt-4.1-mini",
			CreatedAt: 1710000000,
			Status:    "completed",
			Output: []openAIResponseOutput{{
				Type: "message",
				Role: "assistant",
				Content: []openAIResponseContent{{
					Type: "output_text",
					Text: "hello there",
				}},
			}},
			Usage: &openAIUsage{
				InputTokens:  5,
				OutputTokens: 2,
				TotalTokens:  7,
			},
		})
	}))
	defer upstream.Close()

	srv := NewServer(Config{ResponsesURL: upstream.URL, Timeout: 5 * time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-compat",
		"system":"you are helpful",
		"messages":[{"role":"user","content":"hi"}],
		"max_tokens":64,
		"metadata":{"user_id":"u-123","trace":"keep-local-only"}
	}`))
	request.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp claudeMessagesResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Type != "message" || resp.Role != "assistant" {
		t.Fatalf("unexpected envelope: %#v", resp)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "hello there" {
		t.Fatalf("unexpected content: %#v", resp.Content)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("unexpected stop reason: %s", resp.StopReason)
	}
	if resp.Usage == nil || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestOpenAIChatCompletionsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		input, ok := req.Input.([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("unexpected input: %#v", req.Input)
		}
		msg, ok := input[0].(map[string]any)
		if !ok || msg["type"] != "message" || msg["role"] != "user" || msg["content"] != "say hi" {
			t.Fatalf("unexpected input message: %#v", input[0])
		}

		writeJSON(w, http.StatusOK, openAIResponsesResponse{
			ID:        "resp_cmp_1",
			Model:     "gpt-4.1-mini",
			CreatedAt: 1710000010,
			Status:    "completed",
			Output: []openAIResponseOutput{{
				Type: "message",
				Role: "assistant",
				Content: []openAIResponseContent{{
					Type: "output_text",
					Text: "hello STOP trailing",
				}},
			}},
		})
	}))
	defer upstream.Close()

	srv := NewServer(Config{ResponsesURL: upstream.URL, Timeout: 5 * time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"text-davinci-compat",
		"messages":[{"role":"user","content":"say hi"}],
		"max_tokens":16,
		"stop":" STOP"
	}`))
	request.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp openAIChatCompletionsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Fatalf("unexpected object: %s", resp.Object)
	}
	if resp.Choices[0].Message.Content != "hello" {
		t.Fatalf("unexpected text: %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("unexpected finish reason: %s", resp.Choices[0].FinishReason)
	}
}

func TestClaudeMessagesStreamingProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","id":"resp_stream_1","model":"gpt-4.1-mini"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hel"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"lo"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","id":"resp_stream_1","model":"gpt-4.1-mini","usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{ResponsesURL: upstream.URL, Timeout: 5 * time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-compat",
		"messages":[{"role":"user","content":"hi"}],
		"max_tokens":16,
		"stream":true
	}`))
	request.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if !strings.Contains(body, "event: message_start") {
		t.Fatalf("missing message_start: %s", body)
	}
	if !strings.Contains(body, `event: content_block_delta`) || !strings.Contains(body, `"text":"hel"`) || !strings.Contains(body, `"text":"lo"`) {
		t.Fatalf("missing deltas: %s", body)
	}
	if !strings.Contains(body, `event: message_delta`) || !strings.Contains(body, `"stop_reason":"end_turn"`) {
		t.Fatalf("missing message_delta: %s", body)
	}
	if !strings.Contains(body, `event: message_stop`) {
		t.Fatalf("missing message_stop: %s", body)
	}
}

func TestOpenAIChatCompletionsStreamingProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","id":"resp_stream_2","model":"gpt-4.1-mini","created_at":1710001000}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hello STOP"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","id":"resp_stream_2","model":"gpt-4.1-mini","created_at":1710001000}`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{ResponsesURL: upstream.URL, Timeout: 5 * time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"text-davinci-compat",
		"messages":[{"role":"user","content":"say hi"}],
		"stop":" STOP",
		"stream":true
	}`))
	request.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	events := readDataEvents(t, recorder.Body.String())
	if len(events) < 4 {
		t.Fatalf("unexpected stream body: %s", recorder.Body.String())
	}

	var chunk openAIChatCompletionChunkResponse
	if err := json.Unmarshal([]byte(events[0]), &chunk); err != nil {
		t.Fatalf("decode first chunk: %v", err)
	}
	if chunk.Choices[0].Delta.Role != "assistant" {
		t.Fatalf("unexpected first role chunk: %#v", chunk)
	}

	var contentChunk openAIChatCompletionChunkResponse
	if err := json.Unmarshal([]byte(events[1]), &contentChunk); err != nil {
		t.Fatalf("decode content chunk: %v", err)
	}
	if contentChunk.Choices[0].Delta.Content != "hello" {
		t.Fatalf("unexpected content chunk: %#v", contentChunk)
	}

	var finalChunk openAIChatCompletionChunkResponse
	if err := json.Unmarshal([]byte(events[2]), &finalChunk); err != nil {
		t.Fatalf("decode final chunk: %v", err)
	}
	if finalChunk.Choices[0].FinishReason != "stop" {
		t.Fatalf("unexpected finish reason: %#v", finalChunk)
	}
	if events[len(events)-1] != "[DONE]" {
		t.Fatalf("missing done marker: %#v", events)
	}
}

func TestOpenAIResponsesPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer passthrough-key" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("OpenAI-Beta") != "assistants=v2" {
			t.Fatalf("missing forwarded header: %q", r.Header.Get("OpenAI-Beta"))
		}

		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if raw["model"] != "gpt-5-codex" {
			t.Fatalf("unexpected model: %#v", raw["model"])
		}
		if raw["reasoning"] == nil {
			t.Fatalf("expected unknown field to survive passthrough: %#v", raw)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream-Test", "responses")
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "resp_passthrough_1",
			"object": "response",
			"status": "completed",
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": "ok",
				}},
			}},
		})
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ResponsesURL: upstream.URL + "/v1/responses",
		APIKey:       "passthrough-key",
		Timeout:      5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"gpt-5-codex",
		"input":[{"role":"user","content":"print(1)"}],
		"reasoning":{"effort":"high"},
		"stream":false
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("OpenAI-Beta", "assistants=v2")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Upstream-Test") != "responses" {
		t.Fatalf("missing proxied response header: %#v", recorder.Header())
	}

	var resp map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["id"] != "resp_passthrough_1" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestOpenAIResponsesStreamingPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("unexpected accept header: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","id":"resp_stream_passthrough","model":"gpt-5-codex"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hi"}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ResponsesURL: upstream.URL + "/v1/responses",
		Timeout:      5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"gpt-5-codex",
		"input":"hello",
		"stream":true
	}`))
	request.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("unexpected content type: %q", got)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `event: response.created`) || !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("unexpected stream body: %s", body)
	}
}

func TestOpenAIModelsPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer models-key" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id":     "gpt-5-codex",
				"object": "model",
			}},
		})
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ModelsURL: upstream.URL + "/v1/models",
		APIKey:    "models-key",
		Timeout:   5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if resp["object"] != "list" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func readDataEvents(t *testing.T, body string) []string {
	t.Helper()

	scanner := bufio.NewScanner(strings.NewReader(body))
	events := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan stream: %v", err)
	}
	return events
}
