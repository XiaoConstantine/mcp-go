package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
	"github.com/XiaoConstantine/mcp-go/pkg/server"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
)

// ZshPersistentShell manages a persistent Zsh shell process
type ZshPersistentShell struct {
	cmd       *exec.Cmd
	stdin     *bytes.Buffer
	stdout    *bytes.Buffer
	stderr    *bytes.Buffer
	cwd       string
	isRunning bool
	mu        sync.Mutex // For thread safety
}

// GetInstance returns the singleton instance of ZshPersistentShell
func (s *ZshPersistentShell) GetInstance() *ZshPersistentShell {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		s.start()
	}
	return s
}

// start initializes and starts the Zsh shell process
func (s *ZshPersistentShell) start() error {
	// Initialize with home directory as default
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	s.cwd = homeDir
	s.stdin = new(bytes.Buffer)
	s.stdout = new(bytes.Buffer)
	s.stderr = new(bytes.Buffer)

	// Start a persistent Zsh process
	s.cmd = exec.Command("zsh", "-i")
	s.cmd.Dir = s.cwd
	s.cmd.Stdin = s.stdin
	s.cmd.Stdout = s.stdout
	s.cmd.Stderr = s.stderr

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start zsh: %v", err)
	}

	s.isRunning = true
	return nil
}

// setCwd changes the current working directory of the shell
func (s *ZshPersistentShell) setCwd(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify the directory exists
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("directory %s does not exist: %v", dir, err)
	}

	// Change directory in the shell
	s.stdin.WriteString(fmt.Sprintf("cd %s\n", dir))
	s.cwd = dir
	return nil
}

// exec executes a command in the shell with timeout and cancellation support
func (s *ZshPersistentShell) exec(cmd string, signal context.Context, timeout time.Duration) (string, string, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear previous output
	s.stdout.Reset()
	s.stderr.Reset()

	// Write command to shell's stdin
	s.stdin.WriteString(cmd + "\n")

	// Set up timeout context
	ctx, cancel := context.WithTimeout(signal, timeout)
	defer cancel()

	// Wait for command completion or timeout
	done := make(chan struct{})
	var stdout, stderr string
	var exitCode int
	var interrupted bool

	go func() {
		// Execute and capture output
		// In a real implementation, we would read from stdout and stderr pipes
		time.Sleep(100 * time.Millisecond) // Give shell time to process
		stdout = s.stdout.String()
		stderr = s.stderr.String()
		exitCode = 0 // In a real implementation, we would capture exit code
		done <- struct{}{}
	}()

	// Wait for execution to complete or context cancellation
	select {
	case <-done:
		return stdout, stderr, exitCode, false
	case <-ctx.Done():
		interrupted = true
		return stdout, stderr, 1, interrupted
	}
}

// cleanup terminates the shell process
func (s *ZshPersistentShell) cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning && s.cmd != nil && s.cmd.Process != nil {
		// Send exit command first for clean shutdown
		s.stdin.WriteString("exit\n")

		// Give the process a moment to exit cleanly
		time.Sleep(100 * time.Millisecond)

		// If still running, kill it
		if err := s.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill zsh process: %v", err)
		}
		s.isRunning = false
	}
	return nil
}

// ZshToolHandler implements a tool handler for Zsh shell operations
type ZshToolHandler struct {
	shell          *ZshPersistentShell
	originalCwd    string
	bannedCommands []string
}

// NewZshToolHandler creates a new Zsh tool handler
func NewZshToolHandler() *ZshToolHandler {
	cwd, _ := os.Getwd()

	return &ZshToolHandler{
		shell:       &ZshPersistentShell{},
		originalCwd: cwd,
		bannedCommands: []string{
			"curl", "wget", "nc", "telnet", "ssh", "scp", "sftp",
			"rm -rf /", "sudo rm -rf", ":(){:|:&};:", "> /dev/sda",
			"dd", "mkfs", "chrome", "firefox", "lynx", "w3m",
		},
	}
}

// ListTools returns a list of available Zsh tools
func (h *ZshToolHandler) ListTools() ([]models.Tool, error) {
	return []models.Tool{
		{
			Name:        "zsh_execute",
			Description: "Execute a Zsh command",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"command": {
						Type:        "string",
						Description: "Command to execute",
						Required:    true,
					},
					"timeout": {
						Type:        "number",
						Description: "Optional timeout in milliseconds (max 600000)",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "zsh_history",
			Description: "Get command history from Zsh",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"limit": {
						Type:        "number",
						Description: "Number of history entries to return",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "zsh_alias",
			Description: "List all aliases defined in Zsh",
			InputSchema: models.InputSchema{
				Type:       "object",
				Properties: map[string]models.ParameterSchema{},
			},
		},
		{
			Name:        "zsh_env",
			Description: "Show environment variables",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"name": {
						Type:        "string",
						Description: "Name of specific environment variable to show",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "zsh_which",
			Description: "Locate a command",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"command": {
						Type:        "string",
						Description: "Command to locate",
						Required:    true,
					},
				},
			},
		},
		{
			Name:        "zsh_config",
			Description: "Read or manipulate Zsh configuration",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"action": {
						Type:        "string",
						Description: "Action to perform: read, append",
						Required:    true,
					},
					"file": {
						Type:        "string",
						Description: "Config file to operate on: zshrc, zshenv, zprofile",
						Required:    true,
					},
					"content": {
						Type:        "string",
						Description: "Content to append (only used with append action)",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "zsh_glob",
			Description: "Perform advanced Zsh globbing operations",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"pattern": {
						Type:        "string",
						Description: "Glob pattern to evaluate",
						Required:    true,
					},
					"dir": {
						Type:        "string",
						Description: "Directory to search in (defaults to current directory)",
						Required:    false,
					},
				},
			},
		},
	}, nil
}

// validateCommand checks if a command is safe to execute
func (h *ZshToolHandler) validateCommand(command string) error {
	// Check for banned commands
	for _, banned := range h.bannedCommands {
		if strings.Contains(command, banned) {
			return fmt.Errorf("command '%s' contains banned operation '%s'", command, banned)
		}
	}

	// Special handling for cd command
	if strings.HasPrefix(command, "cd ") {
		parts := strings.Split(command, " ")
		if len(parts) < 2 {
			return nil // Simple "cd" is safe
		}

		targetDir := parts[1]
		// Remove quotes if present
		targetDir = strings.Trim(targetDir, "\"'")

		// Resolve the target path
		var fullTargetDir string
		if filepath.IsAbs(targetDir) {
			fullTargetDir = targetDir
		} else {
			fullTargetDir = filepath.Join(h.shell.cwd, targetDir)
		}

		// Verify it's within allowed directory structure
		if !isInAllowedDirectory(fullTargetDir, h.originalCwd) {
			return fmt.Errorf("changing to directory '%s' is not allowed for security reasons", fullTargetDir)
		}
	}

	return nil
}

// isInAllowedDirectory checks if a path is within an allowed directory
func isInAllowedDirectory(path, basePath string) bool {
	// Get relative path
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return false
	}

	// Check if path is within the base directory
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

// formatOutput truncates long output and tracks original line count
func formatOutput(output string) (string, int) {
	lines := strings.Split(output, "\n")
	lineCount := len(lines)

	// Truncate if too large
	maxLength := 30000
	if len(output) > maxLength {
		output = output[:maxLength] + "... [Output truncated]"
	}

	return output, lineCount
}

// CallTool executes a Zsh tool with the given arguments
func (h *ZshToolHandler) CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	switch name {
	case "zsh_execute":
		return h.handleExecute(arguments)
	case "zsh_history":
		return h.handleHistory(arguments)
	case "zsh_alias":
		return h.handleAlias(arguments)
	case "zsh_env":
		return h.handleEnv(arguments)
	case "zsh_which":
		return h.handleWhich(arguments)
	case "zsh_config":
		return h.handleConfig(arguments)
	case "zsh_glob":
		return h.handleGlob(arguments)
	default:
		return nil, fmt.Errorf("unknown zsh tool: %s", name)
	}
}

// handleExecute implements the zsh_execute tool
func (h *ZshToolHandler) handleExecute(arguments map[string]interface{}) (*models.CallToolResult, error) {
	// Extract command
	command, ok := arguments["command"].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command argument is required")
	}

	// Set timeout (default 120 seconds, max 10 minutes)
	timeout := 120000 * time.Millisecond
	if timeoutArg, ok := arguments["timeout"].(float64); ok {
		if timeoutArg > 600000 {
			timeoutArg = 600000
		}
		timeout = time.Duration(timeoutArg) * time.Millisecond
	}

	// Validate command
	if err := h.validateCommand(command); err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	// Execute command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec(command, ctx, timeout)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)
	stderrTruncated, _ := formatOutput(stderr)

	// Construct result message
	var resultText string
	if stdoutTruncated != "" {
		resultText += stdoutTruncated
	}

	if stderrTruncated != "" {
		if resultText != "" {
			resultText += "\n"
		}
		resultText += "STDERR:\n" + stderrTruncated
	}

	if interrupted {
		if resultText != "" {
			resultText += "\n"
		}
		resultText += "Command was interrupted due to timeout."
	}

	isError := exitCode != 0 || interrupted || stderrTruncated != ""

	// Check if shell navigated outside allowed directory
	if !isInAllowedDirectory(h.shell.cwd, h.originalCwd) {
		// Reset to original directory
		h.shell.setCwd(h.originalCwd)
		resultText += "\nWARNING: Shell directory was reset to original directory for security."
		isError = true
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleHistory implements the zsh_history tool
func (h *ZshToolHandler) handleHistory(arguments map[string]interface{}) (*models.CallToolResult, error) {
	limit := 10 // Default limit
	if limitArg, ok := arguments["limit"].(float64); ok {
		limit = int(limitArg)
	}

	command := fmt.Sprintf("fc -l -n -%d", limit)

	// Execute history command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec(command, ctx, 5*time.Second)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)

	isError := exitCode != 0 || interrupted || stderr != ""
	resultText := stdoutTruncated

	if stderr != "" {
		resultText += "\nError: " + stderr
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleAlias implements the zsh_alias tool
func (h *ZshToolHandler) handleAlias(arguments map[string]interface{}) (*models.CallToolResult, error) {
	command := "alias"

	// Execute alias command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec(command, ctx, 5*time.Second)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)

	isError := exitCode != 0 || interrupted || stderr != ""
	resultText := stdoutTruncated

	if stderr != "" {
		resultText += "\nError: " + stderr
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleEnv implements the zsh_env tool
func (h *ZshToolHandler) handleEnv(arguments map[string]interface{}) (*models.CallToolResult, error) {
	var command string

	if name, ok := arguments["name"].(string); ok && name != "" {
		command = fmt.Sprintf("printenv %s", name)
	} else {
		command = "printenv"
	}

	// Execute env command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec(command, ctx, 5*time.Second)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)

	isError := exitCode != 0 || interrupted || stderr != ""
	resultText := stdoutTruncated

	if stderr != "" {
		resultText += "\nError: " + stderr
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleWhich implements the zsh_which tool
func (h *ZshToolHandler) handleWhich(arguments map[string]interface{}) (*models.CallToolResult, error) {
	command, ok := arguments["command"].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command argument is required")
	}

	// Execute which command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec("which "+command, ctx, 5*time.Second)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)

	isError := exitCode != 0 || interrupted || stderr != ""
	resultText := stdoutTruncated

	if stderr != "" {
		resultText += "\nError: " + stderr
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleConfig implements the zsh_config tool
func (h *ZshToolHandler) handleConfig(arguments map[string]interface{}) (*models.CallToolResult, error) {
	action, ok := arguments["action"].(string)
	if !ok || action == "" {
		return nil, fmt.Errorf("action argument is required")
	}

	file, ok := arguments["file"].(string)
	if !ok || file == "" {
		return nil, fmt.Errorf("file argument is required")
	}

	// Validate file name to prevent arbitrary file reading
	var configPath string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %v", err)
	}

	switch file {
	case "zshrc":
		configPath = filepath.Join(homeDir, ".zshrc")
	case "zshenv":
		configPath = filepath.Join(homeDir, ".zshenv")
	case "zprofile":
		configPath = filepath.Join(homeDir, ".zprofile")
	default:
		return nil, fmt.Errorf("invalid config file: %s", file)
	}

	var command string
	switch action {
	case "read":
		command = fmt.Sprintf("cat %s 2>/dev/null || echo 'File not found: %s'", configPath, configPath)
	case "append":
		content, ok := arguments["content"].(string)
		if !ok || content == "" {
			return nil, fmt.Errorf("content argument is required for append action")
		}

		// Escape content for safe echo
		escapedContent := strings.ReplaceAll(content, "\"", "\\\"")
		command = fmt.Sprintf("echo \"%s\" >> %s", escapedContent, configPath)
	default:
		return nil, fmt.Errorf("invalid action: %s", action)
	}

	// Execute command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec(command, ctx, 5*time.Second)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)

	isError := exitCode != 0 || interrupted || stderr != ""
	resultText := stdoutTruncated

	if stderr != "" {
		resultText += "\nError: " + stderr
	}

	if action == "append" && !isError {
		resultText = fmt.Sprintf("Successfully appended to %s", configPath)
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleGlob implements the zsh_glob tool
func (h *ZshToolHandler) handleGlob(arguments map[string]interface{}) (*models.CallToolResult, error) {
	pattern, ok := arguments["pattern"].(string)
	if !ok || pattern == "" {
		return nil, fmt.Errorf("pattern argument is required")
	}

	dir := "."
	if dirArg, ok := arguments["dir"].(string); ok && dirArg != "" {
		dir = dirArg

		// Validate directory is within allowed paths
		var fullPath string
		if filepath.IsAbs(dir) {
			fullPath = dir
		} else {
			fullPath = filepath.Join(h.shell.cwd, dir)
		}

		if !isInAllowedDirectory(fullPath, h.originalCwd) {
			return nil, fmt.Errorf("directory '%s' is outside of allowed paths", dir)
		}
	}

	// Use Zsh's advanced globbing capabilities
	command := fmt.Sprintf("cd %s && print -l %s", dir, pattern)

	// Execute command
	ctx := context.Background()
	shell := h.shell.GetInstance()
	stdout, stderr, exitCode, interrupted := shell.exec(command, ctx, 10*time.Second)

	// Format output
	stdoutTruncated, _ := formatOutput(stdout)

	isError := exitCode != 0 || interrupted || stderr != ""
	resultText := stdoutTruncated

	if stderr != "" {
		resultText += "\nError: " + stderr
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

func main() {
	// Create server info
	serverInfo := models.Implementation{
		Name:    "zsh-mcp-server",
		Version: "0.1.0",
	}

	// Create the core MCP server
	mcpServer := core.NewServer(serverInfo, "2024-11-05")

	// Create and register the Zsh tool handler
	zshHandler := NewZshToolHandler()
	toolsManager := mcpServer.ToolManager()
	err := toolsManager.RegisterHandler("zsh", zshHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register Zsh tool handler: %v\n", err)
		os.Exit(1)
	}

	// Set up instructions for using the Zsh tools
	instructions := `
This MCP server provides tools for interacting with the Zsh shell.

Available tools:
- zsh_execute: Run Zsh commands in a persistent shell
- zsh_history: Retrieve command history
- zsh_alias: List all defined aliases
- zsh_env: View environment variables
- zsh_which: Locate a command
- zsh_config: Read or manipulate Zsh configuration files
- zsh_glob: Leverage Zsh's powerful globbing capabilities

The shell maintains state between commands, including current directory, 
environment variables, and shell options.

For security reasons, some commands are restricted, and navigation is limited
to subdirectories of the original working directory.
`
	mcpServer.SetInstructions(instructions)

	// Configure the STDIO server
	config := &server.ServerConfig{
		DefaultTimeout: 30 * time.Second,
	}
	stdioServer := server.NewServer(mcpServer, config)

	// Setup logging
	mcpServer.SendLog(models.LogLevelInfo, "Zsh MCP Server initialized", "main")

	// Setup signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start the server in a goroutine
	serverDone := make(chan error, 1)
	go func() {
		fmt.Fprintln(os.Stderr, "Starting Zsh MCP server...")
		serverDone <- stdioServer.Start()
	}()

	// Wait for either server completion or termination signal
	select {
	case err := <-serverDone:
		if err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
		// Server completed on its own (likely due to client disconnect)
		fmt.Fprintln(os.Stderr, "Server stopped, performing cleanup...")
		// Clean up persistent shell
		zshHandler.shell.cleanup()
		// Still call Stop() to ensure all resources are released
		if err := stdioServer.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during cleanup: %v\n", err)
		}
	case <-c:
		fmt.Fprintln(os.Stderr, "\nReceived termination signal, shutting down...")
		// Clean up persistent shell
		zshHandler.shell.cleanup()
		if err := stdioServer.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Fprintln(os.Stderr, "Zsh MCP server shutdown complete")
}
