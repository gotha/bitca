package backend

import (
	"context"

	"github.com/ollama/ollama/api"
)

// OllamaBackend wraps the Ollama API client
type OllamaBackend struct {
	client *api.Client
}

// NewOllamaBackend creates a new Ollama backend
func NewOllamaBackend() (*OllamaBackend, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	return &OllamaBackend{client: client}, nil
}

// Name returns the backend name
func (o *OllamaBackend) Name() string {
	return "ollama"
}

// Chat sends a chat request to Ollama
func (o *OllamaBackend) Chat(ctx context.Context, model string, messages []Message, tools []Tool, stream bool,
	callback func(StreamChunk) error) error {

	// Convert messages to Ollama format
	ollamaMessages := make([]api.Message, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = api.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		// Convert tool calls
		if len(msg.ToolCalls) > 0 {
			ollamaMessages[i].ToolCalls = make([]api.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				args := api.NewToolCallFunctionArguments()
				for k, v := range tc.Arguments {
					args.Set(k, v)
				}
				ollamaMessages[i].ToolCalls[j] = api.ToolCall{
					ID: tc.ID,
					Function: api.ToolCallFunction{
						Name:      tc.Name,
						Arguments: args,
					},
				}
			}
		}
	}

	// Convert tools to Ollama format
	ollamaTools := convertToolsToOllama(tools)

	req := &api.ChatRequest{
		Model:    model,
		Messages: ollamaMessages,
		Tools:    ollamaTools,
		Stream:   &stream,
	}

	return o.client.Chat(ctx, req, func(resp api.ChatResponse) error {
		chunk := StreamChunk{
			Content: resp.Message.Content,
			Done:    resp.Done,
		}

		// Convert tool calls
		if len(resp.Message.ToolCalls) > 0 {
			chunk.ToolCalls = make([]ToolCall, len(resp.Message.ToolCalls))
			for i, tc := range resp.Message.ToolCalls {
				chunk.ToolCalls[i] = ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments.ToMap(),
				}
			}
		}

		return callback(chunk)
	})
}

// Close cleans up resources
func (o *OllamaBackend) Close() error {
	return nil
}

// convertToolsToOllama converts backend tools to Ollama API format
func convertToolsToOllama(tools []Tool) api.Tools {
	ollamaTools := make(api.Tools, len(tools))

	for i, tool := range tools {
		props := api.NewToolPropertiesMap()
		var required []string

		if params, ok := tool.Parameters["properties"].(map[string]interface{}); ok {
			for name, prop := range params {
				if propMap, ok := prop.(map[string]interface{}); ok {
					tp := api.ToolProperty{}
					if t, ok := propMap["type"].(string); ok {
						tp.Type = []string{t}
					}
					if d, ok := propMap["description"].(string); ok {
						tp.Description = d
					}
					props.Set(name, tp)
				}
			}
		}

		if req, ok := tool.Parameters["required"].([]interface{}); ok {
			for _, r := range req {
				if s, ok := r.(string); ok {
					required = append(required, s)
				}
			}
		}

		ollamaTools[i] = api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: props,
					Required:   required,
				},
			},
		}
	}

	return ollamaTools
}

