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

func TestResolveOpenAITargetCustom(t *testing.T) {
	target, err := ResolveOpenAITarget("https://relay.example.com/openai/responses", "custom")
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

	if err := app.StartProxy(context.Background(), upstream.URL, "", 0, "default"); err != nil {
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

	if err := app.StartProxy(context.Background(), upstream.URL, "", fixedPort, "default"); err != nil {
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
