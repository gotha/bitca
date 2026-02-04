package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

// HTTPClient manages communication with an MCP server over HTTP/SSE
type HTTPClient struct {
	name       string
	baseURL    string
	httpClient *http.Client
	requestID  int64
	tools      []MCPTool
	isSSE      bool
}

// NewHTTPClient creates a new HTTP-based MCP client
func NewHTTPClient(name string, config MCPServerConfig) (*HTTPClient, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("URL is required for HTTP/SSE transport")
	}

	return &HTTPClient{
		name:       name,
		baseURL:    config.URL,
		httpClient: &http.Client{},
		isSSE:      config.IsSSE(),
	}, nil
}

// Name returns the server name
func (c *HTTPClient) Name() string {
	return c.name
}

// Tools returns the cached tools list
func (c *HTTPClient) Tools() []MCPTool {
	return c.tools
}

// sendRequest sends a JSON-RPC request over HTTP
func (c *HTTPClient) sendRequest(method string, params interface{}) (*JSONRPCResponse, error) {
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

	// For SSE endpoints, we typically POST to a different endpoint
	url := c.baseURL
	if c.isSSE {
		// SSE endpoints often have a /message or /rpc endpoint for requests
		url = c.baseURL + "/message"
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("HTTP error %d: %s", httpResp.StatusCode, string(body))
	}

	respData, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp, nil
}

// Initialize sends the initialize request to the MCP server
func (c *HTTPClient) Initialize() error {
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
func (c *HTTPClient) ListTools() ([]MCPTool, error) {
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
func (c *HTTPClient) CallTool(name string, arguments map[string]interface{}) (string, error) {
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

// Close gracefully shuts down the HTTP client
func (c *HTTPClient) Close() error {
	// HTTP clients don't need explicit cleanup
	return nil
}

