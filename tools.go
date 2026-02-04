package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// Tool execution functions

func toolRead(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	offset := 0
	limit := len(lines)

	if o, ok := args["offset"].(float64); ok {
		offset = int(o)
	}
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	if offset >= len(lines) {
		return "", nil
	}

	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	var result strings.Builder
	for idx, line := range lines[offset:end] {
		result.WriteString(fmt.Sprintf("%4d| %s\n", offset+idx+1, line))
	}

	return result.String(), nil
}

func toolWrite(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content must be a string")
	}

	if !strings.HasPrefix(path, "/") {
		if strings.HasPrefix(path, "./") {
			path = strings.TrimLeft(path, "./")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot get current working directory")
		}

		path = fmt.Sprintf("%s/%s", cwd, path)
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	err := os.WriteFile(path, []byte(content), 0o644)
	if err != nil {
		return "", err
	}
	fmt.Printf("writing file to %s\n", path)

	return "ok", nil
}

func toolEdit(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	old, ok := args["old"].(string)
	if !ok {
		return "", fmt.Errorf("old must be a string")
	}

	new, ok := args["new"].(string)
	if !ok {
		return "", fmt.Errorf("new must be a string")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	text := string(data)
	if !strings.Contains(text, old) {
		return "error: old_string not found", nil
	}

	count := strings.Count(text, old)
	all, _ := args["all"].(bool)

	if !all && count > 1 {
		return fmt.Sprintf("error: old_string appears %d times, must be unique (use all=true)", count), nil
	}

	var replacement string
	if all {
		replacement = strings.ReplaceAll(text, old, new)
	} else {
		replacement = strings.Replace(text, old, new, 1)
	}

	err = os.WriteFile(path, []byte(replacement), 0o644)
	if err != nil {
		return "", err
	}

	return "ok", nil
}

func toolGlob(args map[string]interface{}) (string, error) {
	pat, ok := args["pat"].(string)
	if !ok {
		return "", fmt.Errorf("pat must be a string")
	}

	basePath := "."
	if p, ok := args["path"].(string); ok {
		basePath = p
	}

	pattern := filepath.Join(basePath, pat)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}

	// Sort by modification time, newest first
	sort.Slice(matches, func(i, j int) bool {
		infoI, errI := os.Stat(matches[i])
		infoJ, errJ := os.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	if len(matches) == 0 {
		return "none", nil
	}

	return strings.Join(matches, "\n"), nil
}

func toolGrep(args map[string]interface{}) (string, error) {
	pat, ok := args["pat"].(string)
	if !ok {
		return "", fmt.Errorf("pat must be a string")
	}

	basePath := "."
	if p, ok := args["path"].(string); ok {
		basePath = p
	}

	pattern, err := regexp.Compile(pat)
	if err != nil {
		return "", err
	}

	var hits []string
	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 1
		for scanner.Scan() {
			line := scanner.Text()
			if pattern.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d:%s", path, lineNum, line))
				if len(hits) >= 50 {
					return filepath.SkipAll
				}
			}
			lineNum++
		}
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if len(hits) == 0 {
		return "none", nil
	}

	return strings.Join(hits, "\n"), nil
}

func toolBash(args map[string]interface{}) (string, error) {
	cmd, ok := args["cmd"].(string)
	if !ok {
		return "", fmt.Errorf("cmd must be a string")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "bash", "-c", cmd)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output

	err := command.Run()
	result := output.String()

	if ctx.Err() == context.DeadlineExceeded {
		result += "\n(timed out after 30s)"
	}

	if result == "" {
		if err != nil {
			return fmt.Sprintf("(empty output, error: %v)", err), nil
		}
		return "(empty)", nil
	}

	// Include error info if command failed but produced output
	if err != nil {
		result = fmt.Sprintf("%s\n(exit code: %v)", strings.TrimSpace(result), err)
	}

	return strings.TrimSpace(result), nil
}

// executeTool dispatches tool calls to the appropriate function
func executeTool(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "read":
		return toolRead(args)
	case "write":
		return toolWrite(args)
	case "edit":
		return toolEdit(args)
	case "glob":
		return toolGlob(args)
	case "grep":
		return toolGrep(args)
	case "bash":
		return toolBash(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// defineTools creates the tool definitions for Ollamawha
func defineTools() api.Tools {
	// read tool
	readProps := api.NewToolPropertiesMap()
	readProps.Set("path", api.ToolProperty{
		Type:        []string{"string"},
		Description: "File path to read",
	})
	readProps.Set("offset", api.ToolProperty{
		Type:        []string{"number"},
		Description: "Line offset to start reading from (optional)",
	})
	readProps.Set("limit", api.ToolProperty{
		Type:        []string{"number"},
		Description: "Maximum number of lines to read (optional)",
	})

	// write tool
	writeProps := api.NewToolPropertiesMap()
	writeProps.Set("path", api.ToolProperty{
		Type:        []string{"string"},
		Description: "File path to write to",
	})
	writeProps.Set("content", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Content to write to the file",
	})

	// edit tool
	editProps := api.NewToolPropertiesMap()
	editProps.Set("path", api.ToolProperty{
		Type:        []string{"string"},
		Description: "File path to edit",
	})
	editProps.Set("old", api.ToolProperty{
		Type:        []string{"string"},
		Description: "String to replace",
	})
	editProps.Set("new", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Replacement string",
	})
	editProps.Set("all", api.ToolProperty{
		Type:        []string{"boolean"},
		Description: "Replace all occurrences (optional, default false)",
	})

	// glob tool
	globProps := api.NewToolPropertiesMap()
	globProps.Set("pat", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Glob pattern to match files",
	})
	globProps.Set("path", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Base path to search from (optional, default '.')",
	})

	// grep tool
	grepProps := api.NewToolPropertiesMap()
	grepProps.Set("pat", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Regular expression pattern to search for",
	})
	grepProps.Set("path", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Base path to search from (optional, default '.')",
	})

	// bash tool
	bashProps := api.NewToolPropertiesMap()
	bashProps.Set("cmd", api.ToolProperty{
		Type:        []string{"string"},
		Description: "Shell command to execute",
	})

	return api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "read",
				Description: "Read file with line numbers (file path, not directory)",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: readProps,
					Required:   []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "write",
				Description: "Write content to file",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: writeProps,
					Required:   []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "edit",
				Description: "Replace old with new in file (old must be unique unless all=true)",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: editProps,
					Required:   []string{"path", "old", "new"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "glob",
				Description: "Find files by pattern, sorted by modification time (newest first)",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: globProps,
					Required:   []string{"pat"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "grep",
				Description: "Search files for regex pattern (returns up to 50 matches)",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: grepProps,
					Required:   []string{"pat"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "bash",
				Description: "Run shell command (30 second timeout)",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: bashProps,
					Required:   []string{"cmd"},
				},
			},
		},
	}
}
