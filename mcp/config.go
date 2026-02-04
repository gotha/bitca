package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MCPServerConfig represents configuration for a single MCP server
type MCPServerConfig struct {
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	Type        string            `json:"type"`        // "stdio" (default), "sse", or "http"
	URL         string            `json:"url"`         // URL for SSE/HTTP transports
	Description string            `json:"description"` // Optional description
}

// IsStdio returns true if this server uses stdio transport
func (c MCPServerConfig) IsStdio() bool {
	return c.Type == "" || c.Type == "stdio"
}

// IsSSE returns true if this server uses SSE transport
func (c MCPServerConfig) IsSSE() bool {
	return c.Type == "sse"
}

// IsHTTP returns true if this server uses HTTP transport
func (c MCPServerConfig) IsHTTP() bool {
	return c.Type == "http"
}

// MCPConfig represents the root MCP configuration
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// DefaultConfigPath returns the default path for MCP configuration
func DefaultConfigPath() string {
	// First check if mcp.json exists in the current directory
	localPath := "mcp.json"
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	// Fall back to ~/.config/mcp/mcp.json
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mcp", "mcp.json")
}

// LoadMCPConfig loads the MCP configuration from the specified path
// Returns an empty config if the file doesn't exist (not an error)
// Returns an error only on parse failures
func LoadMCPConfig(path string) (*MCPConfig, error) {
	// Expand ~ to home directory
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return &MCPConfig{MCPServers: make(map[string]MCPServerConfig)}, nil
		}
		path = filepath.Join(home, path[1:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty config
			return &MCPConfig{MCPServers: make(map[string]MCPServerConfig)}, nil
		}
		return nil, err
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.MCPServers == nil {
		config.MCPServers = make(map[string]MCPServerConfig)
	}

	return &config, nil
}

