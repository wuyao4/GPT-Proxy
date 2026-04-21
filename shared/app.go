package proxyshared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AppOptions struct {
	DefaultControlListen string
	DefaultProxyBindHost string
	DefaultDisplayHost   string
}

type App struct {
	controlListen string
	displayHost   string
	proxyBindHost string
	timeout       time.Duration
	logger        *LogHub

	mu              sync.Mutex
	proxy           *runningProxy
	controlURL      string
	controlServer   *http.Server
	controlListener net.Listener
	controlDone     chan error
}

type runningProxy struct {
	server      *http.Server
	listener    net.Listener
	displayURL  string
	upstreamURL string
	protocol    string
	startedAt   time.Time
}

type testConnectionRequest struct {
	Host        string `json:"host"`
	Key         string `json:"key"`
	Model       string `json:"model,omitempty"`
	HostMode    string `json:"host_mode,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	TestMessage string `json:"test_message,omitempty"`
}

type startProxyRequest struct {
	Host     string `json:"host"`
	Key      string `json:"key"`
	Port     int    `json:"port,omitempty"`
	HostMode string `json:"host_mode,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

type StatusResponse struct {
	Running           bool           `json:"running"`
	ControlURL        string         `json:"control_url"`
	ProxyBaseURL      string         `json:"proxy_base_url,omitempty"`
	ProxyPort         int            `json:"proxy_port,omitempty"`
	UpstreamTarget    string         `json:"upstream_target_url,omitempty"`
	UpstreamResponses string         `json:"upstream_responses_url,omitempty"`
	UpstreamProtocol  string         `json:"upstream_protocol,omitempty"`
	Routes            []RouteDisplay `json:"routes"`
	StartedAt         string         `json:"started_at,omitempty"`
}

type RouteDisplay struct {
	Name string `json:"name"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

type OpenAITestResult struct {
	OK                   bool            `json:"ok"`
	NormalizedHost       string          `json:"normalized_host,omitempty"`
	UpstreamTarget       string          `json:"upstream_target"`
	ResponsesProxyTarget string          `json:"responses_proxy_target,omitempty"`
	UpstreamProtocol     string          `json:"upstream_protocol"`
	Model                string          `json:"model"`
	TestMessage          string          `json:"test_message"`
	StatusCode           int             `json:"status_code"`
	RequestPayload       json.RawMessage `json:"request_payload,omitempty"`
	ResponseBody         string          `json:"response_body,omitempty"`
	ResponseJSON         json.RawMessage `json:"response_json,omitempty"`
}

type OpenAITarget struct {
	DisplayURL         string
	ModelsURL          string
	ResponsesURL       string
	ChatCompletionsURL string
	MessagesURL        string
	UpstreamProtocol   string
}

func (t OpenAITarget) UpstreamURL() string {
	switch normalizeUpstreamProtocol(t.UpstreamProtocol) {
	case upstreamProtocolChatCompletions:
		return t.ChatCompletionsURL
	case upstreamProtocolMessages:
		return t.MessagesURL
	default:
		return t.ResponsesURL
	}
}

func NewApp(opts AppOptions) (*App, error) {
	controlListen := strings.TrimSpace(os.Getenv("CONTROL_ADDR"))
	if controlListen == "" {
		controlListen = fallback(opts.DefaultControlListen, "127.0.0.1:0")
	}

	proxyBindHost := strings.TrimSpace(os.Getenv("PROXY_BIND_HOST"))
	if proxyBindHost == "" {
		proxyBindHost = fallback(opts.DefaultProxyBindHost, "127.0.0.1")
	}

	displayHost := strings.TrimSpace(os.Getenv("DISPLAY_HOST"))
	if displayHost == "" {
		displayHost = fallback(opts.DefaultDisplayHost, proxyBindHost)
	}

	timeout := 60 * time.Second
	if raw := strings.TrimSpace(os.Getenv("HTTP_TIMEOUT_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP_TIMEOUT_SECONDS: %w", err)
		}
		timeout = time.Duration(seconds) * time.Second
	}

	logger := newLogHub(500)
	logger.Printf("control panel bootstrap on %s", controlListen)

	return &App{
		controlListen: controlListen,
		displayHost:   displayHost,
		proxyBindHost: proxyBindHost,
		timeout:       timeout,
		logger:        logger,
	}, nil
}

func (a *App) Serve(indexHandler http.HandlerFunc, autoOpenBrowser bool) error {
	if err := a.StartControlServer(indexHandler, autoOpenBrowser); err != nil {
		return err
	}
	return a.WaitControlServer()
}

func (a *App) StartControlServer(indexHandler http.HandlerFunc, autoOpenBrowser bool) error {
	a.mu.Lock()
	if a.controlServer != nil {
		a.mu.Unlock()
		return errors.New("control server is already running")
	}
	controlListen := a.controlListen
	displayHost := a.displayHost
	a.mu.Unlock()

	ln, err := net.Listen("tcp", controlListen)
	if err != nil {
		return err
	}

	controlURL := listenerURL(ln, displayHost)
	server := &http.Server{Handler: a.Routes(indexHandler)}
	done := make(chan error, 1)

	a.mu.Lock()
	a.controlURL = controlURL
	a.controlServer = server
	a.controlListener = ln
	a.controlDone = done
	a.mu.Unlock()
	a.logger.Printf("control panel listening on %s", controlURL)

	if autoOpenBrowser {
		go func() {
			if err := openBrowser(controlURL); err != nil {
				a.logger.Printf("open browser failed: %v", err)
			}
		}()
	}

	go func() {
		serveErr := server.Serve(ln)
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		if serveErr != nil {
			a.logger.Printf("control panel stopped with error: %v", serveErr)
		}

		a.mu.Lock()
		if a.controlServer == server {
			a.controlServer = nil
			a.controlListener = nil
			a.controlDone = nil
		}
		a.mu.Unlock()

		done <- serveErr
		close(done)
	}()

	return nil
}

func (a *App) WaitControlServer() error {
	a.mu.Lock()
	done := a.controlDone
	a.mu.Unlock()

	if done == nil {
		return nil
	}

	return waitControlDone(done)
}

func (a *App) StopControlServer() error {
	a.mu.Lock()
	server := a.controlServer
	done := a.controlDone
	a.mu.Unlock()

	if server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	shutdownErr := server.Shutdown(ctx)
	waitErr := waitControlDone(done)
	if shutdownErr != nil {
		return shutdownErr
	}
	if waitErr != nil {
		return waitErr
	}

	a.logger.Printf("control panel stopped")
	return nil
}

func (a *App) Routes(indexHandler http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if indexHandler == nil {
			http.NotFound(w, r)
			return
		}
		indexHandler(w, r)
	})
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/test-connection", a.handleTestConnection)
	mux.HandleFunc("/api/start", a.handleStart)
	mux.HandleFunc("/api/stop", a.handleStop)
	mux.HandleFunc("/api/logs", a.handleLogs)
	return mux
}

func (a *App) ControlAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.controlAddrLocked()
}

func (a *App) Logger() *LogHub {
	return a.logger
}

func (a *App) SetProxyHosts(bindHost, displayHost string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if strings.TrimSpace(bindHost) != "" {
		a.proxyBindHost = strings.TrimSpace(bindHost)
	}
	if strings.TrimSpace(displayHost) != "" {
		a.displayHost = strings.TrimSpace(displayHost)
	}
}

func (a *App) StartProxy(ctx context.Context, host, key string, port int, hostMode, protocol string) error {
	_ = ctx

	target, err := ResolveOpenAITarget(host, hostMode, protocol)
	if err != nil {
		return err
	}

	if port < 0 || port > 65535 {
		return errors.New("port must be between 0 and 65535")
	}

	listenPort := "0"
	if port > 0 {
		listenPort = strconv.Itoa(port)
	}

	a.mu.Lock()
	proxyBindHost := a.proxyBindHost
	displayHost := a.displayHost
	a.mu.Unlock()

	ln, err := net.Listen("tcp", net.JoinHostPort(proxyBindHost, listenPort))
	if err != nil {
		return fmt.Errorf("listen proxy: %w", err)
	}

	actualPort := 0
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		actualPort = tcpAddr.Port
	}
	displayURL := fmt.Sprintf("http://%s:%d", displayHost, actualPort)

	proxyCfg := Config{
		ListenAddr:         ln.Addr().String(),
		ModelsURL:          target.ModelsURL,
		ResponsesURL:       target.ResponsesURL,
		ChatCompletionsURL: target.ChatCompletionsURL,
		MessagesURL:        target.MessagesURL,
		UpstreamProtocol:   target.UpstreamProtocol,
		APIKey:             strings.TrimSpace(key),
		Timeout:            a.timeout,
	}
	proxySrv := newServerWithLogger(proxyCfg, a.logger)
	httpSrv := &http.Server{Handler: proxySrv.Routes()}

	a.mu.Lock()
	old := a.proxy
	a.proxy = &runningProxy{
		server:      httpSrv,
		listener:    ln,
		displayURL:  displayURL,
		upstreamURL: target.UpstreamURL(),
		protocol:    target.UpstreamProtocol,
		startedAt:   time.Now(),
	}
	a.mu.Unlock()

	if old != nil {
		_ = shutdownProxy(old)
		a.logger.Printf("replaced previous proxy instance")
	}

	go func() {
		if serveErr := httpSrv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			a.logger.Printf("proxy server stopped with error: %v", serveErr)
		}
	}()

	a.logger.Printf("proxy started on %s", displayURL)
	a.logger.Printf("upstream protocol: %s", target.UpstreamProtocol)
	a.logger.Printf("OpenAI Models route: %s/v1/models", displayURL)
	a.logger.Printf("OpenAI Responses route: %s/v1/responses", displayURL)
	a.logger.Printf("Claude Messages route: %s/v1/messages", displayURL)
	a.logger.Printf("OpenAI Chat Completions route: %s/v1/chat/completions", displayURL)
	return nil
}

func (a *App) StopProxy() error {
	a.mu.Lock()
	current := a.proxy
	a.proxy = nil
	a.mu.Unlock()

	if current == nil {
		return nil
	}

	if err := shutdownProxy(current); err != nil {
		return err
	}

	a.logger.Printf("proxy stopped")
	return nil
}

func shutdownProxy(current *runningProxy) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return current.server.Shutdown(ctx)
}

func waitControlDone(done <-chan error) error {
	if done == nil {
		return nil
	}
	if err, ok := <-done; ok {
		return err
	}
	return nil
}

func (a *App) SnapshotStatus() StatusResponse {
	a.mu.Lock()
	defer a.mu.Unlock()

	resp := StatusResponse{
		Running:    a.proxy != nil,
		ControlURL: a.controlAddrLocked(),
		Routes:     []RouteDisplay{},
	}
	if a.proxy == nil {
		return resp
	}

	resp.ProxyBaseURL = a.proxy.displayURL
	resp.UpstreamTarget = a.proxy.upstreamURL
	resp.UpstreamResponses = a.proxy.upstreamURL
	resp.UpstreamProtocol = a.proxy.protocol
	resp.StartedAt = a.proxy.startedAt.Format(time.RFC3339)
	resp.Routes = []RouteDisplay{
		{Name: "OpenAI Models", Path: "/v1/models", URL: a.proxy.displayURL + "/v1/models"},
		{Name: "OpenAI Responses", Path: "/v1/responses", URL: a.proxy.displayURL + "/v1/responses"},
		{Name: "Claude Messages", Path: "/v1/messages", URL: a.proxy.displayURL + "/v1/messages"},
		{Name: "OpenAI Chat Completions", Path: "/v1/chat/completions", URL: a.proxy.displayURL + "/v1/chat/completions"},
	}

	if u, err := url.Parse(a.proxy.displayURL); err == nil {
		if port, parseErr := strconv.Atoi(u.Port()); parseErr == nil {
			resp.ProxyPort = port
		}
	}

	return resp
}

func (a *App) CheckResponsesCompatibility(ctx context.Context, responsesURL, key, model string) error {
	payload := openAIResponsesRequest{
		Model: strings.TrimSpace(model),
		Input: []map[string]any{{
			"type":    "message",
			"role":    "user",
			"content": "ping",
		}},
		MaxOutputTokens: 16,
		Stream:          false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal responses preflight: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build responses preflight request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	}

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("conversation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if readErr != nil {
			return fmt.Errorf("conversation status %d and failed to read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("conversation status %d for model %s: %s", resp.StatusCode, model, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (a *App) CheckChatCompletionsCompatibility(ctx context.Context, chatCompletionsURL, key, model string) error {
	payload := openAIChatCompletionsRequest{
		Model: strings.TrimSpace(model),
		Messages: []openAIChatInputMessage{{
			Role:    "user",
			Content: "ping",
		}},
		MaxTokens: 16,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal chat completions preflight: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build chat completions preflight request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	}

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("conversation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if readErr != nil {
			return fmt.Errorf("conversation status %d and failed to read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("conversation status %d for model %s: %s", resp.StatusCode, model, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (a *App) CheckOpenAICompatibility(ctx context.Context, target OpenAITarget, key, model string) error {
	switch normalizeUpstreamProtocol(target.UpstreamProtocol) {
	case upstreamProtocolChatCompletions:
		return a.CheckChatCompletionsCompatibility(ctx, target.ChatCompletionsURL, key, model)
	case upstreamProtocolMessages:
		return a.CheckMessagesCompatibility(ctx, target.MessagesURL, key, model)
	default:
		return a.CheckResponsesCompatibility(ctx, target.ResponsesURL, key, model)
	}
}

func (a *App) RunOpenAITest(ctx context.Context, target OpenAITarget, key, model, testMessage string) (OpenAITestResult, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return OpenAITestResult{}, errors.New("model is required for connection test")
	}

	message := strings.TrimSpace(testMessage)
	if message == "" {
		message = "hello"
	}

	result := OpenAITestResult{
		NormalizedHost:       target.DisplayURL,
		UpstreamTarget:       target.UpstreamURL(),
		ResponsesProxyTarget: target.UpstreamURL(),
		UpstreamProtocol:     normalizeUpstreamProtocol(target.UpstreamProtocol),
		Model:                model,
		TestMessage:          message,
	}

	switch result.UpstreamProtocol {
	case upstreamProtocolChatCompletions:
		payload := forceStreamingChatCompletionsRequest(openAIChatCompletionsRequest{
			Model: model,
			Messages: []openAIChatInputMessage{{
				Role:    "user",
				Content: message,
			}},
			MaxTokens: 16,
		})
		return a.executeOpenAIStreamTestRequest(ctx, result, key, target.ChatCompletionsURL, payload)
	case upstreamProtocolMessages:
		payload := claudeMessagesRequest{
			Model: model,
			Messages: []claudeInputMessage{{
				Role:    "user",
				Content: mustMarshalJSON("ping"),
			}},
			MaxTokens: 16,
			Stream:    true,
		}
		return a.executeMessagesStreamTestRequest(ctx, result, key, target.MessagesURL, payload)
	default:
		payload := openAIResponsesRequest{
			Model: model,
			Input: []map[string]any{{
				"type":    "message",
				"role":    "user",
				"content": message,
			}},
			MaxOutputTokens: 16,
			Stream:          true,
		}
		return a.executeOpenAIStreamTestRequest(ctx, result, key, target.ResponsesURL, payload)
	}
}

func (a *App) executeOpenAITestRequest(ctx context.Context, result OpenAITestResult, key, targetURL string, payload any) (OpenAITestResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("marshal test request: %w", err)
	}
	result.RequestPayload = json.RawMessage(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("build test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	}

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("conversation request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("read test response body: %w", err)
	}

	result.StatusCode = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	result.ResponseBody = strings.TrimSpace(string(raw))

	if json.Valid(raw) {
		result.ResponseJSON = json.RawMessage(raw)
	}

	return result, nil
}

func (a *App) executeOpenAIStreamTestRequest(ctx context.Context, result OpenAITestResult, key, targetURL string, payload any) (OpenAITestResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("marshal test request: %w", err)
	}
	result.RequestPayload = json.RawMessage(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("build test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	}

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("conversation request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("read test response body: %w", err)
	}

	result.StatusCode = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	result.ResponseBody = strings.TrimSpace(string(raw))
	return result, nil
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, a.SnapshotStatus())
}

func (a *App) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req testConnectionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	target, err := ResolveOpenAITarget(req.Host, req.HostMode, req.Protocol)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "model is required for connection test")
		return
	}

	result, err := a.RunOpenAITest(r.Context(), target, req.Key, req.Model, req.TestMessage)
	if err != nil {
		a.logger.Printf("conversation test failed for %s model=%s: %v", target.DisplayURL, req.Model, err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	a.logger.Printf("conversation test completed for %s model=%s protocol=%s upstream_status=%d ok=%t", target.DisplayURL, result.Model, result.UpstreamProtocol, result.StatusCode, result.OK)
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req startProxyRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.StartProxy(r.Context(), req.Host, req.Key, req.Port, req.HostMode, req.Protocol); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, a.SnapshotStatus())
}

func (a *App) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := a.StopProxy(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.SnapshotStatus())
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by the server")
		return
	}

	initSSEHeaders(w)
	for _, line := range a.logger.Snapshot() {
		if err := writeSSEDataRaw(w, flusher, line); err != nil {
			return
		}
	}

	updates, cancel := a.logger.Subscribe()
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-updates:
			if err := writeSSEDataRaw(w, flusher, line); err != nil {
				return
			}
		case <-ticker.C:
			if err := writeSSEDataRaw(w, flusher, ""); err != nil {
				return
			}
		}
	}
}

func ResolveOpenAITarget(raw, mode, protocol string) (OpenAITarget, error) {
	upstreamProtocol := normalizeUpstreamProtocol(protocol)
	if upstreamProtocol == "" {
		return OpenAITarget{}, errors.New("protocol must be responses, chat_completions, or messages")
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		baseURL, modelsURL, responsesURL, err := NormalizeOpenAIHost(raw)
		if err != nil {
			return OpenAITarget{}, err
		}
		return OpenAITarget{
			DisplayURL:         baseURL,
			ModelsURL:          modelsURL,
			ResponsesURL:       responsesURL,
			ChatCompletionsURL: baseURL + "/v1/chat/completions",
			MessagesURL:        baseURL + "/v1/messages",
			UpstreamProtocol:   upstreamProtocol,
		}, nil
	case "custom":
		exactURL, err := NormalizeAbsoluteURL(raw)
		if err != nil {
			return OpenAITarget{}, err
		}
		target := OpenAITarget{
			DisplayURL:       exactURL,
			UpstreamProtocol: upstreamProtocol,
		}
		switch upstreamProtocol {
		case upstreamProtocolChatCompletions:
			target.ChatCompletionsURL = exactURL
		case upstreamProtocolMessages:
			target.MessagesURL = exactURL
		default:
			target.ResponsesURL = exactURL
		}
		return target, nil
	default:
		return OpenAITarget{}, errors.New("host_mode must be default or custom")
	}
}

func NormalizeOpenAIHost(raw string) (baseURL string, modelsURL string, responsesURL string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", "", errors.New("host is required")
	}
	parsed, err := normalizeURL(trimmed)
	if err != nil {
		return "", "", "", err
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/responses"):
		path = strings.TrimSuffix(path, "/v1/responses")
	case strings.HasSuffix(path, "/v1/chat/completions"):
		path = strings.TrimSuffix(path, "/v1/chat/completions")
	case strings.HasSuffix(path, "/v1"):
		path = strings.TrimSuffix(path, "/v1")
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""

	baseURL = strings.TrimRight(parsed.String(), "/")
	modelsURL = baseURL + "/v1/models"
	responsesURL = baseURL + "/v1/responses"
	return baseURL, modelsURL, responsesURL, nil
}

func NormalizeAbsoluteURL(raw string) (string, error) {
	parsed, err := normalizeURL(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("host is required")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid host: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("host must include a valid scheme and host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func displayControlAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

func (a *App) controlAddrLocked() string {
	if a.controlURL != "" {
		return a.controlURL
	}
	return "http://" + displayControlAddr(a.controlListen)
}

func listenerURL(ln net.Listener, displayHost string) string {
	port := 0
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		port = tcpAddr.Port
	}
	return fmt.Sprintf("http://%s:%d", displayHost, port)
}

func openBrowser(targetURL string) error {
	if targetURL == "" {
		return errors.New("browser target url is empty")
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	return cmd.Start()
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return defaultValue
}

func (a *App) CheckMessagesCompatibility(ctx context.Context, messagesURL, key, model string) error {
	content, _ := json.Marshal("ping")
	payload := claudeMessagesRequest{
		Model: strings.TrimSpace(model),
		Messages: []claudeInputMessage{{
			Role:    "user",
			Content: json.RawMessage(content),
		}},
		MaxTokens: 16,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal messages preflight: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, messagesURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build messages preflight request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
		req.Header.Set("x-api-key", strings.TrimSpace(key))
	}
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("messages request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if readErr != nil {
			return fmt.Errorf("messages status %d and failed to read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("messages status %d for model %s: %s", resp.StatusCode, model, strings.TrimSpace(string(raw)))
	}
	return nil
}

func mustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

func (a *App) executeMessagesStreamTestRequest(ctx context.Context, result OpenAITestResult, key, targetURL string, payload claudeMessagesRequest) (OpenAITestResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("marshal test request: %w", err)
	}
	result.RequestPayload = json.RawMessage(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("build test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	if k := strings.TrimSpace(key); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
		req.Header.Set("x-api-key", k)
	}

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("conversation request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OpenAITestResult{}, fmt.Errorf("read test response body: %w", err)
	}

	result.StatusCode = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	result.ResponseBody = strings.TrimSpace(string(raw))
	return result, nil
}
