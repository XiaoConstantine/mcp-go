# MCP-Go

[![Unit and Integration Tests](https://github.com/XiaoConstantine/mcp-go/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/XiaoConstantine/mcp-go/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/XiaoConstantine/mcp-go/graph/badge.svg?token=RFXBU53AH9)](https://codecov.io/gh/XiaoConstantine/mcp-go)

A Go implementation of the Model Context Protocol (MCP), a standardized protocol for communication between LLM clients and context providers.

## What is MCP?

The Model Context Protocol (MCP) is a standardized communication protocol that enables large language models (LLMs) to access contextual information and tools from external providers. It creates a clear interface between:

- **LLM clients** (applications that interact with language models)
- **Context providers** (services that provide data, functionality, or tools)

Key features of MCP:

- Bi-directional communication through a JSON-RPC based protocol
- Support for resource discovery and reading
- Tool invocation capabilities
- Prompt templates
- Notification system for resource updates
- Structured logging

## Overview

This library provides a complete implementation of the MCP specification in Go, including:

- Client implementation for connecting to MCP servers
- Server framework for building context providers
- Transport layer with support for standard I/O communication
- Complete type definitions for the MCP protocol
- Utilities for logging and error handling

## Installation

```bash
go get github.com/XiaoConstantine/mcp-go
```

## Getting Started

### Creating an MCP Client

```go
import (
    "context"
    "fmt"
    "time"
    
    "github.com/XiaoConstantine/mcp-go/pkg/client"
    "github.com/XiaoConstantine/mcp-go/pkg/logging"
    "github.com/XiaoConstantine/mcp-go/pkg/transport"
)

func main() {
    // Create a logger
    logger := logging.NewStdLogger(logging.InfoLevel)
    
    // Create a transport (using standard I/O in this example)
    t := transport.NewStdioTransport(serverOut, serverIn, logger)
    
    // Create the client
    mcpClient := client.NewClient(
        t,
        client.WithLogger(logger),
        client.WithClientInfo("my-mcp-client", "1.0.0"),
    )
    
    // Set a timeout for initialization
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Initialize the client
    initResult, err := mcpClient.Initialize(ctx)
    if err != nil {
        fmt.Printf("Failed to initialize client: %v\n", err)
        return
    }
    
    fmt.Printf("Connected to MCP server: %s %s\n", 
        initResult.ServerInfo.Name, 
        initResult.ServerInfo.Version)
    
    // Use the client to access tools, resources, etc.
    // ...
    
    // Shutdown when done
    mcpClient.Shutdown()
}
```

### Building an MCP Server

```go
import (
    "fmt"
    "time"
    
    "github.com/XiaoConstantine/mcp-go/pkg/model"
    "github.com/XiaoConstantine/mcp-go/pkg/server"
    "github.com/XiaoConstantine/mcp-go/pkg/server/core"
)

func main() {
    // Create server info
    serverInfo := models.Implementation{
        Name:    "my-mcp-server",
        Version: "1.0.0",
    }
    
    // Create the core MCP server
    mcpServer := core.NewServer(serverInfo, "2024-11-05")
    
    // Register tools, resources, etc.
    // ...
    
    // Configure the server
    config := &server.ServerConfig{
        DefaultTimeout: 60 * time.Second,
    }
    
    // Create a stdio server
    stdioServer := server.NewServer(mcpServer, config)
    
    // Start the server
    fmt.Println("Starting MCP server...")
    if err := stdioServer.Start(); err != nil {
        fmt.Printf("Server error: %v\n", err)
    }
}
```

## Examples

The repository includes several example implementations:

- **Git MCP Server**: Provides Git repository information as MCP resources and Git commands as tools
- **Shell MCP Server**: Exposes shell commands as MCP tools
- **Client Example**: Demonstrates how to connect to and interact with MCP servers

See the `example/` directory for complete examples.

## API Documentation

The library is organized into several key packages:

- `pkg/client`: Client implementation for connecting to MCP servers
- `pkg/server`: Server implementation for creating MCP context providers
- `pkg/transport`: Transport layer implementations (currently supporting stdio)
- `pkg/model`: Type definitions for MCP protocol objects
- `pkg/protocol`: Core protocol implementation (JSON-RPC, capabilities)
- `pkg/errors`: Error definitions and handling
- `pkg/logging`: Logging utilities
- `pkg/handler`: Handler interfaces for server-side functionality

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the terms of the MIT license.
