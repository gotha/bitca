package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIBackend implements the Backend interface for OpenAI API
type OpenAIBackend struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIBackend creates a new OpenAI backend
func NewOpenAIBackend(apiKey, baseURL string) (*OpenAIBackend, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIBackend{
		apiKey:  apiKey,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{},
	}, nil
}

// Name returns the backend name
func (o *OpenAIBackend) Name() string {
	return "openai"
}

// OpenAI API request/response types
type openAIRequest struct {
	Model    string             `json:"model"`
	Messages []openAIMessage    `json:"messages"`
	Tools    []openAITool       `json:"tools,omitempty"`
	Stream   bool               `json:"stream"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string           `json:"type"`
	Function openAIFunction   `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function openAIFunctionCall   `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIResponse struct {
	ID      string           `json:"id"`
	Choices []openAIChoice   `json:"choices"`
	Error   *openAIError     `json:"error,omitempty"`
}

type openAIChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessage  `json:"message,omitempty"`
	Delta        openAIDelta    `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason"`
}

type openAIDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []openAIDeltaToolCall `json:"tool_calls,omitempty"`
}

type openAIDeltaToolCall struct {
	Index    int                  `json:"index"`
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function openAIDeltaFunction  `json:"function,omitempty"`
}

type openAIDeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Chat sends a chat request to OpenAI
func (o *OpenAIBackend) Chat(ctx context.Context, model string, messages []Message, tools []Tool, stream bool,
	callback func(StreamChunk) error) error {

	// Convert messages to OpenAI format
	openAIMessages := make([]openAIMessage, len(messages))
	for i, msg := range messages {
		content := msg.Content
		openAIMessages[i] = openAIMessage{
			Role:       msg.Role,
			Content:    &content,
			ToolCallID: msg.ToolCallID,
		}
		// Convert tool calls
		if len(msg.ToolCalls) > 0 {
			openAIMessages[i].Content = nil // OpenAI requires null content when tool_calls present
			openAIMessages[i].ToolCalls = make([]openAIToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				openAIMessages[i].ToolCalls[j] = openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIFunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				}
			}
		}
	}

	// Convert tools to OpenAI format
	openAITools := make([]openAITool, len(tools))
	for i, tool := range tools {
		openAITools[i] = openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}
	}

	req := openAIRequest{
		Model:    model,
		Messages: openAIMessages,
		Tools:    openAITools,
		Stream:   stream,
	}

	if len(tools) == 0 {
		req.Tools = nil
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if stream {
		return o.handleStreamingResponse(resp.Body, callback)
	}
	return o.handleNonStreamingResponse(resp.Body, callback)
}

// handleNonStreamingResponse processes a non-streaming response
func (o *OpenAIBackend) handleNonStreamingResponse(body io.Reader, callback func(StreamChunk) error) error {
	var resp openAIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("API error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		return fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	chunk := StreamChunk{
		Done: true,
	}

	if choice.Message.Content != nil {
		chunk.Content = *choice.Message.Content
	}

	// Convert tool calls
	if len(choice.Message.ToolCalls) > 0 {
		chunk.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			chunk.ToolCalls[i] = ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			}
		}
	}

	return callback(chunk)
}

// handleStreamingResponse processes an SSE streaming response
func (o *OpenAIBackend) handleStreamingResponse(body io.Reader, callback func(StreamChunk) error) error {
	scanner := bufio.NewScanner(body)

	// Accumulators for tool calls (streamed incrementally)
	toolCallAccum := make(map[int]*ToolCall)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// SSE format: "data: {...}" or "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// Finalize any accumulated tool calls
			var toolCalls []ToolCall
			for _, tc := range toolCallAccum {
				if tc.Name != "" {
					toolCalls = append(toolCalls, *tc)
				}
			}
			return callback(StreamChunk{Done: true, ToolCalls: toolCalls})
		}

		var resp openAIResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue // Skip malformed chunks
		}

		if len(resp.Choices) == 0 {
			continue
		}

		delta := resp.Choices[0].Delta
		chunk := StreamChunk{}

		if delta.Content != nil {
			chunk.Content = *delta.Content
		}

		// Handle incremental tool calls
		for _, tc := range delta.ToolCalls {
			if _, exists := toolCallAccum[tc.Index]; !exists {
				toolCallAccum[tc.Index] = &ToolCall{}
			}
			accum := toolCallAccum[tc.Index]
			if tc.ID != "" {
				accum.ID = tc.ID
			}
			if tc.Function.Name != "" {
				accum.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				// Accumulate JSON arguments string
				if accum.Arguments == nil {
					accum.Arguments = make(map[string]interface{})
					accum.Arguments["_raw"] = tc.Function.Arguments
				} else {
					raw := accum.Arguments["_raw"].(string) + tc.Function.Arguments
					accum.Arguments["_raw"] = raw
				}
			}
		}

		// Only send content chunks (tool calls finalized at end)
		if chunk.Content != "" {
			if err := callback(chunk); err != nil {
				return err
			}
		}
	}

	// Finalize tool calls with parsed arguments
	var toolCalls []ToolCall
	for _, tc := range toolCallAccum {
		if tc.Name != "" {
			// Parse accumulated JSON arguments
			if raw, ok := tc.Arguments["_raw"].(string); ok {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(raw), &args); err == nil {
					tc.Arguments = args
				}
			}
			toolCalls = append(toolCalls, *tc)
		}
	}

	if len(toolCalls) > 0 {
		return callback(StreamChunk{Done: true, ToolCalls: toolCalls})
	}

	return scanner.Err()
}

// Close cleans up resources
func (o *OpenAIBackend) Close() error {
	return nil
}
