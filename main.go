package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Config holds command-line configuration
type Config struct {
	NoStreaming   bool
	Model         string
	Backend       string
	OpenAIModel   string
	OpenAIAPIKey  string
	OpenAIAPIBase string
}

var config Config

func printUsage() {
	fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
	fmt.Printf("A terminal-based chat application with MCP tool support.\n\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  -backend string\n")
	fmt.Printf("        Backend to use: 'ollama' or 'openai' (default \"ollama\")\n")
	fmt.Printf("  -model string\n")
	fmt.Printf("        Ollama model to use (default \"mistral-small:24b\")\n")
	fmt.Printf("  -openai-model string\n")
	fmt.Printf("        OpenAI model to use (default \"gpt-4o\")\n")
	fmt.Printf("  -openai-api-key string\n")
	fmt.Printf("        OpenAI API key (fallback to OPENAI_API_KEY env var)\n")
	fmt.Printf("  -openai-api-base string\n")
	fmt.Printf("        OpenAI API base URL (fallback to OPENAI_API_BASE env var)\n")
	fmt.Printf("  -no-streaming\n")
	fmt.Printf("        Disable response streaming (some models don't support tool calls with streaming)\n")
	fmt.Printf("  -h, -help\n")
	fmt.Printf("        Show this help message\n")
	fmt.Printf("\nSlash Commands (in-app):\n")
	fmt.Printf("  /help     Show available commands\n")
	fmt.Printf("  /model    Show or change the current model\n")
	fmt.Printf("  /backend  Show or change the current backend\n")
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

	flag.BoolVar(&config.NoStreaming, "no-streaming", false, "Disable response streaming")
	flag.StringVar(&config.Model, "model", "mistral-small:24b", "Ollama model to use")
	flag.StringVar(&config.Backend, "backend", "ollama", "Backend to use: 'ollama' or 'openai'")
	flag.StringVar(&config.OpenAIModel, "openai-model", "gpt-4o", "OpenAI model to use")
	flag.StringVar(&config.OpenAIAPIKey, "openai-api-key", "", "OpenAI API key")
	flag.StringVar(&config.OpenAIAPIBase, "openai-api-base", "", "OpenAI API base URL")
	flag.Parse()

	// Use environment variables as fallback for OpenAI configuration
	if config.OpenAIAPIKey == "" {
		config.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if config.OpenAIAPIBase == "" {
		config.OpenAIAPIBase = os.Getenv("OPENAI_API_BASE")
	}

	// Validate backend selection
	if config.Backend != "ollama" && config.Backend != "openai" {
		fmt.Printf("Error: backend must be 'ollama' or 'openai', got '%s'\n", config.Backend)
		os.Exit(1)
	}

	// Validate OpenAI configuration if using OpenAI backend
	if config.Backend == "openai" && config.OpenAIAPIKey == "" {
		fmt.Printf("Error: OpenAI API key is required. Set OPENAI_API_KEY env var or use -openai-api-key flag\n")
		os.Exit(1)
	}

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
