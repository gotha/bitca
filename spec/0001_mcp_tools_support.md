# 0001: MCP Tools Support

## Overview

Add support for Model Context Protocol (MCP) tools integration. The binary should be able to load external tool definitions from MCP servers and make them available to the LLM alongside the built-in tools.

## Configuration

### Config File Location

The MCP configuration file is located at `~/.config/mcp/mcp.json`.

### Config File Format

```json
{
  "mcpServers": {
    "server-name": {
      "command": "path/to/mcp-server",
      "args": ["--arg1", "value1"],
      "env": {
        "ENV_VAR": "value"
      }
    },
    "another-server": {
      "command": "npx",
      "args": ["-y", "@some/mcp-server"]
    }
  }
}
```

## Implementation Tasks

### 1. Create MCP Config Parser

Create a new file `mcp/config.go`:

- Define structs for parsing the MCP configuration:
  - `MCPConfig` - root config containing `mcpServers` map
  - `MCPServerConfig` - individual server with `command`, `args`, `env`
- Implement `LoadMCPConfig()` function that:
  - Expands `~` to user home directory
  - Reads `~/.config/mcp/mcp.json`
  - Returns parsed config or empty config if file doesn't exist
  - Returns error only on parse failures, not on missing file

### 2. Create MCP Client

Create a new file `mcp/client.go`:

- Implement `MCPClient` struct to manage communication with MCP servers
- Use JSON-RPC over stdio (stdin/stdout) to communicate with MCP servers
- Implement `StartServer()` method to:
  - Start the MCP server process with configured command, args, and env
  - Establish JSON-RPC communication
- Implement `Initialize()` method to:
  - Send `initialize` request with protocol version and capabilities
  - Wait for `initialized` notification
- Implement `ListTools()` method to:
  - Send `tools/list` request
  - Parse and return tool definitions
- Implement `CallTool()` method to:
  - Send `tools/call` request with tool name and arguments
  - Return tool execution result
- Implement `Close()` method to gracefully shutdown server

### 3. Create MCP Tool Adapter

Create a new file `mcp/tools.go`:

- Implement `ConvertToOllamaTools()` function to:
  - Convert MCP tool definitions to Ollama `api.Tool` format
  - Map MCP JSON Schema properties to Ollama `ToolProperty`
  - Preserve tool name, description, and parameter schemas
- Implement `MCPToolExecutor` struct to:
  - Hold references to active MCP clients
  - Execute tool calls by routing to appropriate MCP server

### 4. Modify Application Startup

Update `main.go` and `chat.go`:

- In `initialModel()` or a new init function:
  1. Load MCP config from `~/.config/mcp/mcp.json`
  2. For each configured server:
     - Start the MCP server process
     - Send `initialize` request
     - Call `tools/list` to get available tools
  3. Convert all MCP tools to Ollama format
  4. Merge MCP tools with built-in tools from `defineTools()`
  5. Store MCP clients for later tool execution

### 5. Update Tool Execution

Update `tools.go`:

- Modify `executeTool()` to:
  - Check if tool is a built-in tool, execute locally
  - If not built-in, route to appropriate MCP client's `CallTool()`
- Handle MCP tool responses and convert to expected format

## MCP Protocol Details

### JSON-RPC Messages

All communication uses JSON-RPC 2.0 over stdio.

#### Initialize Request
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "bitca",
      "version": "1.0.0"
    }
  }
}
```

#### List Tools Request
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list"
}
```

#### List Tools Response
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "tool_name",
        "description": "Tool description",
        "inputSchema": {
          "type": "object",
          "properties": {
            "param1": {
              "type": "string",
              "description": "Parameter description"
            }
          },
          "required": ["param1"]
        }
      }
    ]
  }
}
```

#### Call Tool Request
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "tool_name",
    "arguments": {
      "param1": "value1"
    }
  }
}
```

## Error Handling

- If config file doesn't exist, proceed without MCP tools (no error)
- If a server fails to start, log warning and continue with other servers
- If `tools/list` fails, skip that server's tools
- If tool execution fails, return error message to LLM

## Testing

- Test config parsing with various valid/invalid configs
- Test MCP client communication with mock server
- Test tool conversion to Ollama format
- Integration test with a real MCP server (e.g., filesystem MCP server)

