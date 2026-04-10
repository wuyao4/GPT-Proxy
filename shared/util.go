package proxyshared

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func promptToInput(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.Join(list, "\n"), nil
	}

	var generic []any
	if err := json.Unmarshal(raw, &generic); err == nil {
		parts := make([]string, 0, len(generic))
		for _, item := range generic {
			switch value := item.(type) {
			case string:
				parts = append(parts, value)
			case float64:
				parts = append(parts, strconv.FormatFloat(value, 'f', -1, 64))
			default:
				return nil, errors.New("prompt array only supports strings or numbers")
			}
		}
		return strings.Join(parts, "\n"), nil
	}

	return nil, errors.New("prompt must be a string or array")
}

func normalizeUpstreamProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", upstreamProtocolResponses:
		return upstreamProtocolResponses
	case upstreamProtocolChatCompletions:
		return upstreamProtocolChatCompletions
	default:
		return ""
	}
}

func normalizeChatCompletionsUpstreamBody(body []byte) ([]byte, openAIChatCompletionsRequestMeta, error) {
	meta := openAIChatCompletionsRequestMeta{}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, meta, errors.New("request body is required")
	}

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, meta, err
	}

	if model, ok := payload["model"].(string); ok {
		meta.Model = strings.TrimSpace(model)
	}
	if stream, ok := payload["stream"].(bool); ok {
		meta.Stream = stream
	}

	payload["stream"] = true
	streamOptions, ok := payload["stream_options"].(map[string]any)
	if !ok || streamOptions == nil {
		streamOptions = map[string]any{}
	}
	streamOptions["include_usage"] = true
	payload["stream_options"] = streamOptions

	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, meta, err
	}
	return normalized, meta, nil
}

func chatMessagesToResponsesInput(messages []openAIChatInputMessage) ([]map[string]any, error) {
	input := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.Role) == "" {
			return nil, errors.New("message role is required")
		}
		input = append(input, map[string]any{
			"type":    "message",
			"role":    message.Role,
			"content": message.Content,
		})
	}
	return input, nil
}

func responsesRequestToChatCompletions(req openAIResponsesBridgeRequest) (openAIChatCompletionsRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		return openAIChatCompletionsRequest{}, errors.New("model is required")
	}

	instructions, err := responsesInstructionsToString(req.Instructions)
	if err != nil {
		return openAIChatCompletionsRequest{}, err
	}

	messages, err := responsesInputToChatMessages(req.Input)
	if err != nil {
		return openAIChatCompletionsRequest{}, err
	}
	if instructions != "" {
		messages = append([]openAIChatInputMessage{{
			Role:    "system",
			Content: instructions,
		}}, messages...)
	}

	return openAIChatCompletionsRequest{
		Model:       strings.TrimSpace(req.Model),
		Messages:    messages,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}, nil
}

func responsesRequestPayloadToChatCompletions(req openAIResponsesRequest) (openAIChatCompletionsRequest, error) {
	inputRaw, err := json.Marshal(req.Input)
	if err != nil {
		return openAIChatCompletionsRequest{}, fmt.Errorf("marshal input: %w", err)
	}

	var instructionsRaw json.RawMessage
	if strings.TrimSpace(req.Instructions) != "" {
		instructionsRaw, err = json.Marshal(req.Instructions)
		if err != nil {
			return openAIChatCompletionsRequest{}, fmt.Errorf("marshal instructions: %w", err)
		}
	}

	return responsesRequestToChatCompletions(openAIResponsesBridgeRequest{
		Model:           req.Model,
		Instructions:    instructionsRaw,
		Input:           json.RawMessage(inputRaw),
		MaxOutputTokens: req.MaxOutputTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          req.Stream,
	})
}

func forceStreamingChatCompletionsRequest(req openAIChatCompletionsRequest) openAIChatCompletionsRequest {
	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &openAIChatStreamOptions{}
	}
	req.StreamOptions.IncludeUsage = true
	return req
}

func responsesInstructionsToString(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text, nil
	}

	return "", errors.New("instructions must be a string")
}

func responsesInputToChatMessages(raw json.RawMessage) ([]openAIChatInputMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, errors.New("input is required")
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("input string must not be empty")
		}
		return []openAIChatInputMessage{{
			Role:    "user",
			Content: text,
		}}, nil
	}

	var messages []responsesInputMessage
	if err := json.Unmarshal(trimmed, &messages); err == nil {
		out := make([]openAIChatInputMessage, 0, len(messages))
		for idx, message := range messages {
			if strings.TrimSpace(message.Type) != "" && message.Type != "message" {
				return nil, fmt.Errorf("input[%d] type %q is not supported", idx, message.Type)
			}
			if strings.TrimSpace(message.Role) == "" {
				return nil, fmt.Errorf("input[%d] role is required", idx)
			}
			content, err := responsesMessageContentToString(message.Content)
			if err != nil {
				return nil, fmt.Errorf("input[%d]: %w", idx, err)
			}
			out = append(out, openAIChatInputMessage{
				Role:    message.Role,
				Content: content,
			})
		}
		if len(out) == 0 {
			return nil, errors.New("input must contain at least one message")
		}
		return out, nil
	}

	return nil, errors.New("input must be a string or an array of message objects")
}

type responsesInputMessage struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type responsesInputContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func responsesMessageContentToString(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", errors.New("content is required")
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text, nil
	}

	var blocks []responsesInputContentBlock
	if err := json.Unmarshal(trimmed, &blocks); err == nil {
		var builder strings.Builder
		for idx, block := range blocks {
			switch block.Type {
			case "input_text", "output_text", "text":
				builder.WriteString(block.Text)
			default:
				return "", fmt.Errorf("content[%d] type %q is not supported", idx, block.Type)
			}
		}
		return builder.String(), nil
	}

	return "", errors.New("content must be a string or a text content array")
}

func chatCompletionsToResponses(resp openAIChatCompletionsResponse) openAIResponsesResponse {
	text, finishReason := extractChatCompletionText(resp)
	status, incomplete := responseStatusFromCompletionFinishReason(finishReason)

	output := []openAIResponseOutput{}
	if text != "" {
		output = []openAIResponseOutput{{
			Type: "message",
			Role: "assistant",
			Content: []openAIResponseContent{{
				Type: "output_text",
				Text: text,
			}},
		}}
	}

	return openAIResponsesResponse{
		Object:            "response",
		ID:                resp.ID,
		Model:             resp.Model,
		CreatedAt:         resp.Created,
		Status:            status,
		Output:            output,
		IncompleteDetails: incomplete,
		Usage:             chatUsageToResponsesUsage(resp.Usage),
	}
}

func extractChatCompletionText(resp openAIChatCompletionsResponse) (string, string) {
	if len(resp.Choices) == 0 {
		return "", ""
	}
	choice := resp.Choices[0]
	return choice.Message.Content, choice.FinishReason
}

func responseStatusFromCompletionFinishReason(reason string) (string, *openAIIncompleteDetail) {
	switch strings.TrimSpace(reason) {
	case "length":
		return "incomplete", &openAIIncompleteDetail{Reason: "max_output_tokens"}
	default:
		return "completed", nil
	}
}

func chatUsageToResponsesUsage(usage *openAIChatUsage) *openAIUsage {
	if usage == nil {
		return nil
	}
	return &openAIUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
	}
}

func responsesUsageToChatUsage(usage *openAIUsage) *openAIChatUsage {
	if usage == nil {
		return nil
	}
	return &openAIChatUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func aggregateChatCompletionsStream(r io.Reader, fallbackModel string) (openAIChatCompletionsResponse, error) {
	state := streamState{Model: fallbackModel}
	role := ""
	finishReason := ""
	var text strings.Builder
	var usage *openAIChatUsage
	sawChunk := false

	err := consumeSSE(r, func(evt sseEvent) error {
		if evt.Data == "" || evt.Data == "[DONE]" {
			return nil
		}

		var chunk openAIChatCompletionChunkResponse
		if err := json.Unmarshal([]byte(evt.Data), &chunk); err != nil {
			return err
		}
		sawChunk = true
		state.ID = coalesce(chunk.ID, state.ID)
		state.Model = coalesce(chunk.Model, state.Model, fallbackModel)
		state.CreatedAt = createdAtOrNow(chunk.Created)

		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			return nil
		}

		choice := chunk.Choices[0]
		if choice.Delta.Role != "" {
			role = choice.Delta.Role
		}
		if choice.Delta.Content != "" {
			text.WriteString(choice.Delta.Content)
		}
		if strings.TrimSpace(choice.FinishReason) != "" {
			finishReason = choice.FinishReason
		}
		return nil
	})
	if err != nil {
		return openAIChatCompletionsResponse{}, err
	}
	if !sawChunk {
		return openAIChatCompletionsResponse{}, errors.New("chat completions stream produced no chunks")
	}

	return openAIChatCompletionsResponse{
		ID:      state.ID,
		Object:  "chat.completion",
		Created: createdAtOrNow(state.CreatedAt),
		Model:   coalesce(state.Model, fallbackModel),
		Choices: []openAIChatCompletionChoice{{
			Index: 0,
			Message: openAIChatOutputMessage{
				Role:    coalesce(role, "assistant"),
				Content: text.String(),
			},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}, nil
}

func parseStopSequences(raw json.RawMessage) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}

	return nil, errors.New("stop must be a string or array of strings")
}

func extractOutputText(resp openAIResponsesResponse) string {
	var builder strings.Builder
	for _, output := range resp.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" {
				builder.WriteString(content.Text)
			}
		}
	}
	return builder.String()
}

func buildClaudeContent(text string) []claudeTextContentBlock {
	if text == "" {
		return []claudeTextContentBlock{}
	}
	return []claudeTextContentBlock{{Type: "text", Text: text}}
}

func toClaudeUsage(usage *openAIUsage) *claudeMessageUsage {
	if usage == nil {
		return nil
	}
	return &claudeMessageUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
	}
}

func toClaudeStopReason(resp openAIResponsesResponse, matchedStop string) (string, *string) {
	if matchedStop != "" {
		return "stop_sequence", stringPtr(matchedStop)
	}
	if resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason == "max_output_tokens" {
		return "max_tokens", nil
	}
	return "end_turn", nil
}

func toCompletionFinishReason(resp openAIResponsesResponse, matchedStop string) string {
	if matchedStop != "" {
		return "stop"
	}
	if resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason == "max_output_tokens" {
		return "length"
	}
	return "stop"
}

func applyStopSequences(text string, stopSequences []string) (string, string) {
	if len(stopSequences) == 0 || text == "" {
		return text, ""
	}
	matchIndex, matchSequence := findFirstStop(text, stopSequences)
	if matchIndex == -1 {
		return text, ""
	}
	return text[:matchIndex], matchSequence
}

func newStopSequenceFilter(stops []string) *stopSequenceFilter {
	filtered := make([]string, 0, len(stops))
	maxStopRunes := 0
	for _, stop := range stops {
		if stop == "" {
			continue
		}
		filtered = append(filtered, stop)
		if n := len([]rune(stop)); n > maxStopRunes {
			maxStopRunes = n
		}
	}
	if maxStopRunes > 0 {
		maxStopRunes--
	}
	return &stopSequenceFilter{stops: filtered, maxStopRunes: maxStopRunes}
}

func (f *stopSequenceFilter) Push(delta string) (string, bool, string) {
	if f.done || delta == "" {
		return "", f.done, f.matched
	}

	combined := f.pending + delta
	idx, matched := findFirstStop(combined, f.stops)
	if idx >= 0 {
		f.done = true
		f.matched = matched
		f.pending = ""
		return combined[:idx], true, matched
	}

	if f.maxStopRunes == 0 {
		return combined, false, ""
	}

	combinedRunes := []rune(combined)
	if len(combinedRunes) <= f.maxStopRunes {
		f.pending = combined
		return "", false, ""
	}

	emitRunes := combinedRunes[:len(combinedRunes)-f.maxStopRunes]
	f.pending = string(combinedRunes[len(combinedRunes)-f.maxStopRunes:])
	return string(emitRunes), false, ""
}

func (f *stopSequenceFilter) Flush() string {
	if f.done {
		return ""
	}
	result := f.pending
	f.pending = ""
	return result
}

func (f *stopSequenceFilter) Matched() string {
	return f.matched
}

func findFirstStop(text string, stops []string) (int, string) {
	matchIndex := -1
	match := ""
	for _, stop := range stops {
		idx := strings.Index(text, stop)
		if idx == -1 {
			continue
		}
		if matchIndex == -1 || idx < matchIndex {
			matchIndex = idx
			match = stop
		}
	}
	return matchIndex, match
}

func createdAtOrNow(createdAt int64) int64 {
	if createdAt > 0 {
		return createdAt
	}
	return time.Now().Unix()
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func decodeJSON(r io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
