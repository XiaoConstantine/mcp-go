package transport

import (
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

func TestSessionManager(t *testing.T) {
	// Note: Each subtest creates its own session manager to avoid interference

	t.Run("CreateSession", func(t *testing.T) {
		config := &SessionManagerConfig{
			SessionTimeout:  100 * time.Millisecond,
			CleanupInterval: 50 * time.Millisecond,
			MaxSessions:     2,
			EnableResumable: true,
			Logger:          &logging.NoopLogger{},
		}
		sm := NewSessionManager(config)
		defer sm.Close()

		// Create first session
		session1, err := sm.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}
		if session1 == nil {
			t.Fatal("CreateSession() returned nil session")
		}
		if session1.ID == "" {
			t.Error("CreateSession() returned session with empty ID")
		}
		if session1.State != SessionStateActive {
			t.Errorf("CreateSession() session state = %v, want %v", session1.State, SessionStateActive)
		}

		// Create second session
		session2, err := sm.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession() second session error = %v", err)
		}
		if session2 == nil {
			t.Fatal("CreateSession() returned nil for second session")
		}

		// Third session should fail due to limit
		session3, err := sm.CreateSession()
		if err == nil {
			t.Error("CreateSession() expected error for third session, got nil")
		}
		if session3 != nil {
			t.Error("CreateSession() expected nil for third session")
		}

		// Check session count
		if count := sm.GetSessionCount(); count != 2 {
			t.Errorf("GetSessionCount() = %d, want 2", count)
		}
	})

	t.Run("GetSession", func(t *testing.T) {
		config := &SessionManagerConfig{
			SessionTimeout:  100 * time.Millisecond,
			CleanupInterval: 50 * time.Millisecond,
			MaxSessions:     5,
			EnableResumable: true,
			Logger:          &logging.NoopLogger{},
		}
		sm := NewSessionManager(config)
		defer sm.Close()

		// Create a session
		session, err := sm.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		// Retrieve the session
		retrieved := sm.GetSession(session.ID)
		if retrieved == nil {
			t.Fatal("GetSession() returned nil")
		}
		if retrieved.ID != session.ID {
			t.Errorf("GetSession() ID = %s, want %s", retrieved.ID, session.ID)
		}

		// Try to get non-existent session
		nonExistent := sm.GetSession("non-existent")
		if nonExistent != nil {
			t.Error("GetSession() for non-existent session should return nil")
		}
	})

	t.Run("ValidateSession", func(t *testing.T) {
		config := &SessionManagerConfig{
			SessionTimeout:  100 * time.Millisecond,
			CleanupInterval: 50 * time.Millisecond,
			MaxSessions:     5,
			EnableResumable: true,
			Logger:          &logging.NoopLogger{},
		}
		sm := NewSessionManager(config)
		defer sm.Close()

		// Create a session
		session, err := sm.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		// Validate existing session
		if err := sm.ValidateSession(session.ID); err != nil {
			t.Errorf("ValidateSession() error = %v", err)
		}

		// Validate non-existent session
		if err := sm.ValidateSession("non-existent"); err == nil {
			t.Error("ValidateSession() for non-existent session should return error")
		} else if err.Code != JSONRPCInvalidSession {
			t.Errorf("ValidateSession() error code = %d, want %d", err.Code, JSONRPCInvalidSession)
		}
	})

	t.Run("AddRemoveConnection", func(t *testing.T) {
		config := &SessionManagerConfig{
			SessionTimeout:  100 * time.Millisecond,
			CleanupInterval: 50 * time.Millisecond,
			MaxSessions:     5,
			EnableResumable: true,
			Logger:          &logging.NoopLogger{},
		}
		sm := NewSessionManager(config)
		defer sm.Close()

		// Create a session
		session, err := sm.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		// Add connection
		if err := sm.AddConnection(session.ID, "conn1"); err != nil {
			t.Errorf("AddConnection() error = %v", err)
		}

		// Get session and check connections
		updated := sm.GetSession(session.ID)
		if updated == nil {
			t.Fatal("GetSession() after AddConnection returned nil")
		}
		if len(updated.ConnectionIDs) != 1 {
			t.Errorf("GetSession() after AddConnection returned %d connections, want 1", len(updated.ConnectionIDs))
		}
		if updated.ConnectionIDs[0] != "conn1" {
			t.Errorf("GetSession() connection ID = %s, want conn1", updated.ConnectionIDs[0])
		}

		// Remove connection
		if err := sm.RemoveConnection(session.ID, "conn1"); err != nil {
			t.Errorf("RemoveConnection() error = %v", err)
		}

		// Check connections removed
		updated = sm.GetSession(session.ID)
		if updated == nil {
			t.Fatal("GetSession() after RemoveConnection returned nil")
		}
		if len(updated.ConnectionIDs) != 0 {
			t.Errorf("GetSession() after RemoveConnection returned %d connections, want 0", len(updated.ConnectionIDs))
		}
	})

	t.Run("PendingRequests", func(t *testing.T) {
		config := &SessionManagerConfig{
			SessionTimeout:  100 * time.Millisecond,
			CleanupInterval: 50 * time.Millisecond,
			MaxSessions:     5,
			EnableResumable: true,
			Logger:          &logging.NoopLogger{},
		}
		sm := NewSessionManager(config)
		defer sm.Close()

		// Create a session
		session, err := sm.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}

		// Create a test request
		requestID := protocol.RequestID("test-request")
		msg := &protocol.Message{
			JSONRPC: "2.0",
			Method:  "test.method",
			ID:      &requestID,
		}

		// Add pending request
		if err := sm.AddPendingRequest(session.ID, msg, 1*time.Second); err != nil {
			t.Errorf("AddPendingRequest() error = %v", err)
		}

		// Get pending requests
		pending := sm.GetPendingRequests(session.ID)
		if len(pending) != 1 {
			t.Errorf("GetPendingRequests() returned %d requests, want 1", len(pending))
		}
		if len(pending) > 0 && pending[0].Request.Method != "test.method" {
			t.Errorf("GetPendingRequests() method = %s, want test.method", pending[0].Request.Method)
		}

		// Remove pending request
		if err := sm.RemovePendingRequest(session.ID, requestID); err != nil {
			t.Errorf("RemovePendingRequest() error = %v", err)
		}

		// Check request removed
		pending = sm.GetPendingRequests(session.ID)
		if len(pending) != 0 {
			t.Errorf("GetPendingRequests() after removal returned %d requests, want 0", len(pending))
		}
	})
}

func TestSessionManager_Cleanup(t *testing.T) {
	config := &SessionManagerConfig{
		SessionTimeout:  50 * time.Millisecond, // Very short for testing
		CleanupInterval: 25 * time.Millisecond, // Very frequent cleanup
		MaxSessions:     5,
		EnableResumable: true,
		Logger:          &logging.NoopLogger{},
	}

	sm := NewSessionManager(config)
	// Don't defer close, we'll test it explicitly

	// Create a session
	session, err := sm.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Verify session exists
	if sm.GetSessionCount() != 1 {
		t.Errorf("GetSessionCount() = %d, want 1", sm.GetSessionCount())
	}

	// Wait for session to expire and cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Session should be cleaned up
	if sm.GetSessionCount() != 0 {
		t.Errorf("GetSessionCount() after cleanup = %d, want 0", sm.GetSessionCount())
	}

	// Validation should fail for expired session
	if err := sm.ValidateSession(session.ID); err == nil {
		t.Error("ValidateSession() for expired session should fail")
	}

	// Test Close method
	if err := sm.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestGenerateSessionID(t *testing.T) {
	sm := NewSessionManager(nil)
	defer sm.Close()

	// Generate multiple session IDs
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id, err := sm.GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() error = %v", err)
		}
		if id == "" {
			t.Error("GenerateSessionID() returned empty string")
		}
		if len(id) != 32 { // 16 bytes = 32 hex chars
			t.Errorf("GenerateSessionID() length = %d, want 32", len(id))
		}
		if ids[id] {
			t.Errorf("GenerateSessionID() generated duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestSessionState_String(t *testing.T) {
	tests := []struct {
		state SessionState
		want  string
	}{
		{SessionStateActive, "active"},
		{SessionStateSuspended, "suspended"},
		{SessionStateExpired, "expired"},
		{SessionStateClosed, "closed"},
		{SessionState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("SessionState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultSessionManagerConfig(t *testing.T) {
	config := DefaultSessionManagerConfig()

	if config == nil {
		t.Fatal("DefaultSessionManagerConfig() returned nil")
	}

	if config.SessionTimeout != 30*time.Minute {
		t.Errorf("DefaultSessionManagerConfig().SessionTimeout = %v, want %v", config.SessionTimeout, 30*time.Minute)
	}

	if config.CleanupInterval != 5*time.Minute {
		t.Errorf("DefaultSessionManagerConfig().CleanupInterval = %v, want %v", config.CleanupInterval, 5*time.Minute)
	}

	if config.MaxSessions != 1000 {
		t.Errorf("DefaultSessionManagerConfig().MaxSessions = %d, want %d", config.MaxSessions, 1000)
	}

	if !config.EnableResumable {
		t.Error("DefaultSessionManagerConfig().EnableResumable = false, want true")
	}

	if config.Logger == nil {
		t.Error("DefaultSessionManagerConfig().Logger is nil")
	}
}