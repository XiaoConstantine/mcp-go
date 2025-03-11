package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/client"
	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	models "github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/transport"
)

func main() {
	// Start the server as a subprocess
	// For this example, we'll use the Git MCP server
	cmd := exec.Command("./git-mcp-server")

	// Set up pipes for stdin and stdout
	serverIn, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create stdin pipe: %v\n", err)
		os.Exit(1)
	}

	serverOut, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create stdout pipe: %v\n", err)
		os.Exit(1)
	}

	// Redirect server's stderr to our stderr for debugging
	cmd.Stderr = os.Stderr

	// Start the server
	fmt.Println("Starting Git MCP server...")
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		os.Exit(1)
	}

	// Give the server a moment to initialize
	fmt.Println("Waiting for server to initialize...")
	time.Sleep(1 * time.Second)

	// Create a logger with DEBUG level for detailed logging
	logger := logging.NewStdLogger(logging.DebugLevel)

	// Create a StdioTransport with the logger
	// Note that we connect our stdout to the server's stdin, and our stdin to the server's stdout
	t := transport.NewStdioTransport(serverOut, serverIn, logger)

	// We already created the logger above
	// Give the server a moment to initialize
	time.Sleep(500 * time.Millisecond)

	// Create the client with options
	mcpClient := client.NewClient(
		t,
		client.WithLogger(logger),
		client.WithClientInfo("example-mcp-client", "0.1.0"),
	)

	// Set a timeout for initialization
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize the client
	initResult, err := mcpClient.Initialize(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize client: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Connected to MCP server: %s %s\n",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version)
	logger.Info("Protocol version: %s\n", initResult.ProtocolVersion)

	// Handle server instructions if provided
	if initResult.Instructions != "" {
		logger.Info("Server instructions: %s\n", initResult.Instructions)
	}

	// Start notification handler
	notificationDone := make(chan struct{})
	go func() {
		defer close(notificationDone)

		notifChan := mcpClient.Notifications()
		for notification := range notifChan {
			if notification.Method == "notifications/resources/updated" {
				logger.Info("Resource update notification received")
				// Handle resource update
			} else {
				logger.Info("Notification received: %s\n", notification.Method)
			}
		}
	}()

	// List available tools
	toolsResult, err := mcpClient.ListTools(ctx, nil)
	if err != nil {
		logger.Error("Failed to list tools: %v\n", err)
	} else {
		logger.Info("Available tools:")
		for _, tool := range toolsResult.Tools {
			logger.Info("- %s: %s\n", tool.Name, tool.Description)
		}
	}

	// Call a git tool (for example, git status)
	toolResult, err := mcpClient.CallTool(ctx, "git_status", map[string]interface{}{})
	if err != nil {
		logger.Error("Failed to call git_status: %v\n", err)
	} else {
		if toolResult.IsError {
			fmt.Println("Tool returned an error:")
		} else {
			fmt.Println("Git status result:")
		}

		// Print the tool result content
		for _, content := range toolResult.Content {
			if textContent, ok := content.(models.TextContent); ok {
				fmt.Println(textContent.Text)
			}
		}
	}

	// List available resources
	resourcesResult, err := mcpClient.ListResources(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list resources: %v\n", err)
	} else {
		fmt.Println("Available resources:")
		for _, resource := range resourcesResult.Resources {
			fmt.Printf("- %s: %s\n", resource.Name, resource.URI)
		}
	}

	// Shutdown the client
	if err := mcpClient.Shutdown(); err != nil {
		fmt.Fprintf(os.Stderr, "Error shutting down client: %v\n", err)
	}

	// Wait for notification handler to finish
	<-notificationDone

	// Terminate the server subprocess
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send interrupt signal to server: %v\n", err)
		if err := cmd.Process.Kill(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to kill server: %v\n", err)

			// Check if process is still running despite Kill failure
			if process, err := os.FindProcess(cmd.Process.Pid); err == nil {
				// On Unix systems, FindProcess always succeeds, so check if process can be signaled
				if err := process.Signal(syscall.Signal(0)); err == nil {
					fmt.Fprintf(os.Stderr, "Attempting alternate termination for PID %d\n", cmd.Process.Pid)

					// As a last resort, use OS-specific forceful termination with proper error handling
					if runtime.GOOS == "windows" {
						killCmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
						if err := killCmd.Run(); err != nil {
							fmt.Fprintf(os.Stderr, "Forceful termination with taskkill failed: %v\n", err)
						}
					} else {
						killCmd := exec.Command("kill", "-9", strconv.Itoa(cmd.Process.Pid))
						if err := killCmd.Run(); err != nil {
							fmt.Fprintf(os.Stderr, "Forceful termination with kill -9 failed: %v\n", err)
						}
					}
				}
			}

			fmt.Fprintf(os.Stderr, "Process handling completed, but state may be uncertain (PID: %d)\n", cmd.Process.Pid)
		}

	}

	// Wait for the server to exit
	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "Server exited with error: %v\n", err)
	}
}
