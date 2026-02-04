package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gotha/bitca/mcp"
	"github.com/ollama/ollama/api"
)

var debugLog *log.Logger

func init() {
	f, err := os.OpenFile("debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening debug log file: %v", err)
	}
	debugLog = log.New(f, "", log.LstdFlags)
}

var (
	// Color palette
	primaryColor   = lipgloss.Color("#7D56F4")
	secondaryColor = lipgloss.Color("#00D9FF")
	accentColor    = lipgloss.Color("#FF6AC1")
	userColor      = lipgloss.Color("#7FFF00")
	assistantColor = lipgloss.Color("#FFD700")
	errorColor     = lipgloss.Color("#FF5555")
	subtleColor    = lipgloss.Color("#6C7086")
	borderColor    = lipgloss.Color("#89B4FA")

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(lipgloss.Color("#1E1E2E")).
			Padding(0, 1).
			MarginBottom(1)

	userMessageStyle = lipgloss.NewStyle().
				Foreground(userColor).
				Bold(true).
				MarginTop(1)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(assistantColor).
				Bold(true).
				MarginTop(1)

	messageContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CDD6F4")).
				MarginLeft(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			MarginTop(1)

	commandStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			MarginTop(1)

	systemStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			MarginTop(1)

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true).
			MarginLeft(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(subtleColor).
			Italic(true).
			MarginLeft(1)

	viewportStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(1, 2)
)

type model struct {
	viewport        viewport.Model
	textInput       textinput.Model
	messages        []api.Message
	conversation    []string
	client          *api.Client
	tools           api.Tools
	mcpManager      *mcp.Manager
	commandRegistry *CommandRegistry
	modelName       string
	waiting         bool
	streaming       bool
	currentResponse string
	streamChan      chan tea.Msg
	err             error
	ready           bool
	width           int
	height          int
}

type streamChunkMsg struct {
	content string
}

type streamDoneMsg struct {
	fullContent  string
	assistantMsg api.Message
	err          error
}

type toolExecutionMsg struct {
	results []api.Message
}

func initialModel() (model, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return model{}, err
	}

	ti := textinput.New()
	ti.Placeholder = "Type your message..."
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(subtleColor)
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(accentColor)
	ti.PromptStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	ti.Prompt = "› "
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 80

	vp := viewport.New(80, 20)
	vp.SetContent("")

	// Define built-in tools
	tools := defineTools()

	// Load MCP tools
	mcpManager := mcp.NewManager()
	if err := mcpManager.LoadFromConfig(mcp.DefaultConfigPath()); err != nil {
		// Log warning but continue without MCP tools
		fmt.Printf("Warning: failed to load MCP config: %v\n", err)
	}

	// Merge MCP tools with built-in tools
	mcpTools := mcpManager.GetOllamaTools()
	tools = append(tools, mcpTools...)

	// Build a concise system prompt - don't list all tools, let the model discover them
	systemPrompt := api.Message{
		Role: "system",
		Content: `You are a helpful AI assistant with access to tools.

CRITICAL INSTRUCTION: You MUST call tools when asked to perform actions. Do NOT describe or explain tools - USE them by making function calls.

When the user asks you to do something, call the appropriate tool immediately. Examples:
- "What is my GitHub username?" -> Call the get_me tool
- "Read a file" -> Call the read tool
- "Run a command" -> Call the bash tool
- "Show git status" -> Call the git_status tool

Never explain how to use a tool. Just call it.`,
	}

	// Create command registry
	commandRegistry := NewCommandRegistry()

	// Use model from command-line config
	modelName := config.Model

	return model{
		viewport:        vp,
		textInput:       ti,
		messages:        []api.Message{systemPrompt},
		conversation:    []string{},
		client:          client,
		tools:           tools,
		mcpManager:      mcpManager,
		commandRegistry: commandRegistry,
		modelName:       modelName,
		waiting:         false,
		streaming:       false,
		currentResponse: "",
		ready:           false,
	}, nil
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Handle nil messages (from closed channels)
	if msg == nil {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Initialize viewport with proper size
			// Account for borders, padding, and input area
			viewportWidth := msg.Width - 8    // Account for viewport border and padding
			viewportHeight := msg.Height - 10 // Account for viewport, input, and help text
			m.viewport = viewport.New(viewportWidth, viewportHeight)
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 8
			m.viewport.Height = msg.Height - 10
		}

		m.textInput.Width = msg.Width - 10 // Account for input box border and padding
		m.updateViewportContent()

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.waiting {
				return m, nil
			}
			userInput := strings.TrimSpace(m.textInput.Value())
			if userInput == "" {
				return m, nil
			}

			// Check if input is a slash command
			if m.commandRegistry != nil && m.commandRegistry.IsCommand(userInput) {
				// Handle /model command specially to update model
				cmdName, cmdArgs := ParseCommand(userInput)
				if strings.ToLower(cmdName) == "model" && len(cmdArgs) > 0 {
					newModel := cmdArgs[0]
					m.modelName = newModel
					m.conversation = append(m.conversation, fmt.Sprintf("Command: %s", userInput))
					m.conversation = append(m.conversation, fmt.Sprintf("System: Model changed to: %s", newModel))
					m.textInput.SetValue("")
					m.updateViewportContent()
					return m, nil
				}

				// Execute the command
				ctx := CommandContext{
					MCPManager:   m.mcpManager,
					Tools:        m.tools,
					CurrentModel: m.modelName,
				}

				// Show the command in conversation
				m.conversation = append(m.conversation, fmt.Sprintf("Command: %s", userInput))
				m.textInput.SetValue("")

				output, err := m.commandRegistry.Execute(userInput, ctx)
				if err != nil {
					m.conversation = append(m.conversation, fmt.Sprintf("Error: %s", err.Error()))
				} else {
					m.conversation = append(m.conversation, fmt.Sprintf("System: %s", output))
				}

				m.updateViewportContent()
				return m, nil
			}

			// Add user message to messages array (for Ollama context)
			userMsg := api.Message{Role: "user", Content: userInput}
			m.messages = append(m.messages, userMsg)

			// Add user message to conversation display
			m.conversation = append(m.conversation, fmt.Sprintf("You: %s", userInput))
			m.textInput.SetValue("")
			m.waiting = true
			m.streaming = true
			m.currentResponse = ""
			m.streamChan = make(chan tea.Msg)

			m.updateViewportContent()

			// Send to API (messages array already contains full context)
			return m, tea.Batch(
				m.sendMessage(),
				waitForStreamChunk(m.streamChan),
			)
		}

	case streamChunkMsg:
		// Accumulate streaming chunks
		m.currentResponse += msg.content
		m.updateViewportContent()
		// Continue waiting for more chunks only if channel is still valid
		if m.streamChan != nil {
			return m, waitForStreamChunk(m.streamChan)
		}
		return m, nil

	case streamDoneMsg:
		m.waiting = false
		m.streaming = false
		m.streamChan = nil // Clear the channel to prevent stale reads
		if msg.err != nil {
			m.err = msg.err
			m.conversation = append(m.conversation, fmt.Sprintf("Error: %s", msg.err))
			m.currentResponse = ""
			m.updateViewportContent()
			return m, nil
		}

		// Add assistant message to messages array (for Ollama context)
		m.messages = append(m.messages, msg.assistantMsg)

		// Check if there are tool calls to execute (from API)
		toolCalls := msg.assistantMsg.ToolCalls

		// If no API tool calls, check if the response contains JSON tool calls
		if len(toolCalls) == 0 && msg.fullContent != "" {
			jsonToolCalls := parseJSONToolCalls(msg.fullContent)
			if len(jsonToolCalls) > 0 {
				toolCalls = jsonToolCalls
			}
		}

		if len(toolCalls) > 0 {
			// Display that tools are being executed
			if msg.fullContent != "" {
				m.conversation = append(m.conversation, fmt.Sprintf("Assistant: %s", msg.fullContent))
			}

			// Show which tools are being called
			for _, tc := range toolCalls {
				args := tc.Function.Arguments.ToMap()
				m.conversation = append(m.conversation, fmt.Sprintf("[Calling tool: %s with args: %v]", tc.Function.Name, args))
			}

			m.updateViewportContent()

			// Execute tools and continue conversation
			return m, m.executeTools(toolCalls)
		}

		// No tool calls, just display the response
		m.conversation = append(m.conversation, fmt.Sprintf("Assistant: %s", msg.fullContent))
		m.currentResponse = ""
		m.updateViewportContent()
		return m, nil

	case toolExecutionMsg:
		// Add tool results to messages
		m.messages = append(m.messages, msg.results...)

		// Display tool execution results
		for i, result := range msg.results {
			// Truncate long results for display
			displayContent := result.Content
			if len(displayContent) > 500 {
				displayContent = displayContent[:500] + "... (truncated)"
			}
			m.conversation = append(m.conversation, fmt.Sprintf("Tool Result %d: %s", i+1, displayContent))
		}

		// Continue the conversation with tool results
		m.waiting = true
		m.streaming = true
		m.currentResponse = ""
		m.streamChan = make(chan tea.Msg)
		m.updateViewportContent()

		return m, tea.Batch(
			m.sendMessage(),
			waitForStreamChunk(m.streamChan),
		)
	}

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Update text input only when not waiting
	if !m.waiting {
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updateViewportContent() {
	var b strings.Builder

	// Display title
	title := titleStyle.Render("💬 Chat with Ollama")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Display conversation history
	for _, line := range m.conversation {
		// Parse the line to determine if it's user or assistant
		if strings.HasPrefix(line, "You: ") {
			content := strings.TrimPrefix(line, "You: ")
			label := userMessageStyle.Render("You:")
			wrapped := wordWrap(content, m.viewport.Width-4)
			styledContent := messageContentStyle.Render(wrapped)
			b.WriteString(label + "\n" + styledContent)
		} else if strings.HasPrefix(line, "Assistant: ") {
			content := strings.TrimPrefix(line, "Assistant: ")
			label := assistantMessageStyle.Render("Assistant:")
			wrapped := wordWrap(content, m.viewport.Width-4)
			styledContent := messageContentStyle.Render(wrapped)
			b.WriteString(label + "\n" + styledContent)
		} else if strings.HasPrefix(line, "Error: ") {
			content := strings.TrimPrefix(line, "Error: ")
			label := errorStyle.Render("Error:")
			wrapped := wordWrap(content, m.viewport.Width-4)
			styledContent := messageContentStyle.Render(wrapped)
			b.WriteString(label + "\n" + styledContent)
		} else if strings.HasPrefix(line, "Command: ") {
			content := strings.TrimPrefix(line, "Command: ")
			label := commandStyle.Render("Command:")
			wrapped := wordWrap(content, m.viewport.Width-4)
			styledContent := messageContentStyle.Render(wrapped)
			b.WriteString(label + "\n" + styledContent)
		} else if strings.HasPrefix(line, "System: ") {
			content := strings.TrimPrefix(line, "System: ")
			label := systemStyle.Render("System:")
			wrapped := wordWrap(content, m.viewport.Width-4)
			styledContent := messageContentStyle.Render(wrapped)
			b.WriteString(label + "\n" + styledContent)
		} else {
			wrapped := wordWrap(line, m.viewport.Width-4)
			b.WriteString(wrapped)
		}
		b.WriteString("\n\n")
	}

	// Display streaming response if active
	if m.streaming && m.currentResponse != "" {
		label := assistantMessageStyle.Render("Assistant:")
		wrapped := wordWrap(m.currentResponse, m.viewport.Width-4)
		styledContent := messageContentStyle.Render(wrapped)
		b.WriteString(label + "\n" + styledContent)
		b.WriteString("\n\n")
	}

	m.viewport.SetContent(b.String())
	// Auto-scroll to bottom
	m.viewport.GotoBottom()
}

// wordWrap wraps text to the specified width
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	return lipgloss.NewStyle().Width(width).Render(text)
}

func (m model) View() string {
	if !m.ready {
		loadingStyle := lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Padding(2)
		return loadingStyle.Render("⏳ Initializing...")
	}

	var b strings.Builder

	// Display viewport with conversation (with border)
	viewportContent := viewportStyle.
		Width(m.width - 4).
		Height(m.height - 6).
		Render(m.viewport.View())
	b.WriteString(viewportContent)
	b.WriteString("\n")

	// Display input or waiting message at the bottom
	if m.waiting {
		var statusMsg string
		if m.streaming {
			statusMsg = statusStyle.Render("⏳ Streaming response...")
		} else {
			statusMsg = statusStyle.Render("⏳ Waiting for response...")
		}
		styledStatus := inputBoxStyle.Width(m.width - 4).Render(statusMsg)
		b.WriteString(styledStatus)
	} else {
		// Style the input box
		inputContent := m.textInput.View()
		styledInput := inputBoxStyle.Width(m.width - 4).Render(inputContent)
		b.WriteString(styledInput)
		b.WriteString("\n")
		help := helpStyle.Render("Press Ctrl+C or Esc to quit • Arrow keys to scroll")
		b.WriteString(help)
	}

	return b.String()
}

func (m model) sendMessage() tea.Cmd {
	// Messages array already contains the full conversation context
	// Streaming is enabled by default, but can be disabled via --no-streaming flag
	// Some models don't return tool calls with streaming enabled
	stream := !config.NoStreaming
	req := &api.ChatRequest{
		Model:    m.modelName,
		Messages: m.messages, // This includes all previous messages for context
		Tools:    m.tools,    // Include tool definitions
		Stream:   &stream,
	}

	// Debug: log the request
	debugLog.Printf("=== Sending request to Ollama ===")
	debugLog.Printf("Model: %s", m.modelName)
	debugLog.Printf("Streaming: %v", stream)
	debugLog.Printf("Number of tools: %d", len(m.tools))
	// Log all tool names to help debug
	for i, tool := range m.tools {
		debugLog.Printf("  Tool %d: %s", i, tool.Function.Name)
	}
	// Log detailed structure of get_me tool (if present)
	for _, tool := range m.tools {
		if tool.Function.Name == "get_me" {
			debugLog.Printf("=== get_me tool details ===")
			debugLog.Printf("  Name: %s", tool.Function.Name)
			debugLog.Printf("  Description: %s", tool.Function.Description)
			debugLog.Printf("  Parameters Type: %s", tool.Function.Parameters.Type)
			debugLog.Printf("  Parameters Required: %v", tool.Function.Parameters.Required)
			// Iterate over properties map
			debugLog.Printf("  Properties:")
			if tool.Function.Parameters.Properties != nil {
				propsMap := tool.Function.Parameters.Properties.ToMap()
				debugLog.Printf("    Properties count: %d", len(propsMap))
				for k, v := range propsMap {
					debugLog.Printf("    %s: type=%v, desc=%s", k, v.Type, v.Description)
				}
			} else {
				debugLog.Printf("    (nil)")
			}
			// Log the full JSON of the tool
			toolJSON, _ := json.MarshalIndent(tool, "", "  ")
			debugLog.Printf("  Full JSON:\n%s", string(toolJSON))
		}
	}
	debugLog.Printf("Number of messages: %d", len(m.messages))

	// Return a command that will stream responses
	return func() tea.Msg {
		ctx := context.Background()

		go func() {
			defer close(m.streamChan)

			var fullContent strings.Builder
			var toolCalls []api.ToolCall

			err := m.client.Chat(ctx, req, func(resp api.ChatResponse) error {
				debugLog.Printf("Response callback - Content len: %d, ToolCalls: %d", len(resp.Message.Content), len(resp.Message.ToolCalls))

				if resp.Message.Content != "" {
					fullContent.WriteString(resp.Message.Content)
					// Send chunk update
					m.streamChan <- streamChunkMsg{content: resp.Message.Content}
				}

				// Capture tool calls from the response
				if len(resp.Message.ToolCalls) > 0 {
					debugLog.Printf("Received tool calls: %d", len(resp.Message.ToolCalls))
					for _, tc := range resp.Message.ToolCalls {
						debugLog.Printf("  Tool call: %s with args: %v", tc.Function.Name, tc.Function.Arguments)
					}
					toolCalls = resp.Message.ToolCalls
				}

				return nil
			})
			if err != nil {
				m.streamChan <- streamDoneMsg{err: err}
				return
			}

			// Create assistant message with the complete response
			content := fullContent.String()
			assistantMsg := api.Message{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			}

			// Send the complete message back to be added to context
			m.streamChan <- streamDoneMsg{
				fullContent:  content,
				assistantMsg: assistantMsg,
			}
		}()

		// Return nil to indicate the goroutine is started
		return nil
	}
}

// executeTools runs the requested tool calls and returns their results
func (m model) executeTools(toolCalls []api.ToolCall) tea.Cmd {
	return func() tea.Msg {
		var results []api.Message

		for _, toolCall := range toolCalls {
			// Get arguments as map
			args := toolCall.Function.Arguments.ToMap()
			toolName := toolCall.Function.Name

			// Log tool execution for debugging
			toolInfo := fmt.Sprintf("Executing tool: %s with args: %v", toolName, args)

			var result string
			var err error

			// Check if this is an MCP tool
			if m.mcpManager != nil && m.mcpManager.HasTool(toolName) {
				result, err = m.mcpManager.ExecuteTool(toolName, args)
			} else {
				// Execute built-in tool
				result, err = executeTool(toolName, args)
			}

			if err != nil {
				// Create tool response message with error
				results = append(results, api.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Error executing %s: %v\nDebug: %s", toolName, err, toolInfo),
					ToolCallID: toolCall.ID, // Link back to the tool call
				})
				continue
			}

			// Add successful result with tool call ID
			results = append(results, api.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID, // Link back to the tool call
			})
		}

		return toolExecutionMsg{results: results}
	}
}

// waitForStreamChunk waits for the next streaming chunk
func waitForStreamChunk(msgChan <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-msgChan
		if !ok {
			return nil
		}
		return msg
	}
}

// parseJSONToolCalls extracts tool calls from JSON in the response text
// Looks for patterns like: {"name": "tool_name", "parameters": {...}}
func parseJSONToolCalls(content string) []api.ToolCall {
	var toolCalls []api.ToolCall

	// Pattern to match JSON objects with "name" and "parameters" fields
	// This matches: {"name": "...", "parameters": {...}}
	jsonPattern := regexp.MustCompile(`\{[^{}]*"name"\s*:\s*"([^"]+)"[^{}]*"parameters"\s*:\s*(\{[^}]*\})[^{}]*\}`)

	matches := jsonPattern.FindAllStringSubmatch(content, -1)

	for i, match := range matches {
		if len(match) < 3 {
			continue
		}

		toolName := match[1]
		paramsJSON := match[2]

		// Parse parameters
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			continue
		}

		// Convert params to ToolCallFunctionArguments
		argsJSON, err := json.Marshal(params)
		if err != nil {
			continue
		}

		var args api.ToolCallFunctionArguments
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			continue
		}

		// Create a ToolCall
		toolCall := api.ToolCall{
			ID: fmt.Sprintf("manual_call_%d", i),
			Function: api.ToolCallFunction{
				Name:      toolName,
				Arguments: args,
			},
		}

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}
