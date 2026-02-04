package main

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input        string
		expectedName string
		expectedArgs []string
	}{
		{"/mcp", "mcp", nil},
		{"/help", "help", nil},
		{"/MCP", "MCP", nil}, // Case preserved in parsing, matched case-insensitively
		{"/command arg1 arg2", "command", []string{"arg1", "arg2"}},
		{"  /mcp  ", "mcp", nil}, // Trimmed
		{"not a command", "", nil},
		{"/", "", nil},
		{"", "", nil},
		{"hello /mcp", "", nil}, // Must start with /
	}

	for _, tt := range tests {
		name, args := ParseCommand(tt.input)
		if name != tt.expectedName {
			t.Errorf("ParseCommand(%q) name = %q, want %q", tt.input, name, tt.expectedName)
		}
		if len(args) != len(tt.expectedArgs) {
			t.Errorf("ParseCommand(%q) args = %v, want %v", tt.input, args, tt.expectedArgs)
		}
	}
}

func TestCommandRegistry(t *testing.T) {
	registry := NewCommandRegistry()

	// Test that default commands are registered
	if !registry.IsCommand("/mcp") {
		t.Error("Expected /mcp to be a command")
	}
	if !registry.IsCommand("/help") {
		t.Error("Expected /help to be a command")
	}

	// Test case insensitivity
	if !registry.IsCommand("/MCP") {
		t.Error("Expected /MCP to be a command (case insensitive)")
	}
	if !registry.IsCommand("/HELP") {
		t.Error("Expected /HELP to be a command (case insensitive)")
	}

	// Test non-commands
	if registry.IsCommand("/unknown") {
		t.Error("Expected /unknown to not be a command")
	}
	if registry.IsCommand("mcp") {
		t.Error("Expected 'mcp' (without /) to not be a command")
	}
	if registry.IsCommand("/path/to/file") {
		t.Error("Expected /path/to/file to not be a command")
	}
}

func TestCmdMCP(t *testing.T) {
	// Test with nil manager
	ctx := CommandContext{MCPManager: nil}
	output, err := cmdMCP(ctx, nil)
	if err != nil {
		t.Errorf("cmdMCP with nil manager returned error: %v", err)
	}
	if output != "No MCP manager configured" {
		t.Errorf("Expected 'No MCP manager configured', got %q", output)
	}
}

func TestCmdHelp(t *testing.T) {
	ctx := CommandContext{}
	output, err := cmdHelp(ctx, nil)
	if err != nil {
		t.Errorf("cmdHelp returned error: %v", err)
	}
	if output == "" {
		t.Error("Expected non-empty help output")
	}
	// Should contain command names
	if !contains(output, "/mcp") {
		t.Error("Expected help to contain /mcp")
	}
	if !contains(output, "/help") {
		t.Error("Expected help to contain /help")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

