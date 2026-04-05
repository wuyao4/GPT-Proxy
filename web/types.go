package main

import (
	"encoding/json"
	"net/http"
	"time"
)

const defaultResponsesURL = "https://api.openai.com/v1/responses"

type config struct {
	ListenAddr   string
	ModelsURL    string
	ResponsesURL string
	APIKey       string
	Timeout      time.Duration
}

type server struct {
	cfg    config
	client *http.Client
	logger *logHub
}

type claudeMessagesRequest struct {
	Model         string               `json:"model"`
	Messages      []claudeInputMessage `json:"messages"`
	System        json.RawMessage      `json:"system,omitempty"`
	MaxTokens     int                  `json:"max_tokens"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	Metadata      map[string]any       `json:"metadata,omitempty"`
}

type claudeInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeMessagesResponse struct {
	ID           string                   `json:"id,omitempty"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Content      []claudeTextContentBlock `json:"content"`
	Model        string                   `json:"model"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence *string                  `json:"stop_sequence"`
	Usage        *claudeMessageUsage      `json:"usage,omitempty"`
}

type claudeTextContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeMessageUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeMessageStartEvent struct {
	Type    string                 `json:"type"`
	Message claudeMessagesResponse `json:"message"`
}

type claudeContentBlockStartEvent struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index"`
	ContentBlock claudeTextContentBlock `json:"content_block"`
}

type claudeContentBlockDeltaEvent struct {
	Type  string                  `json:"type"`
	Index int                     `json:"index"`
	Delta claudeTextDeltaFragment `json:"delta"`
}

type claudeTextDeltaFragment struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type claudeMessageDeltaEvent struct {
	Type  string              `json:"type"`
	Delta claudeMessageDelta  `json:"delta"`
	Usage *claudeMessageUsage `json:"usage,omitempty"`
}

type claudeMessageDelta struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

type claudeMessageStopEvent struct {
	Type string `json:"type"`
}

type claudeErrorEvent struct {
	Type  string           `json:"type"`
	Error claudeErrorInner `json:"error"`
}

type claudeErrorInner struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type openAIChatCompletionsRequest struct {
	Model       string                   `json:"model"`
	Messages    []openAIChatInputMessage `json:"messages"`
	MaxTokens   int                      `json:"max_tokens,omitempty"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
	Stop        json.RawMessage          `json:"stop,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
}

type openAIChatInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionsResponse struct {
	ID      string                       `json:"id"`
	Object  string                       `json:"object"`
	Created int64                        `json:"created"`
	Model   string                       `json:"model"`
	Choices []openAIChatCompletionChoice `json:"choices"`
	Usage   *openAIUsage                 `json:"usage,omitempty"`
}

type openAIChatCompletionChoice struct {
	Index        int                     `json:"index"`
	Message      openAIChatOutputMessage `json:"message"`
	FinishReason string                  `json:"finish_reason"`
	Logprobs     any                     `json:"logprobs,omitempty"`
}

type openAIChatOutputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionChunkResponse struct {
	ID      string                            `json:"id"`
	Object  string                            `json:"object"`
	Created int64                             `json:"created"`
	Model   string                            `json:"model"`
	Choices []openAIChatCompletionChunkChoice `json:"choices"`
}

type openAIChatCompletionChunkChoice struct {
	Index        int             `json:"index"`
	Delta        openAIChatDelta `json:"delta"`
	FinishReason string          `json:"finish_reason"`
	Logprobs     any             `json:"logprobs,omitempty"`
}

type openAIChatDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type openAIResponsesRequest struct {
	Model           string   `json:"model"`
	Instructions    string   `json:"instructions,omitempty"`
	Input           any      `json:"input"`
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	Stream          bool     `json:"stream,omitempty"`
}

type openAIResponsesResponse struct {
	ID                string                  `json:"id"`
	Model             string                  `json:"model"`
	CreatedAt         int64                   `json:"created_at"`
	Status            string                  `json:"status"`
	Output            []openAIResponseOutput  `json:"output"`
	IncompleteDetails *openAIIncompleteDetail `json:"incomplete_details,omitempty"`
	Usage             *openAIUsage            `json:"usage,omitempty"`
}

type openAIResponseOutput struct {
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Content []openAIResponseContent `json:"content"`
}

type openAIResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIIncompleteDetail struct {
	Reason string `json:"reason"`
}

type openAIUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type openAIResponsesTextDeltaEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
}

type streamState struct {
	ID        string
	Model     string
	CreatedAt int64
}

type sseEvent struct {
	Event string
	Data  string
}

type openAIResponsesRequestMeta struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}
