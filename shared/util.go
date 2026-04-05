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
