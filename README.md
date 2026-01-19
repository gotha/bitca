# BITCA

Bat Itso's Tiny Coding Agent

## Prerequisites

- Go 1.25.5 or later
- Ollama running locally with a model (e.g., `qwen2.5-coder:7b` or `llama3.1:8b`)

## Installation

```bash
# Clone the repository
git clone https://github.com/gotha/bitca
cd bitca

# Build the application
go build -o bitca

# Or run directly
go run .
```

## Usage

1. Start the application:
   ```bash
   ./bitca
   ```

2. Type your message in the input field (always at the bottom) and press Enter
3. Watch the AI response stream in real-time in the scrollable viewport
4. Use arrow keys or mouse to scroll through conversation history
5. Continue the conversation - new messages auto-scroll to bottom
6. Press Ctrl+C or Esc to quit

## Development with Nix

If you're using Nix, you can enter the development shell:

```bash
nix develop
```

This will provide you with Go and all necessary development tools.
