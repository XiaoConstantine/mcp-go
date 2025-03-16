package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

func TestSSETransport(t *testing.T) {
	// Create a logger that logs to testing output
	logger := logging.NewStdLogger(logging.DebugLevel)

	// Create the transport
	transport := NewSSETransport(logger)

	t.Run("Send and Receive", func(t *testing.T) {
		// Create a test client
		clientID := "test-client"
		clientCh := make(chan *protocol.Message, 1)

		transport.clientsMu.Lock()
		transport.clients[clientID] = clientCh
		transport.clientsMu.Unlock()

		// Create a test message
		id := protocol.RequestID(1)
		msg := &protocol.Message{
			JSONRPC: "2.0",
			ID:      &id,
			Method:  "test/method",
			Params:  json.RawMessage(`{"test":"value"}`),
		}

		// Send the message
		ctx := context.Background()
		err := transport.Send(ctx, msg)
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		// Check that the client received the message
		select {
		case receivedMsg := <-clientCh:
			if receivedMsg.Method != msg.Method {
				t.Errorf("Method mismatch: expected %s, got %s", msg.Method, receivedMsg.Method)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for message to be received by client")
		}

		// Test receiving a message
		go func() {
			transport.messageCh <- msg
		}()

		receivedMsg, err := transport.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive failed: %v", err)
		}

		if receivedMsg.Method != msg.Method {
			t.Errorf("Method mismatch: expected %s, got %s", msg.Method, receivedMsg.Method)
		}
	})

	t.Run("HandleSSE", func(t *testing.T) {
		// Create a test server
		server := httptest.NewServer(http.HandlerFunc(transport.HandleSSE))
		defer server.Close()

		// Make a request to the SSE endpoint
		resp, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Check headers
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("Content-Type: expected text/event-stream, got %s", ct)
		}

		if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("Cache-Control: expected no-cache, got %s", cc)
		}
	})

	t.Run("HandleClientMessage", func(t *testing.T) {
		// Create a fresh transport for this test to avoid interference
		testTransport := NewSSETransport(logger)

		// Create a test server
		server := httptest.NewServer(http.HandlerFunc(testTransport.HandleClientMessage))
		defer server.Close()

		// Create a test message WITHOUT an ID (so it's treated as a notification)
		// This avoids waiting for a response that will never come
		msg := protocol.Message{
			JSONRPC: "2.0",
			Method:  "test/method",
			Params:  json.RawMessage(`{"test":"value"}`),
		}

		// Marshal to JSON
		msgJSON, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal message: %v", err)
		}

		// Send the message
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(string(msgJSON)))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Check the response status
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected status %d, got %d", http.StatusAccepted, resp.StatusCode)
		}

		// Check that the message was received
		select {
		case receivedMsg := <-testTransport.messageCh:
			if receivedMsg.Method != msg.Method {
				t.Errorf("Method mismatch: expected %s, got %s", msg.Method, receivedMsg.Method)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for message to be received")
		}
	})

	t.Run("HandleClientMessageWithResponse", func(t *testing.T) {
		// Create a fresh transport for this test
		testTransport := NewSSETransport(logger)

		// Create a test server
		server := httptest.NewServer(http.HandlerFunc(testTransport.HandleClientMessage))
		defer server.Close()

		// Create a test message WITH an ID
		id := protocol.RequestID(123)
		msg := protocol.Message{
			JSONRPC: "2.0",
			ID:      &id,
			Method:  "test/method",
			Params:  json.RawMessage(`{"test":"value"}`),
		}

		// Set up a goroutine to provide the response
		go func() {
			// Wait for the message to be received
			var receivedMsg *protocol.Message
			select {
			case receivedMsg = <-testTransport.messageCh:
				// Success
			case <-time.After(500 * time.Millisecond):
				t.Error("Timeout waiting for message to be received")
				return
			}

			// Create a response with the same ID
			response := &protocol.Message{
				JSONRPC: "2.0",
				ID:      receivedMsg.ID,
				Result:  json.RawMessage(`{"status":"success"}`),
			}

			// Register the response channel
			testTransport.responseMu.Lock()
			respCh, ok := testTransport.responseData[*receivedMsg.ID]
			testTransport.responseMu.Unlock()

			if ok {
				// Send the response
				respCh <- response
			} else {
				t.Error("Response channel not found")
			}
		}()

		// Marshal to JSON
		msgJSON, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal message: %v", err)
		}

		// Send the message
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(string(msgJSON)))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// Check the response status
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}

		// Define a custom type for decoding that preserves the json.RawMessage type
		type responseType struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      interface{}     `json:"id"`
			Result  json.RawMessage `json:"result,omitempty"`
			Error   *protocol.ErrorObject `json:"error,omitempty"`
		}
		
		// Decode the response body using our custom type
		var responseMsg responseType
		if err := json.NewDecoder(resp.Body).Decode(&responseMsg); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify response - convert both to strings for comparison since JSON unmarshalling
		// will convert numbers to float64 while the original might be an int
		respIDStr := fmt.Sprintf("%v", responseMsg.ID)
		expectedIDStr := fmt.Sprintf("%v", id)
		if respIDStr != expectedIDStr {
			t.Errorf("Response ID mismatch: expected %v (%T), got %v (%T)", 
				id, id, responseMsg.ID, responseMsg.ID)
		}

		if len(responseMsg.Result) == 0 {
			t.Error("Result is empty")
		} else if string(responseMsg.Result) != `{"status":"success"}` {
			t.Errorf("Response result mismatch: expected %s, got %s", `{"status":"success"}`, string(responseMsg.Result))
		}
	})

	t.Run("Close", func(t *testing.T) {
		// Create a fresh transport
		testTransport := NewSSETransport(logger)

		// Add a client
		clientID := "test-close-client"
		clientCh := make(chan *protocol.Message, 1)

		testTransport.clientsMu.Lock()
		testTransport.clients[clientID] = clientCh
		testTransport.clientsMu.Unlock()

		// Verify client was added
		testTransport.clientsMu.RLock()
		if _, ok := testTransport.clients[clientID]; !ok {
			t.Fatal("Client was not added")
		}
		testTransport.clientsMu.RUnlock()

		// Close the transport
		err := testTransport.Close()
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Verify all clients were removed
		testTransport.clientsMu.RLock()
		if len(testTransport.clients) != 0 {
			t.Errorf("Expected 0 clients after close, got %d", len(testTransport.clients))
		}
		testTransport.clientsMu.RUnlock()

		// Verify receiving on the message channel returns an error
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err = testTransport.Receive(ctx)
		if err == nil {
			t.Error("Expected error after close, got nil")
		}
	})
}
