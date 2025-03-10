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
	ctx := &RequestContext{
		ID:       id,
		Method:   method,
		Response: make(chan *protocol.Message, 1),
		Error:    make(chan error, 1),
	}

	t.pendingRequests.Store(id, ctx)
	return ctx
}

// UntrackRequest removes a request from tracking.
func (t *RequestTracker) UntrackRequest(id protocol.RequestID) {
	t.pendingRequests.Delete(id)
}

// HandleResponse processes a response message and routes it to the appropriate request context.
func (t *RequestTracker) HandleResponse(msg *protocol.Message) bool {
	if msg.ID == nil {
		// This is a notification, not a response
		return false
	}

	reqCtxVal, ok := t.pendingRequests.Load(*msg.ID)
	if !ok {
		t.logger.Warn("Received response for unknown request", "id", *msg.ID)
		return false
	}

	reqCtx := reqCtxVal.(*RequestContext)

	if msg.Error != nil {
		reqCtx.Error <- &mcperrors.ProtocolError{
			Code:    msg.Error.Code,
			Message: msg.Error.Message,
			Data:    msg.Error.Data,
		}
	} else {
		reqCtx.Response <- msg
	}

	t.UntrackRequest(*msg.ID)
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
