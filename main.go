package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Config holds command-line configuration
type Config struct {
	NoStreaming bool
	Model       string
}

var config Config

func printUsage() {
	fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
	fmt.Printf("A terminal-based chat application using Ollama with MCP tool support.\n\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  -model string\n")
	fmt.Printf("        Ollama model to use (default \"mistral-small:24b\")\n")
	fmt.Printf("  -no-streaming\n")
	fmt.Printf("        Disable response streaming (some models don't support tool calls with streaming)\n")
	fmt.Printf("  -h, -help\n")
	fmt.Printf("        Show this help message\n")
	fmt.Printf("\nSlash Commands (in-app):\n")
	fmt.Printf("  /help     Show available commands\n")
	fmt.Printf("  /model    Show or change the current model\n")
	fmt.Printf("  /tools    List available tools\n")
	fmt.Printf("  /mcp      Show MCP server status\n")
	fmt.Printf("  /debug    Show debug information\n")
}

func main() {
	// Check for help flags before using flag package (to avoid TTY issues)
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			printUsage()
			os.Exit(0)
		}
	}

	flag.BoolVar(&config.NoStreaming, "no-streaming", false, "Disable response streaming (some models don't support tool calls with streaming)")
	flag.StringVar(&config.Model, "model", "mistral-small:24b", "Ollama model to use")
	flag.Parse()

	m, err := initialModel()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
