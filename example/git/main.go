package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	models "github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/server"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
)

// GitToolHandler implements a tool handler for Git operations.
type GitToolHandler struct {
	repoPath string
}

// NewGitToolHandler creates a new Git tool handler for the given repository.
func NewGitToolHandler(repoPath string) *GitToolHandler {
	return &GitToolHandler{
		repoPath: repoPath,
	}
}

// ListTools returns a list of available Git tools.
func (h *GitToolHandler) ListTools() ([]models.Tool, error) {
	return []models.Tool{
		{
			Name:        "git_status",
			Description: "Show the working tree status",
			InputSchema: models.InputSchema{
				Type:       "object",
				Properties: map[string]models.ParameterSchema{},
			},
		},
		{
			Name:        "git_log",
			Description: "Show commit logs",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"max_count": {
						Type:        "number",
						Description: "Limit the number of commits to output",
						Required:    false,
					},
					"pretty": {
						Type:        "string",
						Description: "Pretty-print the contents of the commit logs",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "git_branch",
			Description: "List, create, or delete branches",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"all": {
						Type:        "boolean",
						Description: "List both remote-tracking branches and local branches",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "git_diff",
			Description: "Show changes between commits, commit and working tree, etc.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"path": {
						Type:        "string",
						Description: "Specific file path to show diff for",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "git_show",
			Description: "Show various types of objects (commits, tags, etc.)",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"object": {
						Type:        "string",
						Description: "The object to show (commit hash, branch name, etc.)",
						Required:    false,
					},
				},
			},
		},
		{
			Name:        "git_blame",
			Description: "Show what revision and author last modified each line of a file",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.ParameterSchema{
					"file": {
						Type:        "string",
						Description: "File path to blame",
						Required:    true,
					},
				},
			},
		},
	}, nil
}

// runGitCommand executes a git command and returns its output.
func (h *GitToolHandler) runGitCommand(ctx context.Context, args ...string) (string, error) {
	// Create the command
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = h.repoPath

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git command failed: %v\nError: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// CallTool executes a Git tool with the given arguments.
func (h *GitToolHandler) CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var output string
	var err error

	switch name {
	case "status":
		output, err = h.runGitCommand(ctx, "status")

	case "log":
		args := []string{"log"}

		if nStr, ok := arguments["n"].(string); ok { // Check for "n" as string
			// Convert string to int
			if nInt, err := strconv.Atoi(nStr); err == nil && nInt > 0 {
				args = append(args, fmt.Sprintf("--max-count=%d", nInt)) // Use converted int
			}
		}
		// Add optional arguments
		if maxCount, ok := arguments["max_count"].(float64); ok {
			args = append(args, fmt.Sprintf("--max-count=%d", int(maxCount)))
		}

		if pretty, ok := arguments["pretty"].(string); ok {
			args = append(args, fmt.Sprintf("--pretty=%s", pretty))
		}

		output, err = h.runGitCommand(ctx, args...)

	case "branch":
		args := []string{"branch"}

		// Add optional arguments
		if all, ok := arguments["all"].(bool); ok && all {
			args = append(args, "--all")
		}

		output, err = h.runGitCommand(ctx, args...)

	case "diff":
		args := []string{"diff"}

		// Add optional path argument
		if path, ok := arguments["path"].(string); ok && path != "" {
			args = append(args, "--", path)
		}

		output, err = h.runGitCommand(ctx, args...)

	case "show":
		args := []string{"show"}

		// Add optional object argument
		if object, ok := arguments["object"].(string); ok && object != "" {
			args = append(args, object)
		}

		output, err = h.runGitCommand(ctx, args...)

	case "blame":
		file, ok := arguments["file"].(string)
		if !ok || file == "" {
			return nil, fmt.Errorf("file argument is required")
		}

		args := []string{"blame", file}
		output, err = h.runGitCommand(ctx, args...)

	default:
		return nil, fmt.Errorf("unknown git tool: %s", name)
	}

	if err != nil {
		return &models.CallToolResult{
			Content: []models.Content{models.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error: %v", err),
			}},
			IsError: true,
		}, nil
	}

	return &models.CallToolResult{
		Content: []models.Content{models.TextContent{
			Type: "text",
			Text: output,
		}},
	}, nil
}

// detectGitRepository attempts to find a Git repository in the current directory
// or its parent directories.
func detectGitRepository() (string, error) {
	// Start with the current directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if this directory is a Git repository
	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the root directory
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no Git repository found in the current directory or its parents")
}

// addGitResourceRoots adds Git repository structure as roots to the MCP server.
func addGitResourceRoots(mcpServer *core.Server, repoPath string) error {
	// Add the main repository root
	root := models.Root{
		URI:  fmt.Sprintf("file://%s", repoPath),
		Name: "Git Repository",
	}
	if err := mcpServer.AddRoot(root); err != nil {
		return err
	}

	return nil
}

// setupGitPrompts adds Git-related prompts to the prompt manager.
func setupGitPrompts(mcpServer *core.Server) {
	// This would be implemented to add Git-specific prompt templates
	// to help the client understand how to work with the Git repository
}

func main() {

	repoPathFlag := flag.String("repo", "", "Path to Git repository")

	flag.Parse()

	// Create server info
	serverInfo := models.Implementation{
		Name:    "git-mcp-server",
		Version: "0.0.1",
	}

	// Create the core MCP server
	mcpServer := core.NewServer(serverInfo, "2024-11-05")
	// Use provided repository path or detect automatically
	var repoPath string
	var err error

	if *repoPathFlag != "" {
		// Use the path provided via command-line
		repoPath = *repoPathFlag

		// Verify it's a valid Git repository
		gitDir := filepath.Join(repoPath, ".git")
		if _, statErr := os.Stat(gitDir); statErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Specified path does not appear to be a Git repository: %v\n", statErr)
			fmt.Fprintf(os.Stderr, "Will attempt automatic detection instead.\n")

			// Fall back to automatic detection
			repoPath, err = detectGitRepository()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				fmt.Fprintf(os.Stderr, "Using current directory instead\n")
				repoPath, _ = os.Getwd()
			}
		} else {
			fmt.Fprintf(os.Stderr, "Using specified Git repository: %s\n", repoPath)
		}
	} else {
		// No path provided, use automatic detection
		repoPath, err = detectGitRepository()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			fmt.Fprintf(os.Stderr, "Using current directory instead\n")
			repoPath, _ = os.Getwd()
		}
	}

	// Register Git tools
	gitHandler := NewGitToolHandler(repoPath)

	toolsManager := mcpServer.ToolManager()
	err = toolsManager.RegisterHandler("git", gitHandler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register Git tool handler: %v\n", err)
		os.Exit(1)
	}

	// Add Git repository as a resource root
	err = addGitResourceRoots(mcpServer, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to add Git repository as root: %v\n", err)
	}

	// Setup Git-related prompts
	setupGitPrompts(mcpServer)

	// Configure the STDIO server
	config := &server.ServerConfig{
		DefaultTimeout: 60 * time.Second,
		Logger:         logging.NewStdLogger(logging.DebugLevel),
	}
	stdioServer := server.NewServer(mcpServer, config)

	// Setup logging
	mcpServer.SendLog(models.LogLevelInfo, fmt.Sprintf("Git MCP Server initialized with repository at: %s", repoPath), "main")

	// Setup signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start the server in a goroutine
	serverDone := make(chan error, 1)
	go func() {
		fmt.Fprintln(os.Stderr, "Starting Git MCP server...")
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

	fmt.Fprintln(os.Stderr, "Git MCP server shutdown complete")
}
