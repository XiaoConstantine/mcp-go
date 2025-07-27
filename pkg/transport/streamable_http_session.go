package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// SessionState represents the state of a session.
type SessionState int

const (
	SessionStateActive SessionState = iota
	SessionStateSuspended
	SessionStateExpired
	SessionStateClosed
)

// String returns the string representation of SessionState.
func (s SessionState) String() string {
	switch s {
	case SessionStateActive:
		return "active"
	case SessionStateSuspended:
		return "suspended"
	case SessionStateExpired:
		return "expired"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// SessionInfo holds information about a session.
type SessionInfo struct {
	ID            string
	CreatedAt     time.Time
	LastActivity  time.Time
	ExpiresAt     time.Time
	State         SessionState
	ConnectionIDs []string
	Metadata      map[string]interface{}

	// Resumability support
	PendingRequests map[protocol.RequestID]*PendingRequest
	MessageSequence int64
	LastSequenceAck int64

	mu sync.RWMutex
}

// PendingRequest represents a request that's waiting for response.
type PendingRequest struct {
	Request   *protocol.Message
	Timestamp time.Time
	Timeout   time.Time
	Context   context.Context
	Cancel    context.CancelFunc
}

// ResumableSession provides session resumability features.
type ResumableSession struct {
	// Currently unused - will be implemented in future versions
}

// SessionManager manages sessions and provides resumability.
type SessionManager struct {
	sessions   map[string]*SessionInfo
	sessionsMu sync.RWMutex

	// Configuration
	sessionTimeout  time.Duration
	cleanupInterval time.Duration
	maxSessions     int
	enableResumable bool

	// Dependencies
	logger logging.Logger

	// Cleanup
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	cleanupOnce   sync.Once
}

// SessionManagerConfig holds configuration for session manager.
type SessionManagerConfig struct {
	SessionTimeout  time.Duration
	CleanupInterval time.Duration
	MaxSessions     int
	EnableResumable bool
	Logger          logging.Logger
}

// DefaultSessionManagerConfig returns default session manager configuration.
func DefaultSessionManagerConfig() *SessionManagerConfig {
	return &SessionManagerConfig{
		SessionTimeout:  30 * time.Minute,
		CleanupInterval: 5 * time.Minute,
		MaxSessions:     1000,
		EnableResumable: true,
		Logger:          &logging.NoopLogger{},
	}
}

// NewSessionManager creates a new session manager.
func NewSessionManager(config *SessionManagerConfig) *SessionManager {
	if config == nil {
		config = DefaultSessionManagerConfig()
	}

	if config.Logger == nil {
		config.Logger = &logging.NoopLogger{}
	}

	sm := &SessionManager{
		sessions:        make(map[string]*SessionInfo),
		sessionTimeout:  config.SessionTimeout,
		cleanupInterval: config.CleanupInterval,
		maxSessions:     config.MaxSessions,
		enableResumable: config.EnableResumable,
		logger:          config.Logger,
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	sm.startCleanup()

	return sm
}

// GenerateSessionID generates a new secure session ID.
func (sm *SessionManager) GenerateSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random session ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// CreateSession creates a new session.
func (sm *SessionManager) CreateSession() (*SessionInfo, error) {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()

	// Check session limit
	if len(sm.sessions) >= sm.maxSessions {
		return nil, NewServerError("Session limit exceeded")
	}

	// Generate session ID
	sessionID, err := sm.GenerateSessionID()
	if err != nil {
		return nil, NewInternalError(err.Error())
	}

	// Create session info
	now := time.Now()
	sessionInfo := &SessionInfo{
		ID:              sessionID,
		CreatedAt:       now,
		LastActivity:    now,
		ExpiresAt:       now.Add(sm.sessionTimeout),
		State:           SessionStateActive,
		ConnectionIDs:   make([]string, 0),
		Metadata:        make(map[string]interface{}),
		PendingRequests: make(map[protocol.RequestID]*PendingRequest),
		MessageSequence: 0,
		LastSequenceAck: 0,
	}

	sm.sessions[sessionID] = sessionInfo

	sm.logger.Debug("Created session %s. Total sessions: %d", sessionID, len(sm.sessions))

	return sessionInfo, nil
}

// GetSession retrieves a session by ID.
func (sm *SessionManager) GetSession(sessionID string) *SessionInfo {
	sm.sessionsMu.RLock()
	defer sm.sessionsMu.RUnlock()

	if session, exists := sm.sessions[sessionID]; exists {
		// Return a copy with updated activity
		session.mu.Lock()
		session.LastActivity = time.Now()
		session.ExpiresAt = time.Now().Add(sm.sessionTimeout)
		sessionCopy := sm.copySessionInfo(session)
		session.mu.Unlock()
		return sessionCopy
	}

	return nil
}

// ValidateSession checks if a session is valid and active.
func (sm *SessionManager) ValidateSession(sessionID string) *MCPError {
	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return NewInvalidSessionError(sessionID)
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	if session.State != SessionStateActive {
		return NewInvalidSessionError(sessionID)
	}

	if time.Now().After(session.ExpiresAt) {
		return NewInvalidSessionError(sessionID)
	}

	return nil
}

// AddConnection adds a connection to a session.
func (sm *SessionManager) AddConnection(sessionID, connectionID string) error {
	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Check if connection already exists
	for _, connID := range session.ConnectionIDs {
		if connID == connectionID {
			return nil // Already added
		}
	}

	session.ConnectionIDs = append(session.ConnectionIDs, connectionID)
	session.LastActivity = time.Now()
	session.ExpiresAt = time.Now().Add(sm.sessionTimeout)

	sm.logger.Debug("Added connection %s to session %s", connectionID, sessionID)

	return nil
}

// RemoveConnection removes a connection from a session.
func (sm *SessionManager) RemoveConnection(sessionID, connectionID string) error {
	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return nil // Session doesn't exist, nothing to remove
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Find and remove the connection
	for i, connID := range session.ConnectionIDs {
		if connID == connectionID {
			session.ConnectionIDs = append(session.ConnectionIDs[:i], session.ConnectionIDs[i+1:]...)
			sm.logger.Debug("Removed connection %s from session %s", connectionID, sessionID)
			break
		}
	}

	return nil
}

// AddPendingRequest adds a pending request to a session.
func (sm *SessionManager) AddPendingRequest(sessionID string, request *protocol.Message, timeout time.Duration) error {
	if !sm.enableResumable || request.ID == nil {
		return nil // Not using resumable sessions or notification
	}

	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	pendingReq := &PendingRequest{
		Request:   request,
		Timestamp: time.Now(),
		Timeout:   time.Now().Add(timeout),
		Context:   ctx,
		Cancel:    cancel,
	}

	session.mu.Lock()
	session.PendingRequests[*request.ID] = pendingReq
	session.MessageSequence++
	session.mu.Unlock()

	sm.logger.Debug("Added pending request %v to session %s", *request.ID, sessionID)

	return nil
}

// RemovePendingRequest removes a pending request from a session.
func (sm *SessionManager) RemovePendingRequest(sessionID string, requestID protocol.RequestID) error {
	if !sm.enableResumable {
		return nil
	}

	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return nil // Session doesn't exist
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if pendingReq, exists := session.PendingRequests[requestID]; exists {
		if pendingReq.Cancel != nil {
			pendingReq.Cancel()
		}
		delete(session.PendingRequests, requestID)
		sm.logger.Debug("Removed pending request %v from session %s", requestID, sessionID)
	}

	return nil
}

// GetPendingRequests retrieves all pending requests for a session.
func (sm *SessionManager) GetPendingRequests(sessionID string) []*PendingRequest {
	if !sm.enableResumable {
		return nil
	}

	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return nil
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	var pending []*PendingRequest
	for _, req := range session.PendingRequests {
		// Return copy to avoid race conditions
		pending = append(pending, &PendingRequest{
			Request:   req.Request,
			Timestamp: req.Timestamp,
			Timeout:   req.Timeout,
			Context:   req.Context,
		})
	}

	return pending
}

// ExpireSession marks a session as expired.
func (sm *SessionManager) ExpireSession(sessionID string) error {
	sm.sessionsMu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.sessionsMu.RUnlock()

	if !exists {
		return nil // Session doesn't exist
	}

	session.mu.Lock()
	session.State = SessionStateExpired

	// Cancel all pending requests
	for requestID, pendingReq := range session.PendingRequests {
		if pendingReq.Cancel != nil {
			pendingReq.Cancel()
		}
		delete(session.PendingRequests, requestID)
	}
	session.mu.Unlock()

	sm.logger.Debug("Expired session %s", sessionID)

	return nil
}

// CloseSession closes a session.
func (sm *SessionManager) CloseSession(sessionID string) error {
	sm.sessionsMu.Lock()
	defer sm.sessionsMu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		session.mu.Lock()
		session.State = SessionStateClosed

		// Cancel all pending requests
		for requestID, pendingReq := range session.PendingRequests {
			if pendingReq.Cancel != nil {
				pendingReq.Cancel()
			}
			delete(session.PendingRequests, requestID)
		}
		session.mu.Unlock()

		delete(sm.sessions, sessionID)
		sm.logger.Debug("Closed session %s. Total sessions: %d", sessionID, len(sm.sessions))
	}

	return nil
}

// GetSessionCount returns the current number of sessions.
func (sm *SessionManager) GetSessionCount() int {
	sm.sessionsMu.RLock()
	defer sm.sessionsMu.RUnlock()
	return len(sm.sessions)
}

// Close closes the session manager and cleans up resources.
func (sm *SessionManager) Close() error {
	// Stop cleanup goroutine
	sm.cleanupOnce.Do(func() {
		close(sm.stopCleanup)
		if sm.cleanupTicker != nil {
			sm.cleanupTicker.Stop()
		}
	})

	// Close all sessions
	sm.sessionsMu.Lock()
	for sessionID := range sm.sessions {
		if session := sm.sessions[sessionID]; session != nil {
			session.mu.Lock()
			for _, pendingReq := range session.PendingRequests {
				if pendingReq.Cancel != nil {
					pendingReq.Cancel()
				}
			}
			session.mu.Unlock()
		}
		delete(sm.sessions, sessionID)
	}
	sm.sessionsMu.Unlock()

	return nil
}

// startCleanup starts the background cleanup goroutine.
func (sm *SessionManager) startCleanup() {
	if sm.cleanupInterval <= 0 {
		return
	}

	ticker := time.NewTicker(sm.cleanupInterval)
	sm.cleanupTicker = ticker

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sm.cleanupExpiredSessions()
			case <-sm.stopCleanup:
				return
			}
		}
	}()
}

// cleanupExpiredSessions removes expired sessions and requests.
func (sm *SessionManager) cleanupExpiredSessions() {
	now := time.Now()
	var toRemove []string
	var expiredRequests int

	sm.sessionsMu.RLock()
	for sessionID, session := range sm.sessions {
		session.mu.RLock()

		// Check if session is expired
		if session.State == SessionStateActive && now.After(session.ExpiresAt) {
			toRemove = append(toRemove, sessionID)
		}

		// Check for expired pending requests
		for requestID, pendingReq := range session.PendingRequests {
			if now.After(pendingReq.Timeout) {
				session.mu.RUnlock()
				session.mu.Lock()
				if pendingReq.Cancel != nil {
					pendingReq.Cancel()
				}
				delete(session.PendingRequests, requestID)
				expiredRequests++
				session.mu.Unlock()
				session.mu.RLock()
			}
		}

		session.mu.RUnlock()
	}
	sm.sessionsMu.RUnlock()

	// Remove expired sessions
	for _, sessionID := range toRemove {
		if err := sm.ExpireSession(sessionID); err != nil {
			sm.logger.Warn("Failed to expire session %s: %v", sessionID, err)
		}
		if err := sm.CloseSession(sessionID); err != nil {
			sm.logger.Warn("Failed to close session %s: %v", sessionID, err)
		}
	}

	if len(toRemove) > 0 || expiredRequests > 0 {
		sm.logger.Debug("Cleaned up %d expired sessions and %d expired requests", len(toRemove), expiredRequests)
	}
}

// copySessionInfo creates a copy of session info for thread safety.
func (sm *SessionManager) copySessionInfo(session *SessionInfo) *SessionInfo {
	connIDs := make([]string, len(session.ConnectionIDs))
	copy(connIDs, session.ConnectionIDs)

	metadata := make(map[string]interface{})
	for k, v := range session.Metadata {
		metadata[k] = v
	}

	return &SessionInfo{
		ID:              session.ID,
		CreatedAt:       session.CreatedAt,
		LastActivity:    session.LastActivity,
		ExpiresAt:       session.ExpiresAt,
		State:           session.State,
		ConnectionIDs:   connIDs,
		Metadata:        metadata,
		MessageSequence: session.MessageSequence,
		LastSequenceAck: session.LastSequenceAck,
	}
}
