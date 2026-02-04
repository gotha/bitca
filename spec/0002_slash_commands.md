# 0002: Slash Commands

## Overview

Add support for slash commands in the chat input field. Slash commands allow users to execute special actions without sending the text to the LLM. Commands are prefixed with `/` but not all text starting with `/` is treated as a command - only recognized command names are intercepted.

## Command Syntax

```
/<command_name> [arguments...]
```

- Commands start with `/` followed immediately by the command name (no space)
- Command names are case-insensitive
- Arguments are optional and space-separated
- Unrecognized commands starting with `/` are sent to the LLM as regular messages

## Registered Commands

### `/mcp`

**Description:** List all registered MCP servers and their status.

**Usage:**
```
/mcp
```

**Output:** Displays a list of all loaded MCP servers with:
- Server name
- Transport type (stdio, http, sse)
- Number of tools loaded
- Tool names (abbreviated if many)

**Example output:**
```
MCP Servers:
  • github (stdio) - 15 tools
  • memory (stdio) - 3 tools
  • playwright (stdio) - 12 tools
```

## Implementation Tasks

### 1. Create Command Registry

Create a new file `commands.go`:

- Define `Command` struct with:
  - `Name` - command name without the `/` prefix
  - `Description` - help text for the command
  - `Handler` - function to execute the command
- Define `CommandRegistry` to store registered commands
- Implement `RegisterCommand()` to add commands
- Implement `GetCommand()` to look up commands by name
- Implement `IsCommand()` to check if input is a recognized command

### 2. Implement Command Parser

In `commands.go`:

- Implement `ParseCommand()` function that:
  - Takes user input string
  - Returns command name and arguments if input is a valid command
  - Returns empty string if input is not a command
- Command matching should be case-insensitive
- Only exact command name matches are treated as commands

### 3. Implement `/mcp` Command

In `commands.go` or a separate `cmd_mcp.go`:

- Create handler function for `/mcp` command
- Handler receives the MCP Manager instance
- Returns formatted string listing all MCP servers:
  - Server name
  - Transport type
  - Tool count
  - Tool names (first 5, then "..." if more)

### 4. Integrate Commands into Chat

Update `chat.go`:

- In the `KeyEnter` handler, before sending to LLM:
  1. Check if input is a recognized command using `IsCommand()`
  2. If yes, execute the command handler
  3. Display command output in conversation
  4. Do NOT send to LLM
  5. If no, proceed with normal LLM message flow
- Pass necessary context (like `mcpManager`) to command handlers

### 5. Add Command Help

- Implement `/help` command that lists all available commands
- Each command should have a description for the help output

## Command Handler Signature

```go
type CommandHandler func(ctx CommandContext, args []string) (string, error)

type CommandContext struct {
    MCPManager *mcp.Manager
    // Add other context as needed
}
```

## Display Format

Command output should be displayed differently from assistant messages:
- Use a distinct style (e.g., different color or prefix)
- Show as "System:" or similar label
- Do not add to LLM message history (commands are local-only)

## Error Handling

- If a command fails, display error message to user
- Errors should not crash the application
- Invalid arguments should show usage help for that command

## Future Commands (Not in Scope)

These commands may be added later:
- `/help` - List all available commands
- `/clear` - Clear conversation history
- `/model` - Show or switch the current model
- `/tools` - List all available tools (built-in + MCP)
- `/quit` - Exit the application

