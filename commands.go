package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gotha/bitca/backend"
	"github.com/gotha/bitca/mcp"
)

// CommandContext provides context for command execution
type CommandContext struct {
	MCPManager     *mcp.Manager
	Tools          []backend.Tool
	CurrentModel   string
	CurrentBackend string
	SetModel       func(string) // callback to change the model
}

// CommandHandler is the function signature for command handlers
type CommandHandler func(ctx CommandContext, args []string) (string, error)

// Command represents a slash command
type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
}

// CommandRegistry stores all registered commands
type CommandRegistry struct {
	commands map[string]Command
}

// NewCommandRegistry creates a new command registry with default commands
func NewCommandRegistry() *CommandRegistry {
	registry := &CommandRegistry{
		commands: make(map[string]Command),
	}

	// Register default commands
	registry.Register(Command{
		Name:        "mcp",
		Description: "List all registered MCP servers and their tools",
		Handler:     cmdMCP,
	})

	registry.Register(Command{
		Name:        "help",
		Description: "List all available commands",
		Handler:     cmdHelp,
	})

	registry.Register(Command{
		Name:        "tools",
		Description: "List all available tools (built-in and MCP)",
		Handler:     cmdTools,
	})

	registry.Register(Command{
		Name:        "model",
		Description: "Show or change the current model (usage: /model [model_name])",
		Handler:     cmdModel,
	})

	registry.Register(Command{
		Name:        "debug",
		Description: "Show debug information about the current state",
		Handler:     cmdDebug,
	})

	registry.Register(Command{
		Name:        "backend",
		Description: "Show current backend information",
		Handler:     cmdBackend,
	})

	return registry
}

// Register adds a command to the registry
func (r *CommandRegistry) Register(cmd Command) {
	r.commands[strings.ToLower(cmd.Name)] = cmd
}

// GetCommand returns a command by name (case-insensitive)
func (r *CommandRegistry) GetCommand(name string) (Command, bool) {
	cmd, ok := r.commands[strings.ToLower(name)]
	return cmd, ok
}

// IsCommand checks if the input is a recognized command
func (r *CommandRegistry) IsCommand(input string) bool {
	name, _ := ParseCommand(input)
	if name == "" {
		return false
	}
	_, ok := r.commands[strings.ToLower(name)]
	return ok
}

// GetAllCommands returns all registered commands sorted by name
func (r *CommandRegistry) GetAllCommands() []Command {
	var cmds []Command
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// ParseCommand parses user input and returns command name and arguments
// Returns empty string for name if input is not a command
func ParseCommand(input string) (name string, args []string) {
	input = strings.TrimSpace(input)

	// Must start with /
	if !strings.HasPrefix(input, "/") {
		return "", nil
	}

	// Remove the leading /
	input = input[1:]

	// Split into parts
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}

	name = parts[0]
	if len(parts) > 1 {
		args = parts[1:]
	}

	return name, args
}

// Execute runs a command and returns its output
func (r *CommandRegistry) Execute(input string, ctx CommandContext) (string, error) {
	name, args := ParseCommand(input)
	if name == "" {
		return "", fmt.Errorf("invalid command")
	}

	cmd, ok := r.GetCommand(name)
	if !ok {
		return "", fmt.Errorf("unknown command: /%s", name)
	}

	return cmd.Handler(ctx, args)
}

// cmdMCP handles the /mcp command
func cmdMCP(ctx CommandContext, args []string) (string, error) {
	if ctx.MCPManager == nil {
		return "No MCP manager configured", nil
	}

	servers := ctx.MCPManager.GetServers()
	if len(servers) == 0 {
		return "No MCP servers loaded", nil
	}

	var b strings.Builder
	b.WriteString("MCP Servers:\n")

	// Sort servers by name for consistent output
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	for _, server := range servers {
		b.WriteString(fmt.Sprintf("  • %s (%s) - %d tools\n", server.Name, server.Transport, server.ToolCount))

		// Show tool names (first 5, then "..." if more)
		if len(server.ToolNames) > 0 {
			toolsToShow := server.ToolNames
			suffix := ""
			if len(toolsToShow) > 5 {
				toolsToShow = toolsToShow[:5]
				suffix = fmt.Sprintf(" ... and %d more", len(server.ToolNames)-5)
			}
			b.WriteString(fmt.Sprintf("    Tools: %s%s\n", strings.Join(toolsToShow, ", "), suffix))
		}
	}

	return b.String(), nil
}

// cmdHelp handles the /help command
func cmdHelp(ctx CommandContext, args []string) (string, error) {
	// We need access to the registry to list commands
	// This is a bit of a workaround - we'll use a global registry reference
	registry := NewCommandRegistry()

	var b strings.Builder
	b.WriteString("Available Commands:\n")

	for _, cmd := range registry.GetAllCommands() {
		b.WriteString(fmt.Sprintf("  /%s - %s\n", cmd.Name, cmd.Description))
	}

	return b.String(), nil
}

// cmdTools handles the /tools command
func cmdTools(ctx CommandContext, args []string) (string, error) {
	if len(ctx.Tools) == 0 {
		return "No tools available", nil
	}

	// Group tools by source (built-in vs MCP server)
	builtInTools := []string{}
	mcpToolsByServer := make(map[string][]string)

	builtInNames := map[string]bool{
		"read": true, "write": true, "edit": true,
		"glob": true, "grep": true, "bash": true,
	}

	for _, tool := range ctx.Tools {
		name := tool.Name
		desc := tool.Description

		if builtInNames[name] {
			builtInTools = append(builtInTools, fmt.Sprintf("  • %s: %s", name, desc))
		} else if ctx.MCPManager != nil {
			serverName := ctx.MCPManager.GetToolServer(name)
			if serverName != "" {
				mcpToolsByServer[serverName] = append(mcpToolsByServer[serverName],
					fmt.Sprintf("  • %s: %s", name, desc))
			} else {
				// Unknown source
				builtInTools = append(builtInTools, fmt.Sprintf("  • %s: %s", name, desc))
			}
		} else {
			builtInTools = append(builtInTools, fmt.Sprintf("  • %s: %s", name, desc))
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Available Tools (%d total):\n\n", len(ctx.Tools)))

	// Show built-in tools first
	if len(builtInTools) > 0 {
		b.WriteString("Built-in Tools:\n")
		for _, t := range builtInTools {
			b.WriteString(t + "\n")
		}
		b.WriteString("\n")
	}

	// Show MCP tools grouped by server
	// Sort server names for consistent output
	var serverNames []string
	for name := range mcpToolsByServer {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, serverName := range serverNames {
		tools := mcpToolsByServer[serverName]
		b.WriteString(fmt.Sprintf("MCP Server '%s' (%d tools):\n", serverName, len(tools)))
		for _, t := range tools {
			b.WriteString(t + "\n")
		}
		b.WriteString("\n")
	}

	return strings.TrimSuffix(b.String(), "\n"), nil
}

// cmdModel handles the /model command
func cmdModel(ctx CommandContext, args []string) (string, error) {
	if len(args) == 0 {
		// Show current model and list recommended models
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Current model: %s\n\n", ctx.CurrentModel))
		b.WriteString("Recommended models for tool calling:\n")
		b.WriteString("  mistral-small:24b  - Best for tool calling (~14GB)\n")
		b.WriteString("  llama3.1:8b        - Good tool support (~5GB)\n")
		b.WriteString("  qwen2.5:14b        - Good reasoning (~10GB)\n")
		b.WriteString("  qwen2.5:32b        - Best reasoning (~20GB)\n")
		b.WriteString("  mistral-nemo:12b   - Fast responses (~8GB)\n")
		b.WriteString("  mistral:7b         - Lightweight (~5GB)\n")
		b.WriteString("\nUsage: /model <model_name>\n")
		b.WriteString("Example: /model llama3.1:8b")
		return b.String(), nil
	}

	newModel := args[0]
	if ctx.SetModel != nil {
		ctx.SetModel(newModel)
		return fmt.Sprintf("Model changed to: %s", newModel), nil
	}

	return "Unable to change model", nil
}

// cmdDebug handles the /debug command
func cmdDebug(ctx CommandContext, args []string) (string, error) {
	var b strings.Builder

	b.WriteString("Debug Information:\n\n")

	// Model info
	b.WriteString(fmt.Sprintf("Current Model: %s\n", ctx.CurrentModel))

	// Tools info
	b.WriteString(fmt.Sprintf("Total Tools: %d\n", len(ctx.Tools)))

	// Count built-in vs MCP tools
	builtInCount := 0
	mcpCount := 0
	builtInNames := map[string]bool{
		"read": true, "write": true, "edit": true,
		"glob": true, "grep": true, "bash": true,
	}

	for _, tool := range ctx.Tools {
		if builtInNames[tool.Name] {
			builtInCount++
		} else {
			mcpCount++
		}
	}

	b.WriteString(fmt.Sprintf("  - Built-in: %d\n", builtInCount))
	b.WriteString(fmt.Sprintf("  - MCP: %d\n", mcpCount))

	// MCP servers info
	if ctx.MCPManager != nil {
		servers := ctx.MCPManager.GetServers()
		b.WriteString(fmt.Sprintf("\nMCP Servers: %d\n", len(servers)))
		for _, s := range servers {
			b.WriteString(fmt.Sprintf("  - %s (%s): %d tools\n", s.Name, s.Transport, s.ToolCount))
		}
	} else {
		b.WriteString("\nMCP Manager: not initialized\n")
	}

	return b.String(), nil
}


// cmdBackend handles the /backend command
func cmdBackend(ctx CommandContext, args []string) (string, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Current Backend: %s\n", ctx.CurrentBackend))
	b.WriteString(fmt.Sprintf("Current Model: %s\n", ctx.CurrentModel))
	b.WriteString("\nNote: To switch backends, restart the application with:\n")
	b.WriteString("  -backend ollama   (default, local Ollama)\n")
	b.WriteString("  -backend openai   (requires OPENAI_API_KEY)\n")
	return b.String(), nil
}