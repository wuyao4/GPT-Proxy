package main

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

type app struct {
	controlListen string
	displayHost   string
	proxyBindHost string
	timeout       time.Duration
	logger        *logHub

	mu         sync.Mutex
	proxy      *runningProxy
	controlURL string
}

type runningProxy struct {
	server       *http.Server
	listener     net.Listener
	displayURL   string
	responsesURL string
	startedAt    time.Time
}

type testConnectionRequest struct {
	Host     string `json:"host"`
	Key      string `json:"key"`
	Model    string `json:"model,omitempty"`
	HostMode string `json:"host_mode,omitempty"`
}

type startProxyRequest struct {
	Host     string `json:"host"`
	Key      string `json:"key"`
	Port     int    `json:"port,omitempty"`
	HostMode string `json:"host_mode,omitempty"`
}

type statusResponse struct {
	Running           bool           `json:"running"`
	ControlURL        string         `json:"control_url"`
	ProxyBaseURL      string         `json:"proxy_base_url,omitempty"`
	ProxyPort         int            `json:"proxy_port,omitempty"`
	UpstreamResponses string         `json:"upstream_responses_url,omitempty"`
	Routes            []routeDisplay `json:"routes"`
	StartedAt         string         `json:"started_at,omitempty"`
}

type routeDisplay struct {
	Name string `json:"name"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

func newApp() (*app, error) {
	controlListen := strings.TrimSpace(os.Getenv("CONTROL_ADDR"))
	if controlListen == "" {
		controlListen = "127.0.0.1:0"
	}

	proxyBindHost := strings.TrimSpace(os.Getenv("PROXY_BIND_HOST"))
	if proxyBindHost == "" {
		proxyBindHost = "127.0.0.1"
	}

	displayHost := strings.TrimSpace(os.Getenv("DISPLAY_HOST"))
	if displayHost == "" {
		displayHost = proxyBindHost
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

	return &app{
		controlListen: controlListen,
		displayHost:   displayHost,
		proxyBindHost: proxyBindHost,
		timeout:       timeout,
		logger:        logger,
	}, nil
}

func (a *app) serve() error {
	ln, err := net.Listen("tcp", a.controlListen)
	if err != nil {
		return err
	}

	controlURL := listenerURL(ln, a.displayHost)
	a.mu.Lock()
	a.controlURL = controlURL
	a.mu.Unlock()
	a.logger.Printf("control panel listening on %s", controlURL)

	go func() {
		if err := openBrowser(controlURL); err != nil {
			a.logger.Printf("open browser failed: %v", err)
		}
	}()

	server := &http.Server{Handler: a.routes()}
	return server.Serve(ln)
}

func (a *app) controlAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.controlAddrLocked()
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/test-connection", a.handleTestConnection)
	mux.HandleFunc("/api/start", a.handleStart)
	mux.HandleFunc("/api/stop", a.handleStop)
	mux.HandleFunc("/api/logs", a.handleLogs)
	return mux
}

func (a *app) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, a.snapshotStatus())
}

func (a *app) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req testConnectionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	target, err := resolveOpenAITarget(req.Host, req.HostMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "model is required for connection test")
		return
	}

	if err := a.checkResponsesCompatibility(r.Context(), target.ResponsesURL, req.Key, req.Model); err != nil {
		a.logger.Printf("conversation test failed for %s model=%s: %v", target.DisplayURL, req.Model, err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	a.logger.Printf("conversation test succeeded for %s model=%s", target.DisplayURL, req.Model)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":                 "ok",
		"normalized_host":        target.DisplayURL,
		"responses_proxy_target": target.ResponsesURL,
		"conversation_checked":   "ok",
	})
}

func (a *app) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req startProxyRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.startProxy(r.Context(), req.Host, req.Key, req.Port, req.HostMode); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, a.snapshotStatus())
}

func (a *app) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := a.stopProxy(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.snapshotStatus())
}

func (a *app) handleLogs(w http.ResponseWriter, r *http.Request) {
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

func (a *app) checkResponsesCompatibility(ctx context.Context, responsesURL, key, model string) error {
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

func (a *app) startProxy(ctx context.Context, host, key string, port int, hostMode string) error {
	target, err := resolveOpenAITarget(host, hostMode)
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

	ln, err := net.Listen("tcp", net.JoinHostPort(a.proxyBindHost, listenPort))
	if err != nil {
		return fmt.Errorf("listen proxy: %w", err)
	}

	actualPort := 0
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		actualPort = tcpAddr.Port
	}
	displayURL := fmt.Sprintf("http://%s:%d", a.displayHost, actualPort)

	proxyCfg := config{
		ListenAddr:   ln.Addr().String(),
		ModelsURL:    target.ModelsURL,
		ResponsesURL: target.ResponsesURL,
		APIKey:       strings.TrimSpace(key),
		Timeout:      a.timeout,
	}
	proxySrv := newServerWithLogger(proxyCfg, a.logger)
	httpSrv := &http.Server{Handler: proxySrv.routes()}

	a.mu.Lock()
	old := a.proxy
	a.proxy = &runningProxy{
		server:       httpSrv,
		listener:     ln,
		displayURL:   displayURL,
		responsesURL: target.ResponsesURL,
		startedAt:    time.Now(),
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
	a.logger.Printf("OpenAI Models route: %s/v1/models", displayURL)
	a.logger.Printf("OpenAI Responses route: %s/v1/responses", displayURL)
	a.logger.Printf("Claude Messages route: %s/v1/messages", displayURL)
	a.logger.Printf("OpenAI Chat Completions route: %s/v1/chat/completions", displayURL)
	return nil
}

func (a *app) stopProxy() error {
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

func (a *app) snapshotStatus() statusResponse {
	a.mu.Lock()
	defer a.mu.Unlock()

	resp := statusResponse{
		Running:    a.proxy != nil,
		ControlURL: a.controlAddrLocked(),
		Routes:     []routeDisplay{},
	}
	if a.proxy == nil {
		return resp
	}

	resp.ProxyBaseURL = a.proxy.displayURL
	resp.UpstreamResponses = a.proxy.responsesURL
	resp.StartedAt = a.proxy.startedAt.Format(time.RFC3339)
	resp.Routes = []routeDisplay{
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

type openAITarget struct {
	DisplayURL   string
	ModelsURL    string
	ResponsesURL string
}

func resolveOpenAITarget(raw, mode string) (openAITarget, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		baseURL, modelsURL, responsesURL, err := normalizeOpenAIHost(raw)
		if err != nil {
			return openAITarget{}, err
		}
		return openAITarget{
			DisplayURL:   baseURL,
			ModelsURL:    modelsURL,
			ResponsesURL: responsesURL,
		}, nil
	case "custom":
		exactURL, err := normalizeAbsoluteURL(raw)
		if err != nil {
			return openAITarget{}, err
		}
		return openAITarget{
			DisplayURL:   exactURL,
			ResponsesURL: exactURL,
		}, nil
	default:
		return openAITarget{}, errors.New("host_mode must be default or custom")
	}
}

func normalizeOpenAIHost(raw string) (baseURL string, modelsURL string, responsesURL string, err error) {
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

func normalizeAbsoluteURL(raw string) (string, error) {
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

func (a *app) controlAddrLocked() string {
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
