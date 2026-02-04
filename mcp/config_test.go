package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPConfig(t *testing.T) {
	// Test loading non-existent file returns empty config
	config, err := LoadMCPConfig("/nonexistent/path/mcp.json")
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
	if config == nil {
		t.Error("Expected non-nil config")
	}
	if len(config.MCPServers) != 0 {
		t.Errorf("Expected empty MCPServers, got %d", len(config.MCPServers))
	}
}

func TestLoadMCPConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mcp.json")

	configContent := `{
		"mcpServers": {
			"test-server": {
				"command": "/usr/bin/test-mcp",
				"args": ["--arg1", "value1"],
				"env": {
					"TEST_VAR": "test_value"
				}
			},
			"http-server": {
				"type": "http",
				"url": "http://localhost:8080"
			},
			"sse-server": {
				"type": "sse",
				"url": "http://localhost:8081/sse"
			}
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadMCPConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(config.MCPServers) != 3 {
		t.Errorf("Expected 3 servers, got %d", len(config.MCPServers))
	}

	// Test stdio server
	testServer, ok := config.MCPServers["test-server"]
	if !ok {
		t.Error("Expected test-server in config")
	} else {
		if testServer.Command != "/usr/bin/test-mcp" {
			t.Errorf("Expected command /usr/bin/test-mcp, got %s", testServer.Command)
		}
		if len(testServer.Args) != 2 {
			t.Errorf("Expected 2 args, got %d", len(testServer.Args))
		}
		if testServer.Env["TEST_VAR"] != "test_value" {
			t.Errorf("Expected TEST_VAR=test_value, got %s", testServer.Env["TEST_VAR"])
		}
		if !testServer.IsStdio() {
			t.Error("Expected IsStdio() to return true")
		}
	}

	// Test HTTP server
	httpServer, ok := config.MCPServers["http-server"]
	if !ok {
		t.Error("Expected http-server in config")
	} else {
		if !httpServer.IsHTTP() {
			t.Error("Expected IsHTTP() to return true")
		}
		if httpServer.URL != "http://localhost:8080" {
			t.Errorf("Expected URL http://localhost:8080, got %s", httpServer.URL)
		}
	}

	// Test SSE server
	sseServer, ok := config.MCPServers["sse-server"]
	if !ok {
		t.Error("Expected sse-server in config")
	} else {
		if !sseServer.IsSSE() {
			t.Error("Expected IsSSE() to return true")
		}
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Error("Expected non-empty default config path")
	}

	// Should end with .config/mcp/mcp.json
	if filepath.Base(path) != "mcp.json" {
		t.Errorf("Expected path to end with mcp.json, got %s", path)
	}
}

