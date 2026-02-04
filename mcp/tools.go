package mcp

import (
	"fmt"
	"log"

	"github.com/gotha/bitca/backend"
	"github.com/ollama/ollama/api"
)

// MCPClient interface for different transport types
type MCPClient interface {
	Name() string
	Tools() []MCPTool
	Initialize() error
	ListTools() ([]MCPTool, error)
	CallTool(name string, arguments map[string]interface{}) (string, error)
	Close() error
}

// Manager manages multiple MCP clients and their tools
type Manager struct {
	clients      map[string]MCPClient
	toolToClient map[string]MCPClient // maps tool name to its client
}

// NewManager creates a new MCP manager
func NewManager() *Manager {
	return &Manager{
		clients:      make(map[string]MCPClient),
		toolToClient: make(map[string]MCPClient),
	}
}

// LoadFromConfig loads and initializes all MCP servers from config
func (m *Manager) LoadFromConfig(configPath string) error {
	config, err := LoadMCPConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load MCP config: %w", err)
	}

	for name, serverConfig := range config.MCPServers {
		var client MCPClient
		var err error

		if serverConfig.IsStdio() {
			client, err = NewClient(name, serverConfig)
		} else if serverConfig.IsSSE() || serverConfig.IsHTTP() {
			client, err = NewHTTPClient(name, serverConfig)
		} else {
			log.Printf("Warning: unsupported transport type %s for MCP server %s", serverConfig.Type, name)
			continue
		}

		if err != nil {
			log.Printf("Warning: failed to start MCP server %s: %v", name, err)
			continue
		}

		if err := client.Initialize(); err != nil {
			log.Printf("Warning: failed to initialize MCP server %s: %v", name, err)
			client.Close()
			continue
		}

		tools, err := client.ListTools()
		if err != nil {
			log.Printf("Warning: failed to list tools from MCP server %s: %v", name, err)
			client.Close()
			continue
		}

		m.clients[name] = client

		// Map each tool to its client
		for _, tool := range tools {
			m.toolToClient[tool.Name] = client
		}

		log.Printf("Loaded %d tools from MCP server %s", len(tools), name)
	}

	return nil
}

// GetOllamaTools converts all MCP tools to Ollama format
func (m *Manager) GetOllamaTools() api.Tools {
	var tools api.Tools

	for _, client := range m.clients {
		for _, mcpTool := range client.Tools() {
			tool := convertMCPToolToOllama(mcpTool)
			tools = append(tools, tool)
		}
	}

	return tools
}

// GetBackendTools converts all MCP tools to backend.Tool format
func (m *Manager) GetBackendTools() []backend.Tool {
	var tools []backend.Tool

	for _, client := range m.clients {
		for _, mcpTool := range client.Tools() {
			tool := backend.Tool{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				Parameters:  mcpTool.InputSchema,
			}
			tools = append(tools, tool)
		}
	}

	return tools
}

// ExecuteTool executes a tool by name, routing to the appropriate MCP client
func (m *Manager) ExecuteTool(name string, args map[string]interface{}) (string, error) {
	client, ok := m.toolToClient[name]
	if !ok {
		return "", fmt.Errorf("unknown MCP tool: %s", name)
	}

	return client.CallTool(name, args)
}

// HasTool checks if a tool is managed by this MCP manager
func (m *Manager) HasTool(name string) bool {
	_, ok := m.toolToClient[name]
	return ok
}

// GetToolServer returns the server name for a given tool, or empty string if not found
func (m *Manager) GetToolServer(toolName string) string {
	client, ok := m.toolToClient[toolName]
	if !ok {
		return ""
	}
	return client.Name()
}

// Close shuts down all MCP clients
func (m *Manager) Close() {
	for _, client := range m.clients {
		client.Close()
	}
}

// ServerInfo contains information about an MCP server
type ServerInfo struct {
	Name      string
	Transport string
	ToolCount int
	ToolNames []string
}

// GetServers returns information about all loaded MCP servers
func (m *Manager) GetServers() []ServerInfo {
	var servers []ServerInfo

	for name, client := range m.clients {
		tools := client.Tools()
		toolNames := make([]string, len(tools))
		for i, tool := range tools {
			toolNames[i] = tool.Name
		}

		// Determine transport type based on client type
		transport := "stdio"
		if _, ok := client.(*HTTPClient); ok {
			transport = "http"
		}

		servers = append(servers, ServerInfo{
			Name:      name,
			Transport: transport,
			ToolCount: len(tools),
			ToolNames: toolNames,
		})
	}

	return servers
}

// convertMCPToolToOllama converts an MCP tool definition to Ollama format
func convertMCPToolToOllama(mcpTool MCPTool) api.Tool {
	props := api.NewToolPropertiesMap()
	var required []string

	// Extract properties from inputSchema
	if mcpTool.InputSchema != nil {
		if propsMap, ok := mcpTool.InputSchema["properties"].(map[string]interface{}); ok {
			for propName, propDef := range propsMap {
				if propDefMap, ok := propDef.(map[string]interface{}); ok {
					prop := api.ToolProperty{}

					if t, ok := propDefMap["type"].(string); ok {
						prop.Type = []string{t}
					}
					if desc, ok := propDefMap["description"].(string); ok {
						prop.Description = desc
					}

					props.Set(propName, prop)
				}
			}
		}

		// Extract required fields
		if reqList, ok := mcpTool.InputSchema["required"].([]interface{}); ok {
			for _, r := range reqList {
				if s, ok := r.(string); ok {
					required = append(required, s)
				}
			}
		}
	}

	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        mcpTool.Name,
			Description: mcpTool.Description,
			Parameters: api.ToolFunctionParameters{
				Type:       "object",
				Properties: props,
				Required:   required,
			},
		},
	}
}

