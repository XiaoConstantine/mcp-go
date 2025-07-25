package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/client"
	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	models "github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/transport"
)

// BashMCPClient wraps the MCP client and provides bash-specific functionality.
type BashMCPClient struct {
	mcpClient *client.Client
	logger    logging.Logger
	cmd       *exec.Cmd
}

// NewBashMCPClient creates a new bash MCP client.
func NewBashMCPClient() (*BashMCPClient, error) {
	// Start the bash server as a subprocess
	// Use the compiled binary
	cmd := exec.Command("./bash-mcp-server")

	// Set up pipes for communication
	serverIn, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %v", err)
	}

	serverOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	// Redirect server's stderr to our stderr for debugging
	cmd.Stderr = os.Stderr

	// Start the server
	fmt.Println("Starting Bash MCP server...")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %v", err)
	}

	// Give the server a moment to initialize
	fmt.Println("Waiting for server to initialize...")
	time.Sleep(1 * time.Second)

	// Create a logger
	logger := logging.NewStdLogger(logging.InfoLevel)

	// Create a StdioTransport
	transport := transport.NewStdioTransport(serverOut, serverIn, logger)

	// Create the MCP client
	mcpClient := client.NewClient(
		transport,
		client.WithLogger(logger),
		client.WithClientInfo("bash-mcp-client", "0.1.0"),
	)

	return &BashMCPClient{
		mcpClient: mcpClient,
		logger:    logger,
		cmd:       cmd,
	}, nil
}

// Initialize initializes the connection to the bash MCP server.
func (c *BashMCPClient) Initialize() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize the client
	initResult, err := c.mcpClient.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize client: %v", err)
	}

	c.logger.Info("Connected to MCP server: %s %s",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version)
	c.logger.Info("Protocol version: %s", initResult.ProtocolVersion)

	// Display server instructions if provided  
	if initResult.Instructions != "" {
		fmt.Println("\n=== Server Instructions ===")
		fmt.Println(initResult.Instructions)
		fmt.Println("===========================")
	}

	return nil
}

// ListTools lists all available tools.
func (c *BashMCPClient) ListTools() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	toolsResult, err := c.mcpClient.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list tools: %v", err)
	}

	fmt.Println("Available tools:")
	for _, tool := range toolsResult.Tools {
		fmt.Printf("- %s: %s\n", tool.Name, tool.Description)
		
		// Show input schema if available
		if len(tool.InputSchema.Properties) > 0 {
			fmt.Println("  Parameters:")
			for paramName, param := range tool.InputSchema.Properties {
				required := ""
				if param.Required {
					required = " (required)"
				}
				fmt.Printf("    - %s (%s)%s: %s\n", paramName, param.Type, required, param.Description)
			}
		}
		fmt.Println()
	}

	return nil
}

// ExecuteCommand executes a bash command using the bash_execute tool.
func (c *BashMCPClient) ExecuteCommand(command string, timeout float64, workingDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare arguments
	args := map[string]interface{}{
		"command": command,
	}

	if timeout > 0 {
		args["timeout"] = timeout
	}

	if workingDir != "" {
		args["working_dir"] = workingDir
	}

	// Call the tool
	result, err := c.mcpClient.CallTool(ctx, "bash_execute", args)
	if err != nil {
		return fmt.Errorf("failed to execute command: %v", err)
	}

	// Display result
	if result.IsError {
		fmt.Println("Command execution resulted in an error:")
	} else {
		fmt.Println("Command executed successfully:")
	}

	for _, content := range result.Content {
		if textContent, ok := content.(models.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}

	return nil
}

// ExecuteScript executes a multi-line bash script using the bash_script tool.
func (c *BashMCPClient) ExecuteScript(script string, timeout float64, workingDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare arguments
	args := map[string]interface{}{
		"script": script,
	}

	if timeout > 0 {
		args["timeout"] = timeout
	}

	if workingDir != "" {
		args["working_dir"] = workingDir
	}

	// Call the tool
	result, err := c.mcpClient.CallTool(ctx, "bash_script", args)
	if err != nil {
		return fmt.Errorf("failed to execute script: %v", err)
	}

	// Display result
	if result.IsError {
		fmt.Println("Script execution resulted in an error:")
	} else {
		fmt.Println("Script executed successfully:")
	}

	for _, content := range result.Content {
		if textContent, ok := content.(models.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}

	return nil
}

// CallTool calls any available tool with the provided arguments.
func (c *BashMCPClient) CallTool(toolName string, args map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := c.mcpClient.CallTool(ctx, toolName, args)
	if err != nil {
		return fmt.Errorf("failed to call tool %s: %v", toolName, err)
	}

	// Display result
	if result.IsError {
		fmt.Printf("Tool '%s' resulted in an error:\n", toolName)
	} else {
		fmt.Printf("Tool '%s' executed successfully:\n", toolName)
	}

	for _, content := range result.Content {
		if textContent, ok := content.(models.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}

	return nil
}

// Shutdown shuts down the client and server.
func (c *BashMCPClient) Shutdown() error {
	// Shutdown the MCP client
	if err := c.mcpClient.Shutdown(); err != nil {
		c.logger.Error("Error shutting down client: %v", err)
	}

	// Terminate the server subprocess
	if c.cmd != nil && c.cmd.Process != nil {
		if err := c.cmd.Process.Signal(os.Interrupt); err != nil {
			c.logger.Error("Failed to send interrupt signal to server: %v", err)
			if err := c.cmd.Process.Kill(); err != nil {
				c.logger.Error("Failed to kill server: %v", err)

				// Check if process is still running despite Kill failure
				if process, err := os.FindProcess(c.cmd.Process.Pid); err == nil {
					// On Unix systems, FindProcess always succeeds, so check if process can be signaled
					if err := process.Signal(syscall.Signal(0)); err == nil {
						c.logger.Info("Attempting alternate termination for PID %d", c.cmd.Process.Pid)

						// Use OS-specific forceful termination
						if runtime.GOOS == "windows" {
							killCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(c.cmd.Process.Pid))
							if err := killCmd.Run(); err != nil {
								c.logger.Error("Forceful termination with taskkill failed: %v", err)
							}
						} else {
							killCmd := exec.Command("kill", "-9", strconv.Itoa(c.cmd.Process.Pid))
							if err := killCmd.Run(); err != nil {
								c.logger.Error("Forceful termination with kill -9 failed: %v", err)
							}
						}
					}
				}
			}
		}

		// Wait for the server to exit
		if err := c.cmd.Wait(); err != nil {
			c.logger.Error("Server exited with error: %v", err)
		}
	}

	return nil
}

// Interactive mode allows the user to interact with the bash server.
func (c *BashMCPClient) Interactive() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Bash MCP Client Interactive Mode")
	fmt.Println("Commands:")
	fmt.Println("  help           - Show this help")
	fmt.Println("  tools          - List available tools")
	fmt.Println("  exec <cmd>     - Execute a bash command")
	fmt.Println("  script         - Execute a multi-line script (end with empty line)")
	fmt.Println("  env [name]     - Show environment variables")
	fmt.Println("  which <cmd>    - Locate a command")
	fmt.Println("  history [n]    - Show command history")
	fmt.Println("  alias          - List aliases")
	fmt.Println("  jobs           - List active jobs")
	fmt.Println("  pwd            - Show current directory")
	fmt.Println("  quit/exit      - Exit the client")
	fmt.Println()

	for {
		fmt.Print("bash-mcp> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		command := parts[0]

		switch command {
		case "help":
			fmt.Println("Available commands:")
			fmt.Println("  help, tools, exec, script, env, which, history, alias, jobs, pwd, quit, exit")

		case "tools":
			if err := c.ListTools(); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "exec":
			if len(parts) < 2 {
				fmt.Println("Usage: exec <command>")
				continue
			}
			cmd := strings.Join(parts[1:], " ")
			if err := c.ExecuteCommand(cmd, 0, ""); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "script":
			fmt.Println("Enter your script (press Enter on empty line to execute):")
			var lines []string
			for {
				fmt.Print("... ")
				if !scanner.Scan() {
					break
				}
				line := scanner.Text()
				if line == "" {
					break
				}
				lines = append(lines, line)
			}
			script := strings.Join(lines, "\n")
			if script != "" {
				if err := c.ExecuteScript(script, 0, ""); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			}

		case "env":
			args := map[string]interface{}{}
			if len(parts) > 1 {
				args["name"] = parts[1]
			}
			if err := c.CallTool("bash_env", args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "which":
			if len(parts) < 2 {
				fmt.Println("Usage: which <command>")
				continue
			}
			args := map[string]interface{}{
				"command": parts[1],
			}
			if err := c.CallTool("bash_which", args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "history":
			args := map[string]interface{}{}
			if len(parts) > 1 {
				if limit, err := strconv.Atoi(parts[1]); err == nil {
					args["limit"] = float64(limit)
				}
			}
			if err := c.CallTool("bash_history", args); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "alias":
			if err := c.CallTool("bash_alias", map[string]interface{}{}); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "jobs":
			if err := c.CallTool("bash_jobs", map[string]interface{}{}); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "pwd":
			if err := c.CallTool("bash_pwd", map[string]interface{}{}); err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "quit", "exit":
			fmt.Println("Exiting...")
			return

		default:
			fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", command)
		}
	}
}

func main() {
	// Create the client
	client, err := NewBashMCPClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Set up signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Setup shutdown in a goroutine
	go func() {
		<-c
		fmt.Println("\nShutting down...")
		if err := client.Shutdown(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		}
		os.Exit(0)
	}()

	// Initialize the client
	if err := client.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
		if err := client.Shutdown(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		}
		os.Exit(1)
	}

	// Check if running in interactive mode or command mode
	if len(os.Args) > 1 {
		// Command mode - execute the provided command
		command := strings.Join(os.Args[1:], " ")
		
		if strings.HasPrefix(command, "script:") {
			// Handle script execution
			script := strings.TrimPrefix(command, "script:")
			if err := client.ExecuteScript(script, 0, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				if err := client.Shutdown(); err != nil {
				fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
			}
				os.Exit(1)
			}
		} else {
			// Handle single command execution
			if err := client.ExecuteCommand(command, 0, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				if err := client.Shutdown(); err != nil {
				fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
			}
				os.Exit(1)
			}
		}
	} else {
		// Interactive mode
		client.Interactive()
	}

	// Shutdown the client
	if err := client.Shutdown(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		os.Exit(1)
	}
}