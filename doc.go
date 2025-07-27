// Package mcp provides a comprehensive Go implementation of the Model Context Protocol (MCP),
// a standardized protocol for communication between Large Language Models (LLMs) and context providers.
//
// # Overview
//
// The Model Context Protocol enables LLMs to securely access external context through a
// standardized interface. This implementation provides both client and server frameworks
// with multiple transport mechanisms, making it suitable for a wide range of applications
// from simple command-line tools to scalable web services.
//
// # Architecture
//
// The library is organized into several key packages that work together to provide
// a complete MCP implementation:
//
//	pkg/model/      - Protocol data structures and validation
//	pkg/protocol/   - JSON-RPC 2.0 implementation and capability management
//	pkg/transport/  - Multiple transport layer implementations
//	pkg/client/     - MCP client implementation
//	pkg/server/     - MCP server framework with pluggable components
//	pkg/handler/    - Message routing and handling utilities
//	pkg/errors/     - Protocol-specific error definitions
//	pkg/logging/    - Structured logging utilities
//
// # Core Features
//
//   - Full JSON-RPC 2.0 compliance with proper error handling
//   - Multiple transport mechanisms (stdio, HTTP, Server-Sent Events)
//   - Dynamic capability negotiation between clients and servers
//   - Type-safe resource and tool management
//   - Comprehensive validation and error handling
//   - Concurrent operation support with proper synchronization
//   - Progress tracking for long-running operations
//   - Session management for stateful transports
//
// # Transport Mechanisms
//
// The library supports multiple transport layers for different deployment scenarios:
//
// ## Standard I/O Transport
//
// The primary transport mechanism for process-based communication. Ideal for
// command-line tools and subprocess integration:
//
//	transport := transport.NewStdioTransport()
//	client := client.NewClient(transport)
//
// ## HTTP Transport
//
// Web-based communication with session management and connection pooling.
// Suitable for web applications and distributed systems:
//
//	config := &transport.HTTPConfig{
//		BaseURL: "https://api.example.com/mcp",
//		Timeout: 30 * time.Second,
//	}
//	transport := transport.NewHTTPTransport(config)
//
// ## Server-Sent Events (SSE)
//
// Real-time streaming capabilities for applications requiring live updates:
//
//	transport := transport.NewSSETransport(config)
//
// # Client Usage
//
// Creating and using an MCP client:
//
//	transport := transport.NewStdioTransport()
//	client := client.NewClient(transport)
//
//	// Initialize connection with capability negotiation
//	err := client.Initialize(ctx, &model.InitializeRequest{
//		ProtocolVersion: "2024-11-05",
//		Capabilities: model.ClientCapabilities{
//			Roots: &model.RootsCapability{
//				ListChanged: true,
//			},
//		},
//		ClientInfo: model.Implementation{
//			Name:    "my-client",
//			Version: "1.0.0",
//		},
//	})
//
//	// List available tools
//	tools, err := client.ListTools(ctx, &model.ListToolsRequest{})
//
//	// Call a tool
//	result, err := client.CallTool(ctx, &model.CallToolRequest{
//		Name: "get_weather",
//		Arguments: map[string]interface{}{
//			"location": "San Francisco",
//		},
//	})
//
// # Server Development
//
// Building an MCP server with the framework:
//
//	server := server.NewServer(&server.Config{
//		Name:    "weather-server",
//		Version: "1.0.0",
//	})
//
//	// Register tools
//	server.RegisterTool(&model.Tool{
//		Name:        "get_weather",
//		Description: "Get weather information for a location",
//		InputSchema: model.ToolInputSchema{
//			Type: "object",
//			Properties: map[string]interface{}{
//				"location": map[string]interface{}{
//					"type":        "string",
//					"description": "The location to get weather for",
//				},
//			},
//			Required: []string{"location"},
//		},
//	}, func(ctx context.Context, req *model.CallToolRequest) (*model.CallToolResult, error) {
//		location := req.Arguments["location"].(string)
//		// Implement weather fetching logic
//		return &model.CallToolResult{
//			Content: []model.Content{
//				{
//					Type: "text",
//					Text: "Sunny, 72°F in " + location,
//				},
//			},
//		}, nil
//	})
//
//	// Register resources
//	server.RegisterResource("weather://current/{location}", func(ctx context.Context, uri string) (*model.Resource, error) {
//		// Implement resource fetching logic
//		return &model.Resource{
//			URI:         uri,
//			Name:        "Current Weather",
//			Description: "Current weather conditions",
//			MimeType:    "application/json",
//		}, nil
//	})
//
//	// Start server
//	transport := transport.NewStdioTransport()
//	err := server.Serve(ctx, transport)
//
// # Resource Management
//
// The protocol supports both static and dynamic resources with URI templating:
//
//	// Resource templates for dynamic content
//	template := model.ResourceTemplate{
//		URITemplate: "file://{path}",
//		Name:        "File System",
//		Description: "Access to file system resources",
//		MimeType:    "text/plain",
//	}
//
//	// Resource with metadata
//	resource := model.Resource{
//		URI:         "file:///etc/hosts",
//		Name:        "System Hosts File",
//		Description: "System DNS configuration",
//		MimeType:    "text/plain",
//		Annotations: &model.Annotations{
//			Audience:    []model.Role{model.RoleUser},
//			Priority:    0.8,
//		},
//	}
//
// # Tool System
//
// Tools provide executable functionality with JSON Schema validation:
//
//	tool := model.Tool{
//		Name:        "execute_command",
//		Description: "Execute a shell command",
//		InputSchema: model.ToolInputSchema{
//			Type: "object",
//			Properties: map[string]interface{}{
//				"command": map[string]interface{}{
//					"type":        "string",
//					"description": "The command to execute",
//				},
//				"workdir": map[string]interface{}{
//					"type":        "string",
//					"description": "Working directory for command execution",
//				},
//			},
//			Required: []string{"command"},
//		},
//	}
//
// # Error Handling
//
// The library provides comprehensive error handling with protocol-specific error types:
//
//	if err != nil {
//		if mcpErr, ok := err.(*errors.MCPError); ok {
//			switch mcpErr.Code {
//			case errors.InvalidRequest:
//				// Handle invalid request
//			case errors.MethodNotFound:
//				// Handle unknown method
//			case errors.InvalidParams:
//				// Handle parameter validation errors
//			}
//		}
//	}
//
// # Capability Negotiation
//
// Servers and clients negotiate capabilities during initialization to ensure compatibility:
//
//	serverCaps := model.ServerCapabilities{
//		Resources: &model.ResourcesCapability{
//			Subscribe:    true,
//			ListChanged:  true,
//		},
//		Tools: &model.ToolsCapability{
//			ListChanged: true,
//		},
//		Prompts: &model.PromptsCapability{
//			ListChanged: true,
//		},
//		Logging: &model.LoggingCapability{},
//	}
//
// # Logging and Monitoring
//
// Built-in structured logging support for debugging and monitoring:
//
//	logger := logging.NewLogger(&logging.Config{
//		Level:  logging.InfoLevel,
//		Format: logging.JSONFormat,
//	})
//
//	server.SetLogger(logger)
//
// # Security Considerations
//
// The implementation includes several security features:
//
//   - Input validation on all external data using JSON Schema
//   - Resource access controls through URI validation
//   - Session management with configurable timeouts
//   - Memory and connection limits to prevent resource exhaustion
//   - Proper error handling to prevent information leakage
//
// # Performance Features
//
//   - Connection pooling for HTTP transports
//   - Configurable buffer sizes for large messages
//   - Non-blocking message handling where appropriate
//   - Efficient JSON serialization/deserialization
//   - Resource caching mechanisms
//
// # Examples
//
// The library includes comprehensive examples demonstrating real-world usage:
//
//   - Git MCP Server (example/git/) - Provides Git repository context
//   - Bash MCP Server (example/bash/) - Enables shell command execution
//   - Client Examples (example/client/) - Demonstrates client implementation patterns
//   - SSE Example (example/sse/) - Shows Server-Sent Events integration
//
// # Compatibility
//
// This implementation follows the MCP specification version 2024-11-05 and maintains
// compatibility with other MCP implementations. The JSON-RPC 2.0 transport ensures
// interoperability across different programming languages and platforms.
//
// # Thread Safety
//
// All public APIs are designed to be thread-safe. Concurrent access to clients and
// servers is properly synchronized using mutexes and channels. However, individual
// transport instances should not be shared between goroutines unless explicitly
// documented as safe.
//
// For more detailed information, see the documentation for individual packages and
// the examples provided in the example/ directory.
package mcp