# 0003: OpenAI API Support

## Overview

Add support for using OpenAI's ChatGPT API as an alternative to the local Ollama backend. Users should be able to switch between Ollama and OpenAI API either via command-line flags or in-app commands.

## Goals

1. Support OpenAI's Chat Completions API with tool calling
2. Allow users to choose between Ollama (local) and OpenAI (cloud) backends
3. Maintain feature parity for tool calling between both backends
4. Provide seamless switching between backends

## Configuration

### Command-Line Flags

```bash
# Use Ollama (default)
./bitca

# Use OpenAI API
./bitca -backend openai -openai-model gpt-4o

# Specify API key via flag (not recommended, use env var)
./bitca -backend openai -openai-api-key sk-...
```

### Environment Variables

```bash
# OpenAI API key (required for OpenAI backend)
export OPENAI_API_KEY=sk-...

# Optional: OpenAI API base URL (for Azure OpenAI or compatible APIs)
export OPENAI_API_BASE=https://api.openai.com/v1
```

### New Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-backend` | `ollama` | Backend to use: `ollama` or `openai` |
| `-openai-model` | `gpt-4o` | OpenAI model to use |
| `-openai-api-key` | (from env) | OpenAI API key (fallback to OPENAI_API_KEY env var if argument not provided) |
| `-openai-api-base` | (from env) | OpenAI API base URL |

## Implementation Tasks

### 1. Create Backend Interface

Create a new file `backend/backend.go`:

```go
package backend

import "context"

// Message represents a chat message
type Message struct {
    Role       string
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string // For tool response messages
}

// ToolCall represents a tool invocation request
type ToolCall struct {
    ID       string
    Name     string
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
    Chat(ctx context.Context, messages []Message, tools []Tool, stream bool,
         callback func(StreamChunk) error) error

    // Close cleans up any resources
    Close() error
}
```

### 2. Create Ollama Backend Adapter

Create a new file `backend/ollama.go`:

- Implement `OllamaBackend` struct that wraps the existing Ollama client
- Implement the `Backend` interface
- Convert between internal types and Ollama API types
- Preserve existing functionality

### 3. Create OpenAI Backend

Create a new file `backend/openai.go`:

- Implement `OpenAIBackend` struct
- Use OpenAI's Chat Completions API (`/v1/chat/completions`)
- Support streaming responses via Server-Sent Events
- Implement tool calling using OpenAI's function calling format
- Handle API authentication via API key

#### OpenAI API Request Format

```json
{
  "model": "gpt-4o",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string", "description": "City name"}
          },
          "required": ["location"]
        }
      }
    }
  ],
  "stream": true
}
```

#### OpenAI Tool Call Response

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_abc123",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\": \"Paris\"}"
        }
      }]
    }
  }]
}
```

### 4. Update Configuration

Update `main.go`:

- Add new command-line flags for backend selection
- Add flags for OpenAI-specific configuration
- Create appropriate backend based on flags
- Validate configuration (e.g., API key required for OpenAI)

### 5. Update Chat Application

Update `chat.go`:

- Replace direct Ollama client usage with Backend interface
- Update message handling to use internal Message type
- Update tool execution flow to work with both backends
- Handle tool call ID tracking for OpenAI (required for tool responses)

### 6. Add Backend Switching Command

Update `commands.go`:

- Add `/backend` command to show or switch backends at runtime
- Usage: `/backend` (show current) or `/backend openai` (switch)
- Validate that required configuration exists before switching

### 7. Update Help and Model Commands

- Update `/model` command to show models appropriate for current backend
- Update `/help` to include backend-related commands
- Update `-h` output to include new flags

## Tool Calling Differences

### Ollama Format
- Tool calls returned in `message.tool_calls` array
- Arguments are already parsed as `map[string]interface{}`
- No tool call ID required

### OpenAI Format
- Tool calls returned in `message.tool_calls` array
- Arguments are JSON string, need parsing
- Tool call ID required for tool response messages
- Tool responses must include `tool_call_id` field

## Streaming Differences

### Ollama
- Single callback per chunk
- Tool calls may come in final chunk

### OpenAI
- Server-Sent Events format
- Tool calls streamed incrementally (name, then arguments)
- Need to accumulate tool call data across chunks

## Error Handling

- Invalid API key: Show clear error message
- Rate limiting: Implement exponential backoff
- Network errors: Show error and allow retry
- Model not available: Suggest alternatives

## Security Considerations

- Never log or display API keys
- Prefer environment variables over command-line flags for API keys
- Warn if API key is passed via command line

## Testing

- Unit tests for OpenAI client with mock HTTP server
- Integration tests with OpenAI API (requires API key)
- Test tool calling with both backends
- Test streaming with both backends
- Test backend switching

## Future Enhancements (Out of Scope)

- Support for other OpenAI-compatible APIs (Azure, Anthropic, etc.)
- Token counting and cost estimation
- Conversation export/import
- Multiple API key management
