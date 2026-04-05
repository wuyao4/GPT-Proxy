package proxyshared

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *Server) streamClaudeMessages(w http.ResponseWriter, incoming *http.Request, payload openAIResponsesRequest, fallbackModel string, stopSequences []string) error {
	upstream, statusCode, err := s.forwardResponsesStream(incoming, payload)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return nil
	}
	defer upstream.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by the server")
		return nil
	}

	initSSEHeaders(w)
	filter := newStopSequenceFilter(stopSequences)
	state := streamState{Model: fallbackModel}
	messageStarted := false
	contentStarted := false
	completed := false

	err = consumeSSE(upstream.Body, func(evt sseEvent) error {
		eventType, err := detectResponsesStreamEventType(evt)
		if err != nil || eventType == "" {
			return err
		}

		switch eventType {
		case "response.created":
			var created openAIResponsesResponse
			if err := json.Unmarshal([]byte(evt.Data), &created); err != nil {
				return err
			}
			state.ID = created.ID
			state.Model = coalesce(created.Model, state.Model, fallbackModel)
			if !messageStarted {
				if err := writeSSEEvent(w, flusher, "message_start", claudeMessageStartEvent{
					Type: "message_start",
					Message: claudeMessagesResponse{
						ID:      state.ID,
						Type:    "message",
						Role:    "assistant",
						Content: []claudeTextContentBlock{},
						Model:   state.Model,
						Usage:   &claudeMessageUsage{},
					},
				}); err != nil {
					return err
				}
				messageStarted = true
			}
		case "response.output_text.delta":
			if !messageStarted {
				if err := writeSSEEvent(w, flusher, "message_start", claudeMessageStartEvent{
					Type: "message_start",
					Message: claudeMessagesResponse{
						ID:      state.ID,
						Type:    "message",
						Role:    "assistant",
						Content: []claudeTextContentBlock{},
						Model:   coalesce(state.Model, fallbackModel),
						Usage:   &claudeMessageUsage{},
					},
				}); err != nil {
					return err
				}
				messageStarted = true
			}
			if !contentStarted {
				if err := writeSSEEvent(w, flusher, "content_block_start", claudeContentBlockStartEvent{
					Type:  "content_block_start",
					Index: 0,
					ContentBlock: claudeTextContentBlock{
						Type: "text",
						Text: "",
					},
				}); err != nil {
					return err
				}
				contentStarted = true
			}

			var delta openAIResponsesTextDeltaEvent
			if err := json.Unmarshal([]byte(evt.Data), &delta); err != nil {
				return err
			}
			emitted, _, _ := filter.Push(delta.Delta)
			if emitted != "" {
				if err := writeSSEEvent(w, flusher, "content_block_delta", claudeContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: claudeTextDeltaFragment{
						Type: "text_delta",
						Text: emitted,
					},
				}); err != nil {
					return err
				}
			}
		case "response.completed":
			var done openAIResponsesResponse
			if err := json.Unmarshal([]byte(evt.Data), &done); err != nil {
				return err
			}
			state.ID = coalesce(done.ID, state.ID)
			state.Model = coalesce(done.Model, state.Model, fallbackModel)

			finalText := filter.Flush()
			if finalText != "" {
				if !messageStarted {
					if err := writeSSEEvent(w, flusher, "message_start", claudeMessageStartEvent{
						Type: "message_start",
						Message: claudeMessagesResponse{
							ID:      state.ID,
							Type:    "message",
							Role:    "assistant",
							Content: []claudeTextContentBlock{},
							Model:   state.Model,
							Usage:   &claudeMessageUsage{},
						},
					}); err != nil {
						return err
					}
					messageStarted = true
				}
				if !contentStarted {
					if err := writeSSEEvent(w, flusher, "content_block_start", claudeContentBlockStartEvent{
						Type:  "content_block_start",
						Index: 0,
						ContentBlock: claudeTextContentBlock{
							Type: "text",
							Text: "",
						},
					}); err != nil {
						return err
					}
					contentStarted = true
				}
				if err := writeSSEEvent(w, flusher, "content_block_delta", claudeContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: claudeTextDeltaFragment{
						Type: "text_delta",
						Text: finalText,
					},
				}); err != nil {
					return err
				}
			}

			if contentStarted {
				if err := writeSSEEvent(w, flusher, "content_block_stop", claudeContentBlockStopEvent{
					Type:  "content_block_stop",
					Index: 0,
				}); err != nil {
					return err
				}
			}

			stopReason, stopSequence := toClaudeStopReason(done, filter.Matched())
			if err := writeSSEEvent(w, flusher, "message_delta", claudeMessageDeltaEvent{
				Type: "message_delta",
				Delta: claudeMessageDelta{
					StopReason:   stopReason,
					StopSequence: stopSequence,
				},
				Usage: toClaudeUsage(done.Usage),
			}); err != nil {
				return err
			}
			if err := writeSSEEvent(w, flusher, "message_stop", claudeMessageStopEvent{Type: "message_stop"}); err != nil {
				return err
			}
			completed = true
		case "error", "response.failed":
			return writeSSEEvent(w, flusher, "error", claudeErrorEvent{
				Type: "error",
				Error: claudeErrorInner{
					Type:    "api_error",
					Message: evt.Data,
				},
			})
		}
		return nil
	})

	if err != nil && !completed {
		_ = writeSSEEvent(w, flusher, "error", claudeErrorEvent{
			Type: "error",
			Error: claudeErrorInner{
				Type:    "api_error",
				Message: err.Error(),
			},
		})
	}
	return err
}

func (s *Server) streamOpenAIChatCompletions(w http.ResponseWriter, incoming *http.Request, payload openAIResponsesRequest, fallbackModel string, stopSequences []string) error {
	upstream, statusCode, err := s.forwardResponsesStream(incoming, payload)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return nil
	}
	defer upstream.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by the server")
		return nil
	}

	initSSEHeaders(w)
	filter := newStopSequenceFilter(stopSequences)
	state := streamState{Model: fallbackModel}
	completed := false
	roleChunkSent := false

	err = consumeSSE(upstream.Body, func(evt sseEvent) error {
		eventType, err := detectResponsesStreamEventType(evt)
		if err != nil || eventType == "" {
			return err
		}

		switch eventType {
		case "response.created":
			var created openAIResponsesResponse
			if err := json.Unmarshal([]byte(evt.Data), &created); err != nil {
				return err
			}
			state.ID = created.ID
			state.Model = coalesce(created.Model, state.Model, fallbackModel)
			state.CreatedAt = createdAtOrNow(created.CreatedAt)
			if !roleChunkSent {
				if err := writeChatCompletionChunk(w, flusher, openAIChatCompletionChunkResponse{
					ID:      state.ID,
					Object:  "chat.completion.chunk",
					Created: createdAtOrNow(state.CreatedAt),
					Model:   coalesce(state.Model, fallbackModel),
					Choices: []openAIChatCompletionChunkChoice{{
						Index: 0,
						Delta: openAIChatDelta{
							Role: "assistant",
						},
						FinishReason: "",
					}},
				}); err != nil {
					return err
				}
				roleChunkSent = true
			}
		case "response.output_text.delta":
			var delta openAIResponsesTextDeltaEvent
			if err := json.Unmarshal([]byte(evt.Data), &delta); err != nil {
				return err
			}
			if !roleChunkSent {
				if err := writeChatCompletionChunk(w, flusher, openAIChatCompletionChunkResponse{
					ID:      state.ID,
					Object:  "chat.completion.chunk",
					Created: createdAtOrNow(state.CreatedAt),
					Model:   coalesce(state.Model, fallbackModel),
					Choices: []openAIChatCompletionChunkChoice{{
						Index: 0,
						Delta: openAIChatDelta{
							Role: "assistant",
						},
						FinishReason: "",
					}},
				}); err != nil {
					return err
				}
				roleChunkSent = true
			}
			emitted, _, _ := filter.Push(delta.Delta)
			if emitted != "" {
				if err := writeChatCompletionChunk(w, flusher, openAIChatCompletionChunkResponse{
					ID:      state.ID,
					Object:  "chat.completion.chunk",
					Created: createdAtOrNow(state.CreatedAt),
					Model:   coalesce(state.Model, fallbackModel),
					Choices: []openAIChatCompletionChunkChoice{{
						Index: 0,
						Delta: openAIChatDelta{
							Content: emitted,
						},
						FinishReason: "",
					}},
				}); err != nil {
					return err
				}
			}
		case "response.completed":
			var done openAIResponsesResponse
			if err := json.Unmarshal([]byte(evt.Data), &done); err != nil {
				return err
			}
			state.ID = coalesce(done.ID, state.ID)
			state.Model = coalesce(done.Model, state.Model, fallbackModel)
			state.CreatedAt = createdAtOrNow(done.CreatedAt)

			finalText := filter.Flush()
			if finalText != "" {
				if err := writeChatCompletionChunk(w, flusher, openAIChatCompletionChunkResponse{
					ID:      state.ID,
					Object:  "chat.completion.chunk",
					Created: state.CreatedAt,
					Model:   state.Model,
					Choices: []openAIChatCompletionChunkChoice{{
						Index: 0,
						Delta: openAIChatDelta{
							Content: finalText,
						},
						FinishReason: "",
					}},
				}); err != nil {
					return err
				}
			}

			if err := writeChatCompletionChunk(w, flusher, openAIChatCompletionChunkResponse{
				ID:      state.ID,
				Object:  "chat.completion.chunk",
				Created: state.CreatedAt,
				Model:   state.Model,
				Choices: []openAIChatCompletionChunkChoice{{
					Index:        0,
					Delta:        openAIChatDelta{},
					FinishReason: toCompletionFinishReason(done, filter.Matched()),
				}},
			}); err != nil {
				return err
			}
			if err := writeSSEDataRaw(w, flusher, "[DONE]"); err != nil {
				return err
			}
			completed = true
		}
		return nil
	})

	if err != nil && !completed {
		_ = writeSSEDataRaw(w, flusher, "[DONE]")
	}
	return err
}

func detectResponsesStreamEventType(evt sseEvent) (string, error) {
	if evt.Event != "" {
		return evt.Event, nil
	}
	if evt.Data == "" || evt.Data == "[DONE]" {
		return "", nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(evt.Data), &envelope); err != nil {
		return "", err
	}
	return envelope.Type, nil
}

func consumeSSE(r io.Reader, onEvent func(sseEvent) error) error {
	reader := bufio.NewReader(r)
	var current sseEvent
	var dataLines []string

	flushCurrent := func() error {
		if current.Event == "" && len(dataLines) == 0 {
			return nil
		}
		current.Data = strings.Join(dataLines, "\n")
		err := onEvent(current)
		current = sseEvent{}
		dataLines = nil
		return err
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if flushErr := flushCurrent(); flushErr != nil {
				return flushErr
			}
		case strings.HasPrefix(line, "event:"):
			current.Event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
		}

		if errors.Is(err, io.EOF) {
			return flushCurrent()
		}
	}
}

func initSSEHeaders(w http.ResponseWriter) {
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
}

func writeSSEEvent(w io.Writer, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEDataRaw(w io.Writer, flusher http.Flusher, data string) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeChatCompletionChunk(w io.Writer, flusher http.Flusher, payload openAIChatCompletionChunkResponse) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSEDataRaw(w, flusher, string(data))
}
