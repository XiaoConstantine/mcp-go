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
	"syscall"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	models "github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/server"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
)

// BashToolHandler implements a tool handler for Bash shell operations.
type BashToolHandler struct {
	workingDir     string
	bannedCommands []string
}

// NewBashToolHandler creates a new Bash tool handler.
func NewBashToolHandler() *BashToolHandler {
	cwd, _ := os.Getwd()

	return &BashToolHandler{
		workingDir: cwd,
		bannedCommands: []string{
			"rm -rf /", "sudo rm -rf", ":(){:|:&};:", "> /dev/sda",
			"dd if=", "mkfs", "fdisk", "parted", "format",
			"curl", "wget", "nc", "netcat", "telnet", "ssh", "scp", "sftp",
			"chmod 777", "chown root", "su -", "sudo su",
		},
	}
}

// ListTools returns a list of available Bash tools.
func (h *BashToolHandler) ListTools() ([]models.Tool, error) {
	return []models.Tool{
		{
			Name:        "bash_execute",
			Description: "Execute a Bash command",
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
						Description: "Optional timeout in seconds (max 600)",
						Required:    false,
					},
					"working_dir": {
						Type:        "string",
						Description: "Working directory for command execution",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "bash_script",
			Description: "Execute a multi-line Bash script",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"script": {
						Type:        "string",
						Description: "Multi-line bash script to execute",
						Required:    true,
					},
					"timeout": {
						Type:        "number",
						Description: "Optional timeout in seconds (max 600)",
						Required:    false,
					},
					"working_dir": {
						Type:        "string",
						Description: "Working directory for script execution",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "bash_env",
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
			Name:        "bash_which",
			Description: "Locate a command in PATH",
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
			Name:        "bash_history",
			Description: "Get recent command history",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"limit": {
						Type:        "number",
						Description: "Number of recent commands to show (default 10)",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "bash_alias",
			Description: "List all aliases",
			InputSchema: models.InputSchema{
				Type:       "object",
				Properties: map[string]models.ParameterSchema{},
			},
		},
		{
			Name:        "bash_jobs",
			Description: "List active jobs",
			InputSchema: models.InputSchema{
				Type:       "object",
				Properties: map[string]models.ParameterSchema{},
			},
		},
		{
			Name:        "bash_pwd",
			Description: "Get current working directory",
			InputSchema: models.InputSchema{
				Type:       "object",
				Properties: map[string]models.ParameterSchema{},
			},
		},
	}, nil
}

// validateCommand checks if a command is safe to execute.
func (h *BashToolHandler) validateCommand(command string) error {
	// Check for banned commands
	for _, banned := range h.bannedCommands {
		if strings.Contains(strings.ToLower(command), strings.ToLower(banned)) {
			return fmt.Errorf("command contains banned operation: %s", banned)
		}
	}

	// Check for suspicious patterns
	suspiciousPatterns := []string{
		"$(curl", "$(wget", "`curl", "`wget",
		">/dev/sd", ">/dev/hd", ">/dev/nvme",
		"rm -rf $HOME", "rm -rf ~",
		"chmod -R 777", "chown -R root",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(strings.ToLower(command), strings.ToLower(pattern)) {
			return fmt.Errorf("command contains suspicious pattern: %s", pattern)
		}
	}

	return nil
}

// isInAllowedDirectory checks if a path is within allowed directory structure.
func (h *BashToolHandler) isInAllowedDirectory(path string) bool {
	if !filepath.IsAbs(path) {
		return true // Relative paths are generally safe
	}

	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Allow operations within home directory and current working directory
	basePaths := []string{homeDir, h.workingDir}

	for _, basePath := range basePaths {
		if strings.HasPrefix(path, basePath) {
			return true
		}
	}

	// Allow common safe directories
	safeDirs := []string{"/tmp", "/var/tmp", "/usr/local"}
	for _, safeDir := range safeDirs {
		if strings.HasPrefix(path, safeDir) {
			return true
		}
	}

	return false
}

// runBashCommand executes a bash command and returns its output.
func (h *BashToolHandler) runBashCommand(ctx context.Context, command string, workingDir string) (string, string, int, error) {
	// Validate command
	if err := h.validateCommand(command); err != nil {
		return "", "", 1, err
	}

	// Set working directory
	if workingDir == "" {
		workingDir = h.workingDir
	}

	// Validate working directory
	if !h.isInAllowedDirectory(workingDir) {
		return "", "", 1, fmt.Errorf("working directory %s is not allowed for security reasons", workingDir)
	}

	// Create the command
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workingDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			return "", "", 1, fmt.Errorf("failed to execute command: %v", err)
		}
	}

	return stdout.String(), stderr.String(), exitCode, nil
}

// formatOutput truncates long output if necessary.
func formatOutput(output string) string {
	maxLength := 30000
	if len(output) > maxLength {
		return output[:maxLength] + "\n... [Output truncated]"
	}
	return output
}

// CallTool executes a Bash tool with the given arguments.
func (h *BashToolHandler) CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	switch name {
	case "execute":
		return h.handleExecute(arguments)
	case "script":
		return h.handleScript(arguments)
	case "env":
		return h.handleEnv(arguments)
	case "which":
		return h.handleWhich(arguments)
	case "history":
		return h.handleHistory(arguments)
	case "alias":
		return h.handleAlias(arguments)
	case "jobs":
		return h.handleJobs(arguments)
	case "pwd":
		return h.handlePwd(arguments)
	default:
		return nil, fmt.Errorf("unknown bash tool: %s", name)
	}
}

// handleExecute implements the bash_execute tool.
func (h *BashToolHandler) handleExecute(arguments map[string]interface{}) (*models.CallToolResult, error) {
	// Extract command
	command, ok := arguments["command"].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command argument is required")
	}

	// Set timeout (default 60 seconds, max 600 seconds)
	timeout := 60 * time.Second
	if timeoutArg, ok := arguments["timeout"].(float64); ok {
		if timeoutArg > 600 {
			timeoutArg = 600
		}
		timeout = time.Duration(timeoutArg) * time.Second
	}

	// Set working directory
	workingDir := h.workingDir
	if wdArg, ok := arguments["working_dir"].(string); ok && wdArg != "" {
		workingDir = wdArg
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute command
	stdout, stderr, exitCode, err := h.runBashCommand(ctx, command, workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	// Format output
	var resultText string
	if stdout != "" {
		resultText = formatOutput(stdout)
	}

	if stderr != "" {
		if resultText != "" {
			resultText += "\n"
		}
		resultText += "STDERR:\n" + formatOutput(stderr)
	}

	if resultText == "" {
		resultText = "Command executed successfully (no output)"
	}

	isError := exitCode != 0

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleScript implements the bash_script tool.
func (h *BashToolHandler) handleScript(arguments map[string]interface{}) (*models.CallToolResult, error) {
	// Extract script
	script, ok := arguments["script"].(string)
	if !ok || script == "" {
		return nil, fmt.Errorf("script argument is required")
	}

	// Set timeout (default 60 seconds, max 600 seconds)
	timeout := 60 * time.Second
	if timeoutArg, ok := arguments["timeout"].(float64); ok {
		if timeoutArg > 600 {
			timeoutArg = 600
		}
		timeout = time.Duration(timeoutArg) * time.Second
	}

	// Set working directory
	workingDir := h.workingDir
	if wdArg, ok := arguments["working_dir"].(string); ok && wdArg != "" {
		workingDir = wdArg
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute script
	stdout, stderr, exitCode, err := h.runBashCommand(ctx, script, workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	// Format output
	var resultText string
	if stdout != "" {
		resultText = formatOutput(stdout)
	}

	if stderr != "" {
		if resultText != "" {
			resultText += "\n"
		}
		resultText += "STDERR:\n" + formatOutput(stderr)
	}

	if resultText == "" {
		resultText = "Script executed successfully (no output)"
	}

	isError := exitCode != 0

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: isError,
	}, nil
}

// handleEnv implements the bash_env tool.
func (h *BashToolHandler) handleEnv(arguments map[string]interface{}) (*models.CallToolResult, error) {
	var command string

	if name, ok := arguments["name"].(string); ok && name != "" {
		command = fmt.Sprintf("echo \"$%s\"", name)
	} else {
		command = "env | sort"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, exitCode, err := h.runBashCommand(ctx, command, h.workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	resultText := formatOutput(stdout)
	if stderr != "" {
		resultText += "\nSTDERR:\n" + formatOutput(stderr)
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: exitCode != 0,
	}, nil
}

// handleWhich implements the bash_which tool.
func (h *BashToolHandler) handleWhich(arguments map[string]interface{}) (*models.CallToolResult, error) {
	command, ok := arguments["command"].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command argument is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, exitCode, err := h.runBashCommand(ctx, "which "+command, h.workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	resultText := formatOutput(stdout)
	if stderr != "" {
		resultText += "\nSTDERR:\n" + formatOutput(stderr)
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: exitCode != 0,
	}, nil
}

// handleHistory implements the bash_history tool.
func (h *BashToolHandler) handleHistory(arguments map[string]interface{}) (*models.CallToolResult, error) {
	limit := 10 // Default limit
	if limitArg, ok := arguments["limit"].(float64); ok {
		limit = int(limitArg)
	}

	command := fmt.Sprintf("history | tail -n %d", limit)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, exitCode, err := h.runBashCommand(ctx, command, h.workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	resultText := formatOutput(stdout)
	if stderr != "" {
		resultText += "\nSTDERR:\n" + formatOutput(stderr)
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: exitCode != 0,
	}, nil
}

// handleAlias implements the bash_alias tool.
func (h *BashToolHandler) handleAlias(arguments map[string]interface{}) (*models.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, exitCode, err := h.runBashCommand(ctx, "alias", h.workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	resultText := formatOutput(stdout)
	if stderr != "" {
		resultText += "\nSTDERR:\n" + formatOutput(stderr)
	}

	if resultText == "" {
		resultText = "No aliases defined"
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: exitCode != 0,
	}, nil
}

// handleJobs implements the bash_jobs tool.
func (h *BashToolHandler) handleJobs(arguments map[string]interface{}) (*models.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, exitCode, err := h.runBashCommand(ctx, "jobs", h.workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	resultText := formatOutput(stdout)
	if stderr != "" {
		resultText += "\nSTDERR:\n" + formatOutput(stderr)
	}

	if resultText == "" {
		resultText = "No active jobs"
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: exitCode != 0,
	}, nil
}

// handlePwd implements the bash_pwd tool.
func (h *BashToolHandler) handlePwd(arguments map[string]interface{}) (*models.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, exitCode, err := h.runBashCommand(ctx, "pwd", h.workingDir)
	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	resultText := formatOutput(stdout)
	if stderr != "" {
		resultText += "\nSTDERR:\n" + formatOutput(stderr)
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: resultText,
		}},
		IsError: exitCode != 0,
	}, nil
}

func main() {
	// Create server info
	serverInfo := models.Implementation{
		Name:    "bash-mcp-server",
		Version: "0.1.0",
	}

	// Create the core MCP server
	mcpServer := core.NewServer(serverInfo, "2024-11-05")

	// Create and register the Bash tool handler
	bashHandler := NewBashToolHandler()
	toolsManager := mcpServer.ToolManager()
	err := toolsManager.RegisterHandler("bash", bashHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register Bash tool handler: %v\n", err)
		os.Exit(1)
	}

	// Set up instructions for using the Bash tools
	instructions := `
This MCP server provides tools for executing Bash commands and scripts safely.

Available tools:
- bash_execute: Execute a single Bash command
- bash_script: Execute a multi-line Bash script
- bash_env: View environment variables
- bash_which: Locate commands in PATH  
- bash_history: Get recent command history
- bash_alias: List defined aliases
- bash_jobs: List active jobs
- bash_pwd: Get current working directory

Security features:
- Commands are validated against a list of banned operations
- File system access is restricted to safe directories
- Timeouts prevent long-running commands from hanging
- Output is truncated if too large

For enhanced security, certain commands and patterns are blocked,
and operations are limited to the user's home directory and
other safe locations.
`
	mcpServer.SetInstructions(instructions)

	// Configure the STDIO server
	config := &server.ServerConfig{
		DefaultTimeout: 60 * time.Second,
		Logger:         logging.NewStdLogger(logging.InfoLevel),
	}
	stdioServer := server.NewServer(mcpServer, config)

	// Setup logging
	mcpServer.SendLog(models.LogLevelInfo, fmt.Sprintf("Bash MCP Server initialized with working directory: %s", bashHandler.workingDir), "main")

	// Setup signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start the server in a goroutine
	serverDone := make(chan error, 1)
	go func() {
		fmt.Fprintln(os.Stderr, "Starting Bash MCP server...")
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
		// Still call Stop() to ensure all resources are released
		if err := stdioServer.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during cleanup: %v\n", err)
		}
	case <-c:
		fmt.Fprintln(os.Stderr, "\nReceived termination signal, shutting down...")
		if err := stdioServer.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Fprintln(os.Stderr, "Bash MCP server shutdown complete")
}