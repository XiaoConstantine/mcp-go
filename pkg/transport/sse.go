package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// SSETransport implements Transport using Server-Sent Events.
type SSETransport struct {
	messageCh    chan *protocol.Message
	clients      map[string]chan *protocol.Message
	clientsMu    sync.RWMutex
	logger       logging.Logger
	responseData map[protocol.RequestID]chan *protocol.Message
	responseMu   sync.RWMutex
}

// NewSSETransport creates a new Transport that uses Server-Sent Events.
func NewSSETransport(logger logging.Logger) *SSETransport {
	if logger == nil {
		logger = &logging.NoopLogger{}
	}
	return &SSETransport{
		messageCh:    make(chan *protocol.Message, 100),
		clients:      make(map[string]chan *protocol.Message),
		logger:       logger,
		responseData: make(map[protocol.RequestID]chan *protocol.Message),
	}
}

// Send implements Transport.Send for SSETransport.
// This will broadcast the message to all connected clients.
func (t *SSETransport) Send(ctx context.Context, msg *protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Log the message being sent
	var idStr string
	if msg.ID != nil {
		idStr = fmt.Sprintf("%v", *msg.ID)
	} else {
		idStr = "<notification>"
	}
	t.logger.Debug("SENDING message ID=%s, Method=%s, Content: %s", idStr, msg.Method, string(data))

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		t.clientsMu.RLock()
		for _, clientCh := range t.clients {
			select {
			case clientCh <- msg:
				// Message sent successfully
			default:
				// Client channel is full, skip this client
				t.logger.Debug("Client channel full, skipping message")
			}
		}
		t.clientsMu.RUnlock()
	}

	return nil
}

// Receive implements Transport.Receive for SSETransport.
func (t *SSETransport) Receive(ctx context.Context) (*protocol.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-t.messageCh:
		if !ok {
			// Channel is closed
			return nil, io.EOF // or some other appropriate error
		}

		// Log the received message
		var idStr string
		if msg.ID != nil {
			idStr = fmt.Sprintf("%v", *msg.ID)
		} else {
			idStr = "<notification>"
		}
		t.logger.Debug("RECEIVED message ID=%s, Method=%s", idStr, msg.Method)

		// If this is a response to a request, save it to retrieve later
		if msg.Result != nil && msg.ID != nil {
			t.responseMu.Lock()
			if ch, ok := t.responseData[*msg.ID]; ok {
				ch <- msg
			}
			t.responseMu.Unlock()
		}

		return msg, nil
	}
}

// Close implements Transport.Close for SSETransport.
func (t *SSETransport) Close() error {
	t.clientsMu.Lock()
	for id, ch := range t.clients {
		close(ch)
		delete(t.clients, id)
	}
	t.clientsMu.Unlock()
	close(t.messageCh)
	return nil
}

// HandleSSE is the HTTP handler for SSE connections.
func (t *SSETransport) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a client ID
	clientID := r.RemoteAddr

	// Create a channel for this client
	msgCh := make(chan *protocol.Message, 10)

	// Register the client
	t.clientsMu.Lock()
	t.clients[clientID] = msgCh
	t.clientsMu.Unlock()

	// Make sure we clean up when the client disconnects
	defer func() {
		t.clientsMu.Lock()
		delete(t.clients, clientID)
		close(msgCh)
		t.clientsMu.Unlock()
	}()

	// Send messages to the client
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send a welcome message
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Send messages to the client
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case msg, ok := <-msgCh:
			if !ok {
				// Channel closed
				return
			}

			// Marshal the message
			data, err := json.Marshal(msg)
			if err != nil {
				t.logger.Error("Failed to marshal message: %v", err)
				continue
			}

			// Send the message
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// HandleClientMessage processes messages received from clients via HTTP POST.
func (t *SSETransport) HandleClientMessage(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode the message
	var msg protocol.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Send the message to the transport
	select {
	case t.messageCh <- &msg:
		// If this is a request that expects a response, wait for the response
		if msg.ID != nil {
			// Create a channel to receive the response
			respCh := make(chan *protocol.Message, 1)
			t.responseMu.Lock()
			t.responseData[*msg.ID] = respCh
			t.responseMu.Unlock()

			// Set a timeout for the response
			ctx, cancel := context.WithTimeout(r.Context(), 30*1000)
			defer cancel()

			// Wait for the response
			select {
			case resp := <-respCh:
				// Send the response
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					http.Error(w, "Failed to encode response", http.StatusInternalServerError)
					return
				}

				// Clean up
				t.responseMu.Lock()
				delete(t.responseData, *msg.ID)
				close(respCh)
				t.responseMu.Unlock()
				return
			case <-ctx.Done():
				// Timeout
				http.Error(w, "Request timeout", http.StatusGatewayTimeout)
				t.responseMu.Lock()
				delete(t.responseData, *msg.ID)
				close(respCh)
				t.responseMu.Unlock()
				return
			}
		}

		// For notifications, just acknowledge receipt
		w.WriteHeader(http.StatusAccepted)
	default:
		// Message channel is full
		http.Error(w, "Server busy", http.StatusServiceUnavailable)
	}
}
