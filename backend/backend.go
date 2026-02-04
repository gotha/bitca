package backend

import "context"

// Message represents a chat message
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string // For tool response messages (required by OpenAI)
}

// ToolCall represents a tool invocation request
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}

// Tool represents a tool definition
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{} // JSON Schema
}

// StreamChunk represents a streaming response chunk
type StreamChunk struct {
	Content   string
	ToolCalls []ToolCall
	Done      bool
}

// Backend defines the interface for LLM backends
type Backend interface {
	// Name returns the backend name (e.g., "ollama", "openai")
	Name() string

	// Chat sends a chat request and returns the response via callback
	// The callback is called for each streaming chunk
	Chat(ctx context.Context, model string, messages []Message, tools []Tool, stream bool,
		callback func(StreamChunk) error) error

	// Close cleans up any resources
	Close() error
}

