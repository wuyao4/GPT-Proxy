package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func newServer(cfg config) *server {
	return newServerWithLogger(cfg, nil)
}

func newServerWithLogger(cfg config, logger *logHub) *server {
	return &server{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", s.handleClaudeMessages)
	mux.HandleFunc("/v1/chat/completions", s.handleOpenAIChatCompletions)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func loadConfig() (config, error) {
	timeout := 60 * time.Second
	if raw := strings.TrimSpace(os.Getenv("HTTP_TIMEOUT_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			return config{}, fmt.Errorf("invalid HTTP_TIMEOUT_SECONDS: %w", err)
		}
		timeout = time.Duration(seconds) * time.Second
	}

	responsesURL := strings.TrimSpace(os.Getenv("OPENAI_RESPONSES_URL"))
	if responsesURL == "" {
		responsesURL = defaultResponsesURL
	}

	listenAddr := strings.TrimSpace(os.Getenv("LISTEN_ADDR"))
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	return config{
		ListenAddr:   listenAddr,
		ResponsesURL: responsesURL,
		APIKey:       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		Timeout:      timeout,
	}, nil
}

func (s *server) handleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	s.logf("proxy request %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req claudeMessagesRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	input, err := claudeMessagesToResponsesInput(req.System, req.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	instructions, err := claudeSystemToInstructions(req.System)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	responsesReq := openAIResponsesRequest{
		Model:           req.Model,
		Instructions:    instructions,
		Input:           input,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          req.Stream,
	}

	if req.Stream {
		s.logf("streaming Claude Messages request model=%s", req.Model)
		_ = s.streamClaudeMessages(w, r, responsesReq, req.Model, req.StopSequences)
		return
	}

	responsesResp, statusCode, err := s.forwardResponses(r, responsesReq)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return
	}

	text, matchedStop := applyStopSequences(extractOutputText(responsesResp), req.StopSequences)
	stopReason, stopSequence := toClaudeStopReason(responsesResp, matchedStop)
	writeJSON(w, http.StatusOK, claudeMessagesResponse{
		ID:           responsesResp.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      buildClaudeContent(text),
		Model:        coalesce(responsesResp.Model, req.Model),
		StopReason:   stopReason,
		StopSequence: stopSequence,
		Usage:        toClaudeUsage(responsesResp.Usage),
	})
	s.logf("completed Claude Messages request model=%s", req.Model)
}

func (s *server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.logf("proxy request %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req openAIChatCompletionsRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Model) == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	input, err := chatMessagesToResponsesInput(req.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stopSequences, err := parseStopSequences(req.Stop)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	responsesReq := openAIResponsesRequest{
		Model:           req.Model,
		Input:           input,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          req.Stream,
	}

	if req.Stream {
		s.logf("streaming OpenAI Chat Completions request model=%s", req.Model)
		_ = s.streamOpenAIChatCompletions(w, r, responsesReq, req.Model, stopSequences)
		return
	}

	responsesResp, statusCode, err := s.forwardResponses(r, responsesReq)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return
	}

	text, matchedStop := applyStopSequences(extractOutputText(responsesResp), stopSequences)
	writeJSON(w, http.StatusOK, openAIChatCompletionsResponse{
		ID:      responsesResp.ID,
		Object:  "chat.completion",
		Created: createdAtOrNow(responsesResp.CreatedAt),
		Model:   coalesce(responsesResp.Model, req.Model),
		Choices: []openAIChatCompletionChoice{{
			Index: 0,
			Message: openAIChatOutputMessage{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: toCompletionFinishReason(responsesResp, matchedStop),
			Logprobs:     nil,
		}},
		Usage: responsesResp.Usage,
	})
	s.logf("completed OpenAI Chat Completions request model=%s", req.Model)
}

func (s *server) forwardResponses(incoming *http.Request, payload openAIResponsesRequest) (openAIResponsesResponse, int, error) {
	resp, statusCode, err := s.doResponsesRequest(incoming, payload)
	if err != nil {
		return openAIResponsesResponse{}, statusCode, err
	}
	defer resp.Body.Close()

	var decoded openAIResponsesResponse
	if err := decodeJSON(resp.Body, &decoded); err != nil {
		return openAIResponsesResponse{}, http.StatusBadGateway, fmt.Errorf("decode upstream response: %w", err)
	}
	return decoded, http.StatusOK, nil
}

func (s *server) forwardResponsesStream(incoming *http.Request, payload openAIResponsesRequest) (*http.Response, int, error) {
	return s.doResponsesRequest(incoming, payload)
}

func (s *server) doResponsesRequest(incoming *http.Request, payload openAIResponsesRequest) (*http.Response, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("marshal upstream request: %w", err)
	}

	s.logf("forward upstream url=%s model=%s stream=%t", s.cfg.ResponsesURL, payload.Model, payload.Stream)

	req, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, s.cfg.ResponsesURL, bytes.NewReader(body))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("build upstream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if payload.Stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	if s.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	} else if auth := strings.TrimSpace(incoming.Header.Get("Authorization")); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.logf("upstream request failed: %v", err)
		return nil, http.StatusBadGateway, fmt.Errorf("request upstream responses failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			s.logf("upstream status %d read body failed: %v", resp.StatusCode, readErr)
			return nil, http.StatusBadGateway, fmt.Errorf("upstream status %d and failed to read body: %w", resp.StatusCode, readErr)
		}
		s.logf("upstream status %d model=%s stream=%t body=%s", resp.StatusCode, payload.Model, payload.Stream, strings.TrimSpace(string(raw)))
		return nil, resp.StatusCode, fmt.Errorf("upstream status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return resp, http.StatusOK, nil
}

func claudeMessagesToResponsesInput(_ json.RawMessage, messages []claudeInputMessage) ([]map[string]any, error) {
	input := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		content, err := claudeContentToResponsesString(message.Content)
		if err != nil {
			return nil, fmt.Errorf("message role %q: %w", message.Role, err)
		}
		input = append(input, map[string]any{
			"type":    "message",
			"role":    message.Role,
			"content": content,
		})
	}

	return input, nil
}

func claudeSystemToInstructions(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}
	return claudeContentToResponsesString(raw)
}

func claudeContentToResponsesString(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}

	var asBlocks []claudeTextContentBlock
	if err := json.Unmarshal(raw, &asBlocks); err == nil {
		var builder strings.Builder
		for _, block := range asBlocks {
			if block.Type != "text" {
				return "", fmt.Errorf("unsupported Claude content block type %q", block.Type)
			}
			builder.WriteString(block.Text)
		}
		return builder.String(), nil
	}

	return "", errors.New("content must be a string or an array of text blocks")
}

func (s *server) logf(format string, args ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Printf(format, args...)
}
