package transport

import (
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
)

func TestConnectionManager(t *testing.T) {
	limits := &ConnectionLimits{
		MaxConnections:    2,
		MaxMessagesPerSec: 5,
		ConnectionTimeout: 100 * time.Millisecond,
		RequestTimeout:    1 * time.Second,
		MessageBufferSize: 10,
		KeepAliveInterval: 30 * time.Second,
	}

	cm := NewConnectionManager(limits, &logging.NoopLogger{})
	defer cm.Close()

	t.Run("RegisterConnection", func(t *testing.T) {
		// First connection should succeed
		conn1, err := cm.RegisterConnection("conn1", "session1", "127.0.0.1:8080")
		if err != nil {
			t.Fatalf("RegisterConnection() error = %v", err)
		}
		if conn1 == nil {
			t.Fatal("RegisterConnection() returned nil connection")
		}
		if conn1.ID != "conn1" {
			t.Errorf("RegisterConnection() ID = %s, want conn1", conn1.ID)
		}
		if conn1.SessionID != "session1" {
			t.Errorf("RegisterConnection() SessionID = %s, want session1", conn1.SessionID)
		}

		// Second connection should succeed
		conn2, err := cm.RegisterConnection("conn2", "session2", "127.0.0.1:8081")
		if err != nil {
			t.Fatalf("RegisterConnection() second connection error = %v", err)
		}
		if conn2 == nil {
			t.Fatal("RegisterConnection() returned nil for second connection")
		}

		// Third connection should fail due to limit
		conn3, err := cm.RegisterConnection("conn3", "session3", "127.0.0.1:8082")
		if err == nil {
			t.Fatal("RegisterConnection() expected error for third connection, got nil")
		}
		if conn3 != nil {
			t.Error("RegisterConnection() expected nil for third connection")
		}
		if err.Code != JSONRPCConnectionLimit {
			t.Errorf("RegisterConnection() error code = %d, want %d", err.Code, JSONRPCConnectionLimit)
		}

		// Check connection count
		count := cm.GetConnectionCount()
		if count != 2 {
			t.Errorf("GetConnectionCount() = %d, want 2", count)
		}
	})

	t.Run("UnregisterConnection", func(t *testing.T) {
		// Unregister first connection
		cm.UnregisterConnection("conn1")

		// Now third connection should succeed
		conn3, err := cm.RegisterConnection("conn3", "session3", "127.0.0.1:8082")
		if err != nil {
			t.Fatalf("RegisterConnection() after unregister error = %v", err)
		}
		if conn3 == nil {
			t.Fatal("RegisterConnection() after unregister returned nil")
		}

		count := cm.GetConnectionCount()
		if count != 2 {
			t.Errorf("GetConnectionCount() after unregister = %d, want 2", count)
		}
	})

	t.Run("UpdateActivity", func(t *testing.T) {
		// Get initial connection info
		conn := cm.GetConnectionInfo("conn2")
		if conn == nil {
			t.Fatal("GetConnectionInfo() returned nil")
		}
		initialActivity := conn.LastActivity
		initialCount := conn.MessageCount

		// Wait a moment and update activity
		time.Sleep(10 * time.Millisecond)
		cm.UpdateActivity("conn2")

		// Check updated info
		updatedConn := cm.GetConnectionInfo("conn2")
		if updatedConn == nil {
			t.Fatal("GetConnectionInfo() after update returned nil")
		}

		if !updatedConn.LastActivity.After(initialActivity) {
			t.Error("UpdateActivity() did not update LastActivity")
		}

		if updatedConn.MessageCount != initialCount+1 {
			t.Errorf("UpdateActivity() MessageCount = %d, want %d", updatedConn.MessageCount, initialCount+1)
		}
	})

	t.Run("GetConnectionsBySession", func(t *testing.T) {
		// Register another connection for session2
		_, err := cm.RegisterConnection("conn4", "session2", "127.0.0.1:8083")
		if err == nil {
			t.Error("Expected connection limit error, got nil")
		}

		// Get connections for session2 (should have conn2)
		connections := cm.GetConnectionsBySession("session2")
		if len(connections) != 1 {
			t.Errorf("GetConnectionsBySession() returned %d connections, want 1", len(connections))
		}
		if len(connections) > 0 && connections[0].ID != "conn2" {
			t.Errorf("GetConnectionsBySession() connection ID = %s, want conn2", connections[0].ID)
		}

		// Get connections for non-existent session
		emptyConnections := cm.GetConnectionsBySession("nonexistent")
		if len(emptyConnections) != 0 {
			t.Errorf("GetConnectionsBySession() for nonexistent session returned %d connections, want 0", len(emptyConnections))
		}
	})
}

func TestRateLimiter(t *testing.T) {
	// Create rate limiter allowing 2 tokens per second
	rl := NewRateLimiter(2)

	// First two requests should succeed
	if !rl.Allow() {
		t.Error("RateLimiter.Allow() first request should succeed")
	}
	if !rl.Allow() {
		t.Error("RateLimiter.Allow() second request should succeed")
	}

	// Third request should fail (no tokens left)
	if rl.Allow() {
		t.Error("RateLimiter.Allow() third request should fail")
	}

	// Wait for token refill (just over 1 second)
	time.Sleep(1100 * time.Millisecond)

	// Now requests should succeed again
	if !rl.Allow() {
		t.Error("RateLimiter.Allow() after refill should succeed")
	}
	if !rl.Allow() {
		t.Error("RateLimiter.Allow() second after refill should succeed")
	}
}

func TestConnectionManager_CheckRateLimit(t *testing.T) {
	limits := &ConnectionLimits{
		MaxConnections:    5,
		MaxMessagesPerSec: 2, // Very low for testing
		ConnectionTimeout: 1 * time.Minute,
		RequestTimeout:    30 * time.Second,
		MessageBufferSize: 10,
		KeepAliveInterval: 30 * time.Second,
	}

	cm := NewConnectionManager(limits, &logging.NoopLogger{})
	defer cm.Close()

	// Register a connection
	_, err := cm.RegisterConnection("conn1", "session1", "127.0.0.1:8080")
	if err != nil {
		t.Fatalf("RegisterConnection() error = %v", err)
	}

	// First two rate limit checks should pass
	if err := cm.CheckRateLimit("conn1"); err != nil {
		t.Errorf("CheckRateLimit() first check failed: %v", err)
	}
	if err := cm.CheckRateLimit("conn1"); err != nil {
		t.Errorf("CheckRateLimit() second check failed: %v", err)
	}

	// Third check should fail
	if err := cm.CheckRateLimit("conn1"); err == nil {
		t.Error("CheckRateLimit() third check should have failed")
	} else if err.Code != JSONRPCServerBusy {
		t.Errorf("CheckRateLimit() error code = %d, want %d", err.Code, JSONRPCServerBusy)
	}

	// Check for non-existent connection should pass (no rate limiting)
	if err := cm.CheckRateLimit("nonexistent"); err != nil {
		t.Errorf("CheckRateLimit() for nonexistent connection failed: %v", err)
	}
}

func TestConnectionManager_Cleanup(t *testing.T) {
	limits := &ConnectionLimits{
		MaxConnections:    5,
		MaxMessagesPerSec: 10,
		ConnectionTimeout: 50 * time.Millisecond, // Very short for testing
		RequestTimeout:    30 * time.Second,
		MessageBufferSize: 10,
		KeepAliveInterval: 30 * time.Second,
	}

	// Create connection manager with very short cleanup interval for testing
	cm := &ConnectionManager{
		limits:            limits,
		logger:            &logging.NoopLogger{},
		connections:       make(map[string]*ConnectionInfo),
		rateLimiters:      make(map[string]*RateLimiter),
		ipRateLimiters:    make(map[string]*RateLimiter),
		ipConnectionCount: make(map[string]int),
		cleanupInterval:   25 * time.Millisecond,
		stopCleanup:       make(chan struct{}),
	}
	
	// Start cleanup goroutine
	cm.startCleanup()
	
	// Ensure we clean up at the end
	defer cm.Close()

	// Register a connection
	_, err := cm.RegisterConnection("conn1", "session1", "127.0.0.1:8080")
	if err != nil {
		t.Fatalf("RegisterConnection() error = %v", err)
	}

	// Verify connection exists
	if cm.GetConnectionCount() != 1 {
		t.Errorf("GetConnectionCount() = %d, want 1", cm.GetConnectionCount())
	}

	// Wait for connection to expire and cleanup to run
	// Connection timeout is 50ms, cleanup runs every 25ms
	time.Sleep(100 * time.Millisecond) // Wait for connection to timeout and cleanup to run

	// Connection should be cleaned up
	if cm.GetConnectionCount() != 0 {
		t.Errorf("GetConnectionCount() after cleanup = %d, want 0", cm.GetConnectionCount())
	}

	// Test Close method
	if err := cm.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify all connections are closed
	if cm.GetConnectionCount() != 0 {
		t.Errorf("GetConnectionCount() after Close() = %d, want 0", cm.GetConnectionCount())
	}
}

func TestDefaultConnectionLimits(t *testing.T) {
	limits := DefaultConnectionLimits()
	
	if limits == nil {
		t.Fatal("DefaultConnectionLimits() returned nil")
	}

	if limits.MaxConnections != DefaultMaxConnections {
		t.Errorf("DefaultConnectionLimits().MaxConnections = %d, want %d", limits.MaxConnections, DefaultMaxConnections)
	}

	if limits.MaxMessagesPerSec != DefaultMaxMessagesPerSec {
		t.Errorf("DefaultConnectionLimits().MaxMessagesPerSec = %d, want %d", limits.MaxMessagesPerSec, DefaultMaxMessagesPerSec)
	}

	if limits.ConnectionTimeout != DefaultConnectionTimeout {
		t.Errorf("DefaultConnectionLimits().ConnectionTimeout = %v, want %v", limits.ConnectionTimeout, DefaultConnectionTimeout)
	}

	if limits.RequestTimeout != DefaultRequestTimeout {
		t.Errorf("DefaultConnectionLimits().RequestTimeout = %v, want %v", limits.RequestTimeout, DefaultRequestTimeout)
	}

	if limits.MessageBufferSize != DefaultMessageBufferSize {
		t.Errorf("DefaultConnectionLimits().MessageBufferSize = %d, want %d", limits.MessageBufferSize, DefaultMessageBufferSize)
	}

	if limits.KeepAliveInterval != DefaultKeepAliveInterval {
		t.Errorf("DefaultConnectionLimits().KeepAliveInterval = %v, want %v", limits.KeepAliveInterval, DefaultKeepAliveInterval)
	}
}