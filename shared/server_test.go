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

func TestProxyRuntimeRequestLogging(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     "resp_log_1",
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

	logger := newLogHub(100)
	srv := newServerWithLogger(Config{
		ResponsesURL: upstream.URL,
		Timeout:      5 * time.Second,
	}, logger)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"gpt-5-codex",
		"input":"hello",
		"stream":false
	}`))
	request.Header.Set("Content-Type", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	logs := strings.Join(logger.Snapshot(), "\n")
	if !strings.Contains(logs, "[PROXY] request method=POST path=/v1/responses model=gpt-5-codex stream=false") {
		t.Fatalf("missing request log: %s", logs)
	}
	if !strings.Contains(logs, `"input":"hello"`) {
		t.Fatalf("missing request body in log: %s", logs)
	}
	if !strings.Contains(logs, "[PROXY] response path=") || !strings.Contains(logs, "upstream_status=200") {
		t.Fatalf("missing response log: %s", logs)
	}
}

func TestClaudeMessagesViaChatCompletionsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("unexpected accept header: %q", r.Header.Get("Accept"))
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["stream"] != true {
			t.Fatalf("expected stream=true: %#v", req)
		}
		messages, ok := req["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("unexpected messages: %#v", req["messages"])
		}
		first, ok := messages[0].(map[string]any)
		if !ok || first["role"] != "system" || first["content"] != "you are helpful" {
			t.Fatalf("unexpected system message: %#v", messages[0])
		}
		second, ok := messages[1].(map[string]any)
		if !ok || second["role"] != "user" || second["content"] != "hi" {
			t.Fatalf("unexpected user message: %#v", messages[1])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl_claude_1","object":"chat.completion.chunk","created":1710000600,"model":"claude-compat","choices":[{"index":0,"delta":{"role":"assistant","content":"hello there"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl_claude_1","object":"chat.completion.chunk","created":1710000600,"model":"claude-compat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":2,"prompt_tokens":5,"total_tokens":7}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ChatCompletionsURL: upstream.URL + "/v1/chat/completions",
		UpstreamProtocol:   upstreamProtocolChatCompletions,
		Timeout:            5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-compat",
		"system":"you are helpful",
		"messages":[{"role":"user","content":"hi"}],
		"max_tokens":64
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
}

func TestOpenAIChatCompletionsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("OpenAI-Beta") != "assistants=v2" {
			t.Fatalf("missing forwarded header: %q", r.Header.Get("OpenAI-Beta"))
		}

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
	request.Header.Set("OpenAI-Beta", "assistants=v2")

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

func TestClaudeMessagesStreamingViaChatCompletionsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("unexpected accept header: %q", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl_claude_stream_1","object":"chat.completion.chunk","created":1710000605,"model":"claude-compat","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":""}]}`,
			``,
			`data: {"id":"chatcmpl_claude_stream_1","object":"chat.completion.chunk","created":1710000605,"model":"claude-compat","choices":[{"index":0,"delta":{"content":"hel"},"finish_reason":""}]}`,
			``,
			`data: {"id":"chatcmpl_claude_stream_1","object":"chat.completion.chunk","created":1710000605,"model":"claude-compat","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":""}]}`,
			``,
			`data: {"id":"chatcmpl_claude_stream_1","object":"chat.completion.chunk","created":1710000605,"model":"claude-compat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":2,"prompt_tokens":5,"total_tokens":7}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ChatCompletionsURL: upstream.URL + "/v1/chat/completions",
		UpstreamProtocol:   upstreamProtocolChatCompletions,
		Timeout:            5 * time.Second,
	})
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

func TestOpenAIResponsesViaChatCompletionsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("unexpected accept header: %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("OpenAI-Beta") != "assistants=v2" {
			t.Fatalf("missing forwarded header: %q", r.Header.Get("OpenAI-Beta"))
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat completions request: %v", err)
		}
		if req["model"] != "gpt-4.1-mini" {
			t.Fatalf("unexpected model: %#v", req["model"])
		}
		if req["max_tokens"] != float64(32) {
			t.Fatalf("unexpected max tokens: %#v", req["max_tokens"])
		}
		if req["stream"] != true {
			t.Fatalf("expected stream=true: %#v", req)
		}
		streamOptions, ok := req["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("unexpected stream_options: %#v", req["stream_options"])
		}
		if req["temperature"] != 0.3 {
			t.Fatalf("unexpected temperature: %#v", req["temperature"])
		}
		if req["top_p"] != 0.9 {
			t.Fatalf("unexpected top_p: %#v", req["top_p"])
		}
		messages, ok := req["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("unexpected message count: %#v", req["messages"])
		}
		first, ok := messages[0].(map[string]any)
		if !ok || first["role"] != "system" || first["content"] != "be concise" {
			t.Fatalf("unexpected system message: %#v", messages[0])
		}
		second, ok := messages[1].(map[string]any)
		if !ok || second["role"] != "user" || second["content"] != "say hi" {
			t.Fatalf("unexpected user message: %#v", messages[1])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl_bridge_1","object":"chat.completion.chunk","created":1710000400,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"hello from bridge"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl_bridge_1","object":"chat.completion.chunk","created":1710000400,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":3,"prompt_tokens":4,"total_tokens":7}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ChatCompletionsURL: upstream.URL + "/v1/chat/completions",
		UpstreamProtocol:   upstreamProtocolChatCompletions,
		Timeout:            5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"gpt-4.1-mini",
		"instructions":"be concise",
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"say hi"}]}],
		"max_output_tokens":32,
		"temperature":0.3,
		"top_p":0.9
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("OpenAI-Beta", "assistants=v2")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp openAIResponsesResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "response" {
		t.Fatalf("unexpected object: %#v", resp)
	}
	if resp.Status != "completed" {
		t.Fatalf("unexpected status: %#v", resp)
	}
	if len(resp.Output) != 1 || len(resp.Output[0].Content) != 1 || resp.Output[0].Content[0].Text != "hello from bridge" {
		t.Fatalf("unexpected output: %#v", resp)
	}
	if resp.Usage == nil || resp.Usage.OutputTokens != 3 || resp.Usage.InputTokens != 4 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestOpenAIChatCompletionsViaStreamingUpstreamAggregates(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("unexpected accept header: %q", r.Header.Get("Accept"))
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat completions request: %v", err)
		}
		if req["stream"] != true {
			t.Fatalf("expected stream=true: %#v", req)
		}
		streamOptions, ok := req["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("unexpected stream_options: %#v", req["stream_options"])
		}
		if req["verbosity"] != "medium" {
			t.Fatalf("missing passthrough verbosity: %#v", req["verbosity"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl_passthrough_1","object":"chat.completion.chunk","created":1710000450,"model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant","content":"在"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl_passthrough_1","object":"chat.completion.chunk","created":1710000450,"model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"呢"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl_passthrough_1","object":"chat.completion.chunk","created":1710000450,"model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":2,"prompt_tokens":5,"total_tokens":7}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ChatCompletionsURL: upstream.URL + "/v1/chat/completions",
		UpstreamProtocol:   upstreamProtocolChatCompletions,
		Timeout:            5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"gpt-5.4",
		"verbosity":"medium",
		"messages":[{"role":"user","content":"hello"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	srv.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("unexpected content type: %q", got)
	}

	var resp openAIChatCompletionsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("unexpected object: %#v", resp)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "在呢" {
		t.Fatalf("unexpected choices: %#v", resp)
	}
	if resp.Usage == nil || resp.Usage.CompletionTokens != 2 || resp.Usage.PromptTokens != 5 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestOpenAIResponsesStreamingViaChatCompletionsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("unexpected accept header: %q", r.Header.Get("Accept"))
		}

		var req openAIChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat completions request: %v", err)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "say hi" {
			t.Fatalf("unexpected prompt: %#v", req.Messages)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl_stream_bridge","object":"chat.completion.chunk","created":1710000500,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":""}]}`,
			``,
			`data: {"id":"chatcmpl_stream_bridge","object":"chat.completion.chunk","created":1710000500,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{"content":"hel"},"finish_reason":""}]}`,
			``,
			`data: {"id":"chatcmpl_stream_bridge","object":"chat.completion.chunk","created":1710000500,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":""}]}`,
			``,
			`data: {"id":"chatcmpl_stream_bridge","object":"chat.completion.chunk","created":1710000500,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	srv := NewServer(Config{
		ChatCompletionsURL: upstream.URL + "/v1/chat/completions",
		UpstreamProtocol:   upstreamProtocolChatCompletions,
		Timeout:            5 * time.Second,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"gpt-4.1-mini",
		"input":"say hi",
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
	if !strings.Contains(body, `event: response.created`) {
		t.Fatalf("missing response.created: %s", body)
	}
	if !strings.Contains(body, `event: response.output_text.delta`) || !strings.Contains(body, `"delta":"hel"`) || !strings.Contains(body, `"delta":"lo"`) {
		t.Fatalf("missing output_text deltas: %s", body)
	}
	if !strings.Contains(body, `event: response.completed`) {
		t.Fatalf("missing response.completed: %s", body)
	}
	if !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("missing done marker: %s", body)
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

func TestSetUpstreamAuthorizationPrefersConfiguredKey(t *testing.T) {
	dst := http.Header{}
	src := http.Header{}
	src.Set("Authorization", "Bearer client-placeholder")

	source := setUpstreamAuthorization(dst, src, "real-upstream-key")

	if source != "configured" {
		t.Fatalf("unexpected source: %s", source)
	}
	if got := dst.Get("Authorization"); got != "Bearer real-upstream-key" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestSetUpstreamAuthorizationFallsBackToPassthrough(t *testing.T) {
	dst := http.Header{}
	src := http.Header{}
	src.Set("Authorization", "Bearer client-placeholder")

	source := setUpstreamAuthorization(dst, src, "")

	if source != "passthrough" {
		t.Fatalf("unexpected source: %s", source)
	}
	if got := dst.Get("Authorization"); got != "Bearer client-placeholder" {
		t.Fatalf("unexpected authorization header: %q", got)
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
