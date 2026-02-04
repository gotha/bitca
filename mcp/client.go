package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPTool represents a tool definition from MCP server
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPToolsListResult represents the result of tools/list
type MCPToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

// MCPToolCallResult represents the result of tools/call
type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents content in tool call result
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Client manages communication with an MCP server
type Client struct {
	name       string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	requestID  int64
	mu         sync.Mutex
	tools      []MCPTool
}

// NewClient creates a new MCP client for the given server configuration
func NewClient(name string, config MCPServerConfig) (*Client, error) {
	cmd := exec.Command(config.Command, config.Args...)

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	return &Client{
		name:   name,
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

// Name returns the server name
func (c *Client) Name() string {
	return c.name
}

// Tools returns the cached tools list
func (c *Client) Tools() []MCPTool {
	return c.tools
}

// sendRequest sends a JSON-RPC request and waits for response
func (c *Client) sendRequest(method string, params interface{}) (*JSONRPCResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := atomic.AddInt64(&c.requestID, 1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Write request with newline delimiter
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response lines, skipping non-JSON and notifications until we get our response
	maxAttempts := 50 // Prevent infinite loops
	for attempt := 0; attempt < maxAttempts; attempt++ {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// Skip empty lines
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Skip lines that don't look like JSON
		if line[0] != '{' {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// Not valid JSON-RPC, skip it
			continue
		}

		// Check if this is a notification (no ID) - skip it
		if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
			continue
		}

		// Check if this is our response (matching ID)
		if resp.ID == id {
			if resp.Error != nil {
				return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return &resp, nil
		}

		// Response with different ID - could be out of order, but we'll skip for now
		// In a more robust implementation, we'd queue these
	}

	return nil, fmt.Errorf("no response received after %d attempts", maxAttempts)
}

// Initialize sends the initialize request to the MCP server
func (c *Client) Initialize() error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "bitca",
			"version": "1.0.0",
		},
	}

	_, err := c.sendRequest("initialize", params)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	return nil
}

// ListTools retrieves the list of available tools from the MCP server
func (c *Client) ListTools() ([]MCPTool, error) {
	resp, err := c.sendRequest("tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}

	var result MCPToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %w", err)
	}

	c.tools = result.Tools
	return result.Tools, nil
}

// CallTool executes a tool on the MCP server
func (c *Client) CallTool(name string, arguments map[string]interface{}) (string, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	}

	resp, err := c.sendRequest("tools/call", params)
	if err != nil {
		return "", fmt.Errorf("tools/call failed: %w", err)
	}

	var result MCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse tool result: %w", err)
	}

	if result.IsError {
		var errMsg string
		for _, content := range result.Content {
			if content.Type == "text" {
				errMsg += content.Text
			}
		}
		return "", fmt.Errorf("tool error: %s", errMsg)
	}

	// Concatenate all text content
	var output string
	for _, content := range result.Content {
		if content.Type == "text" {
			output += content.Text
		}
	}

	return output, nil
}

// Close gracefully shuts down the MCP server
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stdin != nil {
		c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	return nil
}

