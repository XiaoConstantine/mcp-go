package handler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	mcperrors "github.com/XiaoConstantine/mcp-go/pkg/errors"
	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// MessageHandler processes incoming messages.
type MessageHandler interface {
	// HandleMessage processes a message and returns a response if appropriate.
	HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error)
}

// MessageRouter routes messages to the appropriate handler.
type MessageRouter struct {
	handlers map[string]MessageHandler
	logger   logging.Logger
	mu       sync.RWMutex
}

// NewMessageRouter creates a new MessageRouter.
func NewMessageRouter(logger logging.Logger) *MessageRouter {
	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	return &MessageRouter{
		handlers: make(map[string]MessageHandler),
		logger:   logger,
	}
}

// RegisterHandler registers a handler for a specific method.
func (r *MessageRouter) RegisterHandler(method string, handler MessageHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[method] = handler
}

// HandleMessage routes a message to the appropriate handler.
func (r *MessageRouter) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	if msg.Method == "" {
		// This is a response, not a request
		return nil, nil
	}

	r.mu.RLock()
	handler, ok := r.handlers[msg.Method]
	r.mu.RUnlock()

	if !ok {
		return nil, errors.New("no handler registered for method: " + msg.Method)
	}

	return handler.HandleMessage(ctx, msg)
}

// RequestTracker keeps track of pending requests and their responses.
type RequestTracker struct {
	pendingRequests sync.Map
	idCounter       int64
	logger          logging.Logger
}

// RequestContext tracks an in-flight request.
type RequestContext struct {
	ID       protocol.RequestID
	Method   string
	Response chan *protocol.Message
	Error    chan error
}

// NewRequestTracker creates a new RequestTracker.
func NewRequestTracker(logger logging.Logger) *RequestTracker {
	if logger == nil {
		logger = logging.NewNoopLogger()
	}

	return &RequestTracker{
		logger: logger,
	}
}

// NextID generates a new unique request ID.
func (t *RequestTracker) NextID() protocol.RequestID {
	return atomic.AddInt64(&t.idCounter, 1)
}

// TrackRequest creates a new request context for tracking a request.
func (t *RequestTracker) TrackRequest(id protocol.RequestID, method string) *RequestContext {
	t.logger.Debug("Tracking new request with ID: %v, Method: %s", id, method)
	ctx := &RequestContext{
		ID:       id,
		Method:   method,
		Response: make(chan *protocol.Message, 1),
		Error:    make(chan error, 1),
	}

	t.pendingRequests.Store(id, ctx)

	// Debug-dump all currently tracked request IDs after adding
	var trackedIDs []protocol.RequestID
	t.pendingRequests.Range(func(key, value interface{}) bool {
		id := key.(protocol.RequestID)
		trackedIDs = append(trackedIDs, id)
		return true
	})
	t.logger.Debug("Currently tracked request IDs: %v", trackedIDs)

	return ctx
}

// UntrackRequest removes a request from tracking.
func (t *RequestTracker) UntrackRequest(id interface{}) {
	// Handle float64 to int64 conversion if needed
	var lookupID = id
	switch v := id.(type) {
	case float64:
		// Convert float64 to int64 for lookup if it's a whole number
		if v == float64(int64(v)) {
			lookupID = int64(v)
		}
	}
	t.pendingRequests.Delete(lookupID)
}

// IsTracked checks if a request ID is being tracked.
func (t *RequestTracker) IsTracked(id interface{}) bool {
	// Handle float64 to int64 conversion if needed
	var lookupID = id
	switch v := id.(type) {
	case float64:
		// Convert float64 to int64 for lookup if it's a whole number
		if v == float64(int64(v)) {
			lookupID = int64(v)
		}
	}
	_, ok := t.pendingRequests.Load(lookupID)
	return ok
}

// HandleResponse processes a response message and routes it to the appropriate request context.
func (t *RequestTracker) HandleResponse(msg *protocol.Message) bool {
	if msg.ID == nil {
		// This is a notification, not a response
		t.logger.Debug("Received notification message with method: %s", msg.Method)
		return false
	}

	// Log detailed information about the received response
	id := *msg.ID
	t.logger.Debug("Processing response message with ID: %v (type: %T), Method: %s", id, id, msg.Method)

	// Handle type conversion for ID lookup
	var lookupID interface{} = id
	switch v := id.(type) {
	case float64:
		// Convert float64 to int64 for lookup if it's a whole number
		if v == float64(int64(v)) {
			lookupID = int64(v)
			t.logger.Debug("Converting request ID from float64 to int64: %v -> %v", id, lookupID)
		}
	}

	reqCtxVal, ok := t.pendingRequests.Load(lookupID)
	// If we still couldn't find it after all retries
	if !ok {
		// Enhanced error logging with more context
		t.logger.Warn("Received response for unknown request id=%v(%T) after %d retries (full message: %+v)", lookupID, lookupID, 5, msg)
		return false
	}

	reqCtx := reqCtxVal.(*RequestContext)
	t.logger.Debug("Found matching request context for ID %v (method: %s)", lookupID, reqCtx.Method)

	if msg.Error != nil {
		t.logger.Debug("Response contains error: %v", msg.Error)
		reqCtx.Error <- &mcperrors.ProtocolError{
			Code:    msg.Error.Code,
			Message: msg.Error.Message,
			Data:    msg.Error.Data,
		}
	} else {
		t.logger.Debug("Sending successful response to request channel")
		reqCtx.Response <- msg
	}

	t.logger.Debug("Untracking request ID: %v", lookupID)
	t.UntrackRequest(lookupID)
	return true
}

// WaitForResponse waits for a response to a tracked request.
func (t *RequestTracker) WaitForResponse(ctx context.Context, reqCtx *RequestContext) (*protocol.Message, error) {
	select {
	case <-ctx.Done():
		t.UntrackRequest(reqCtx.ID)
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &mcperrors.TimeoutError{Duration: 0}
		}
		return nil, ctx.Err()
	case err := <-reqCtx.Error:
		return nil, err
	case resp := <-reqCtx.Response:
		return resp, nil
	}
}
