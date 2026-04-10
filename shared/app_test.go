package proxyshared

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeOpenAIHost(t *testing.T) {
	baseURL, modelsURL, responsesURL, err := NormalizeOpenAIHost("https://example.com/custom/v1/responses")
	if err != nil {
		t.Fatalf("normalize host: %v", err)
	}

	if baseURL != "https://example.com/custom" {
		t.Fatalf("unexpected base url: %s", baseURL)
	}
	if modelsURL != "https://example.com/custom/v1/models" {
		t.Fatalf("unexpected models url: %s", modelsURL)
	}
	if responsesURL != "https://example.com/custom/v1/responses" {
		t.Fatalf("unexpected responses url: %s", responsesURL)
	}
}

func TestNormalizeOpenAIHostChatCompletionsPath(t *testing.T) {
	baseURL, modelsURL, responsesURL, err := NormalizeOpenAIHost("https://example.com/custom/v1/chat/completions")
	if err != nil {
		t.Fatalf("normalize host: %v", err)
	}

	if baseURL != "https://example.com/custom" {
		t.Fatalf("unexpected base url: %s", baseURL)
	}
	if modelsURL != "https://example.com/custom/v1/models" {
		t.Fatalf("unexpected models url: %s", modelsURL)
	}
	if responsesURL != "https://example.com/custom/v1/responses" {
		t.Fatalf("unexpected responses url: %s", responsesURL)
	}
}

func TestResolveOpenAITargetCustom(t *testing.T) {
	target, err := ResolveOpenAITarget("https://relay.example.com/openai/responses", "custom", "")
	if err != nil {
		t.Fatalf("resolve custom target: %v", err)
	}
	if target.DisplayURL != "https://relay.example.com/openai/responses" {
		t.Fatalf("unexpected display url: %s", target.DisplayURL)
	}
	if target.ModelsURL != "" {
		t.Fatalf("custom mode should not derive models url: %s", target.ModelsURL)
	}
	if target.ResponsesURL != "https://relay.example.com/openai/responses" {
		t.Fatalf("unexpected responses url: %s", target.ResponsesURL)
	}
}

func TestResolveOpenAITargetChatCompletions(t *testing.T) {
	target, err := ResolveOpenAITarget("https://api.openai.com", "default", "chat_completions")
	if err != nil {
		t.Fatalf("resolve default target: %v", err)
	}
	if target.DisplayURL != "https://api.openai.com" {
		t.Fatalf("unexpected display url: %s", target.DisplayURL)
	}
	if target.ChatCompletionsURL != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("unexpected chat completions url: %s", target.ChatCompletionsURL)
	}
	if target.UpstreamProtocol != upstreamProtocolChatCompletions {
		t.Fatalf("unexpected upstream protocol: %s", target.UpstreamProtocol)
	}
}

func TestListenerURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	url := listenerURL(ln, "127.0.0.1")
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("unexpected listener url: %s", url)
	}
}

func TestAppStartProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			writeJSON(w, http.StatusOK, openAIResponsesResponse{
				ID:        "resp_app_1",
				Model:     "gpt-4.1-mini",
				CreatedAt: 1710000020,
				Status:    "completed",
				Output: []openAIResponseOutput{{
					Type: "message",
					Role: "assistant",
					Content: []openAIResponseContent{{
						Type: "output_text",
						Text: "hello from app",
					}},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	app := &App{
		controlListen: "127.0.0.1:8080",
		displayHost:   "127.0.0.1",
		proxyBindHost: "127.0.0.1",
		timeout:       5 * time.Second,
		logger:        newLogHub(100),
	}

	if err := app.StartProxy(context.Background(), upstream.URL, "", 0, "default", "responses"); err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	defer func() { _ = app.StopProxy() }()

	status := app.SnapshotStatus()
	if !status.Running {
		t.Fatalf("expected running status")
	}
	if len(status.Routes) != 4 {
		t.Fatalf("unexpected routes: %#v", status.Routes)
	}

	chatCompletionsURL := ""
	for _, route := range status.Routes {
		if route.Path == "/v1/chat/completions" {
			chatCompletionsURL = route.URL
			break
		}
	}
	if chatCompletionsURL == "" {
		t.Fatalf("missing chat completions route: %#v", status.Routes)
	}

	resp, err := http.Post(chatCompletionsURL, "application/json", strings.NewReader(`{
		"model":"text-davinci-compat",
		"messages":[{"role":"user","content":"say hi"}],
		"max_tokens":16
	}`))
	if err != nil {
		t.Fatalf("request proxy: %v", err)
	}
	defer resp.Body.Close()

	var completionResp openAIChatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}
	if completionResp.Choices[0].Message.Content != "hello from app" {
		t.Fatalf("unexpected completion text: %#v", completionResp)
	}
}

func TestAppStartProxyWithFixedPort(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			writeJSON(w, http.StatusOK, openAIResponsesResponse{
				ID:        "resp_app_2",
				Model:     "gpt-4.1-mini",
				CreatedAt: 1710000030,
				Status:    "completed",
				Output:    []openAIResponseOutput{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	fixedPort := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()

	app := &App{
		controlListen: "127.0.0.1:8080",
		displayHost:   "127.0.0.1",
		proxyBindHost: "127.0.0.1",
		timeout:       5 * time.Second,
		logger:        newLogHub(100),
	}

	if err := app.StartProxy(context.Background(), upstream.URL, "", fixedPort, "default", "responses"); err != nil {
		t.Fatalf("start proxy with fixed port: %v", err)
	}
	defer func() { _ = app.StopProxy() }()

	status := app.SnapshotStatus()
	if status.ProxyPort != fixedPort {
		t.Fatalf("expected fixed port %d, got %d", fixedPort, status.ProxyPort)
	}
}

func TestCheckResponsesCompatibility(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var req openAIResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode preflight body: %v", err)
		}
		if req.Model != "gpt-5.4" {
			t.Fatalf("unexpected model: %s", req.Model)
		}
		input, ok := req.Input.([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("unexpected input: %#v", req.Input)
		}
		msg, ok := input[0].(map[string]any)
		if !ok || msg["type"] != "message" || msg["role"] != "user" || msg["content"] != "ping" {
			t.Fatalf("unexpected conversation input: %#v", input[0])
		}
		writeJSON(w, http.StatusOK, openAIResponsesResponse{
			ID:        "resp_preflight",
			Model:     req.Model,
			CreatedAt: 1710000100,
			Status:    "completed",
			Output:    []openAIResponseOutput{},
		})
	}))
	defer upstream.Close()

	app := &App{timeout: 5 * time.Second}
	if err := app.CheckResponsesCompatibility(context.Background(), upstream.URL+"/v1/responses", "", "gpt-5.4"); err != nil {
		t.Fatalf("responses compatibility: %v", err)
	}
}

func TestCheckChatCompletionsCompatibility(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req openAIChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode preflight body: %v", err)
		}
		if req.Model != "gpt-5.4" {
			t.Fatalf("unexpected model: %s", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "ping" {
			t.Fatalf("unexpected prompt messages: %#v", req.Messages)
		}
		writeJSON(w, http.StatusOK, openAIChatCompletionsResponse{
			ID:      "chatcmpl_preflight",
			Object:  "chat.completion",
			Created: 1710000200,
			Model:   req.Model,
			Choices: []openAIChatCompletionChoice{{
				Index: 0,
				Message: openAIChatOutputMessage{
					Role:    "assistant",
					Content: "pong",
				},
				FinishReason: "stop",
			}},
		})
	}))
	defer upstream.Close()

	app := &App{timeout: 5 * time.Second}
	if err := app.CheckChatCompletionsCompatibility(context.Background(), upstream.URL+"/v1/chat/completions", "", "gpt-5.4"); err != nil {
		t.Fatalf("chat completions compatibility: %v", err)
	}
}

func TestHandleTestConnectionResponsesReturnsFullResult(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}

		var req openAIResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode responses request: %v", err)
		}
		if req.Model != "gpt-5.4" {
			t.Fatalf("unexpected model: %s", req.Model)
		}
		if !req.Stream {
			t.Fatalf("expected stream=true: %#v", req)
		}
		input, ok := req.Input.([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("unexpected input: %#v", req.Input)
		}
		msg, ok := input[0].(map[string]any)
		if !ok || msg["role"] != "user" || msg["content"] != "hello from test" {
			t.Fatalf("unexpected message: %#v", input[0])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","id":"resp_test_1","model":"gpt-5.4"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"pong"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","id":"resp_test_1","model":"gpt-5.4","usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	app := &App{timeout: 5 * time.Second, logger: newLogHub(100)}
	request := httptest.NewRequest(http.MethodPost, "/api/test-connection", strings.NewReader(`{
		"host":"`+upstream.URL+`",
		"model":"gpt-5.4",
		"host_mode":"default",
		"protocol":"responses",
		"test_message":"hello from test"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	app.handleTestConnection(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		OK               bool            `json:"ok"`
		UpstreamProtocol string          `json:"upstream_protocol"`
		UpstreamTarget   string          `json:"upstream_target"`
		StatusCode       int             `json:"status_code"`
		RequestPayload   json.RawMessage `json:"request_payload"`
		ResponseBody     string          `json:"response_body"`
		ResponseJSON     json.RawMessage `json:"response_json"`
		TestMessage      string          `json:"test_message"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode test response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok result: %#v", resp)
	}
	if resp.UpstreamProtocol != upstreamProtocolResponses {
		t.Fatalf("unexpected protocol: %#v", resp)
	}
	if resp.UpstreamTarget != upstream.URL+"/v1/responses" {
		t.Fatalf("unexpected target: %#v", resp)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %#v", resp)
	}
	if resp.TestMessage != "hello from test" {
		t.Fatalf("unexpected test message: %#v", resp)
	}
	if !strings.Contains(string(resp.RequestPayload), `"content":"hello from test"`) {
		t.Fatalf("missing request payload content: %s", string(resp.RequestPayload))
	}
	if !strings.Contains(string(resp.RequestPayload), `"stream":true`) {
		t.Fatalf("missing stream request: %s", string(resp.RequestPayload))
	}
	if !strings.Contains(resp.ResponseBody, `event: response.created`) || !strings.Contains(resp.ResponseBody, `data: [DONE]`) {
		t.Fatalf("missing response body: %s", resp.ResponseBody)
	}
	if len(resp.ResponseJSON) != 0 {
		t.Fatalf("stream test should not return response_json: %s", string(resp.ResponseJSON))
	}
}

func TestHandleTestConnectionChatCompletionsReturnsFullResult(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat completions request: %v", err)
		}
		if req["model"] != "gpt-5.4" {
			t.Fatalf("unexpected model: %#v", req["model"])
		}
		if req["stream"] != true {
			t.Fatalf("expected stream=true: %#v", req)
		}
		streamOptions, ok := req["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("unexpected stream_options: %#v", req["stream_options"])
		}
		messages, ok := req["messages"].([]any)
		if !ok || len(messages) != 1 {
			t.Fatalf("unexpected messages: %#v", req["messages"])
		}
		msg, ok := messages[0].(map[string]any)
		if !ok || msg["role"] != "user" || msg["content"] != "hello chat" {
			t.Fatalf("unexpected message payload: %#v", messages[0])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl_test_1","object":"chat.completion.chunk","created":1710000610,"model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant","content":"pong"},"finish_reason":null,"native_finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl_test_1","object":"chat.completion.chunk","created":1710000610,"model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop","native_finish_reason":"stop"}],"usage":{"completion_tokens":1,"total_tokens":2,"prompt_tokens":1}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer upstream.Close()

	app := &App{timeout: 5 * time.Second, logger: newLogHub(100)}
	request := httptest.NewRequest(http.MethodPost, "/api/test-connection", strings.NewReader(`{
		"host":"`+upstream.URL+`",
		"model":"gpt-5.4",
		"host_mode":"default",
		"protocol":"chat_completions",
		"test_message":"hello chat"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	app.handleTestConnection(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		OK               bool            `json:"ok"`
		UpstreamProtocol string          `json:"upstream_protocol"`
		UpstreamTarget   string          `json:"upstream_target"`
		StatusCode       int             `json:"status_code"`
		RequestPayload   json.RawMessage `json:"request_payload"`
		ResponseBody     string          `json:"response_body"`
		ResponseJSON     json.RawMessage `json:"response_json"`
		TestMessage      string          `json:"test_message"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode test response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok result: %#v", resp)
	}
	if resp.UpstreamProtocol != upstreamProtocolChatCompletions {
		t.Fatalf("unexpected protocol: %#v", resp)
	}
	if resp.UpstreamTarget != upstream.URL+"/v1/chat/completions" {
		t.Fatalf("unexpected target: %#v", resp)
	}
	if !strings.Contains(string(resp.RequestPayload), `"content":"hello chat"`) {
		t.Fatalf("missing request payload content: %s", string(resp.RequestPayload))
	}
	if !strings.Contains(string(resp.RequestPayload), `"stream":true`) {
		t.Fatalf("missing forced stream request: %s", string(resp.RequestPayload))
	}
	if !strings.Contains(string(resp.RequestPayload), `"include_usage":true`) {
		t.Fatalf("missing stream options include_usage: %s", string(resp.RequestPayload))
	}
	if !strings.Contains(resp.ResponseBody, `data: {"id":"chatcmpl_test_1"`) {
		t.Fatalf("missing response body: %s", resp.ResponseBody)
	}
	if len(resp.ResponseJSON) != 0 {
		t.Fatalf("stream test should not return response_json: %s", string(resp.ResponseJSON))
	}
}

func TestHandleTestConnectionUpstreamStatusReturnsStructuredFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer upstream.Close()

	app := &App{timeout: 5 * time.Second, logger: newLogHub(100)}
	request := httptest.NewRequest(http.MethodPost, "/api/test-connection", strings.NewReader(`{
		"host":"`+upstream.URL+`",
		"model":"gpt-5.4",
		"host_mode":"default",
		"protocol":"responses",
		"test_message":"ping"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	app.handleTestConnection(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		OK           bool   `json:"ok"`
		StatusCode   int    `json:"status_code"`
		ResponseBody string `json:"response_body"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected failed test result: %#v", resp)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected upstream status: %#v", resp)
	}
	if resp.ResponseBody != `{"error":{"message":"bad key"}}` {
		t.Fatalf("unexpected response body: %#v", resp)
	}
}

func TestRunOpenAITestDefaultsToHello(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}

		var req openAIChatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat completions request: %v", err)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Fatalf("unexpected messages: %#v", req.Messages)
		}

		writeJSON(w, http.StatusOK, openAIChatCompletionsResponse{
			ID:      "chatcmpl_test_default_hello",
			Object:  "chat.completion",
			Created: 1710000620,
			Model:   req.Model,
			Choices: []openAIChatCompletionChoice{},
		})
	}))
	defer upstream.Close()

	app := &App{timeout: 5 * time.Second}
	result, err := app.RunOpenAITest(context.Background(), OpenAITarget{
		DisplayURL:         upstream.URL,
		ChatCompletionsURL: upstream.URL + "/v1/chat/completions",
		UpstreamProtocol:   upstreamProtocolChatCompletions,
	}, "", "gpt-5.4", "")
	if err != nil {
		t.Fatalf("run openai test: %v", err)
	}

	if result.TestMessage != "hello" {
		t.Fatalf("unexpected default test message: %#v", result)
	}
}

func TestAppStartProxyWithChatCompletionsUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat completions request: %v", err)
			}
			if req["stream"] != true {
				t.Fatalf("expected stream=true: %#v", req)
			}
			messages, ok := req["messages"].([]any)
			if !ok || len(messages) != 1 {
				t.Fatalf("unexpected messages: %#v", req["messages"])
			}
			msg, ok := messages[0].(map[string]any)
			if !ok || msg["role"] != "user" || msg["content"] != "say hi" {
				t.Fatalf("unexpected message payload: %#v", messages[0])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(strings.Join([]string{
				`data: {"id":"chatcmpl_app_1","object":"chat.completion.chunk","created":1710000300,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"hello from chat completions"},"finish_reason":null}]}`,
				``,
				`data: {"id":"chatcmpl_app_1","object":"chat.completion.chunk","created":1710000300,"model":"gpt-4.1-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":4,"prompt_tokens":3,"total_tokens":7}}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	app := &App{
		controlListen: "127.0.0.1:8080",
		displayHost:   "127.0.0.1",
		proxyBindHost: "127.0.0.1",
		timeout:       5 * time.Second,
		logger:        newLogHub(100),
	}

	if err := app.StartProxy(context.Background(), upstream.URL, "", 0, "default", "chat_completions"); err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	defer func() { _ = app.StopProxy() }()

	status := app.SnapshotStatus()
	if !status.Running {
		t.Fatalf("expected running status")
	}

	responsesURL := ""
	for _, route := range status.Routes {
		if route.Path == "/v1/responses" {
			responsesURL = route.URL
			break
		}
	}
	if responsesURL == "" {
		t.Fatalf("missing responses route: %#v", status.Routes)
	}

	resp, err := http.Post(responsesURL, "application/json", strings.NewReader(`{
		"model":"gpt-4.1-mini",
		"input":"say hi"
	}`))
	if err != nil {
		t.Fatalf("request proxy: %v", err)
	}
	defer resp.Body.Close()

	var responsesResp openAIResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&responsesResp); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}
	if len(responsesResp.Output) != 1 || len(responsesResp.Output[0].Content) != 1 || responsesResp.Output[0].Content[0].Text != "hello from chat completions" {
		t.Fatalf("unexpected responses text: %#v", responsesResp)
	}
}

func TestAppControlServerLifecycle(t *testing.T) {
	app, err := NewApp(AppOptions{
		DefaultControlListen: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	indexCalls := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		indexCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}

	if err := app.StartControlServer(handler, false); err != nil {
		t.Fatalf("start control server: %v", err)
	}

	if err := app.StartControlServer(handler, false); err == nil {
		t.Fatalf("expected duplicate control server start to fail")
	}

	resp, err := http.Get(app.ControlAddr() + "/")
	if err != nil {
		t.Fatalf("get control index: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected control index status: %d", resp.StatusCode)
	}
	if indexCalls != 1 {
		t.Fatalf("unexpected index call count: %d", indexCalls)
	}

	if err := app.StopControlServer(); err != nil {
		t.Fatalf("stop control server: %v", err)
	}
}
