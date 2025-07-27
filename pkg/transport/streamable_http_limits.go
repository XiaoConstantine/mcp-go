package transport

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
)

// Default limits for resource management.
const (
	DefaultMaxConnections         = 100
	DefaultMaxMessagesPerSec      = 10
	DefaultMaxConnectionsPerIP    = 10 // Max 10 connections per IP
	DefaultMaxMessagesPerSecPerIP = 50 // Max 50 messages per second per IP
	DefaultConnectionTimeout      = 5 * time.Minute
	DefaultRequestTimeout         = 30 * time.Second
	DefaultMessageBufferSize      = 10
	DefaultKeepAliveInterval      = 30 * time.Second
)

// ConnectionLimits defines resource limits for connections.
type ConnectionLimits struct {
	// Maximum number of concurrent SSE connections
	MaxConnections int

	// Maximum messages per second per connection
	MaxMessagesPerSec int

	// Maximum connections per IP address
	MaxConnectionsPerIP int

	// Maximum messages per second per IP address
	MaxMessagesPerSecPerIP int

	// Timeout for idle connections
	ConnectionTimeout time.Duration

	// Timeout for individual requests
	RequestTimeout time.Duration

	// Buffer size for message channels
	MessageBufferSize int

	// Keep-alive interval for SSE connections
	KeepAliveInterval time.Duration
}

// DefaultConnectionLimits returns default connection limits.
func DefaultConnectionLimits() *ConnectionLimits {
	return &ConnectionLimits{
		MaxConnections:         DefaultMaxConnections,
		MaxMessagesPerSec:      DefaultMaxMessagesPerSec,
		MaxConnectionsPerIP:    DefaultMaxConnectionsPerIP,
		MaxMessagesPerSecPerIP: DefaultMaxMessagesPerSecPerIP,
		ConnectionTimeout:      DefaultConnectionTimeout,
		RequestTimeout:         DefaultRequestTimeout,
		MessageBufferSize:      DefaultMessageBufferSize,
		KeepAliveInterval:      DefaultKeepAliveInterval,
	}
}

// ConnectionManager manages connection limits and resource allocation.
type ConnectionManager struct {
	limits *ConnectionLimits
	logger logging.Logger

	// Connection tracking
	connections     map[string]*ConnectionInfo
	connectionsMu   sync.RWMutex
	connectionCount int

	// Rate limiting (connection-based)
	rateLimiters   map[string]*RateLimiter
	rateLimitersMu sync.RWMutex

	// IP-based rate limiting
	ipRateLimiters      map[string]*RateLimiter
	ipRateLimitersMu    sync.RWMutex
	ipConnectionCount   map[string]int // Track connections per IP
	ipConnectionCountMu sync.RWMutex

	// Cleanup
	cleanupInterval time.Duration
	cleanupTicker   *time.Ticker
	stopCleanup     chan struct{}
	cleanupOnce     sync.Once
}

// ConnectionInfo holds information about a connection.
type ConnectionInfo struct {
	ID           string
	SessionID    string
	RemoteAddr   string
	CreatedAt    time.Time
	LastActivity time.Time
	MessageCount int64
	IsActive     bool
	Context      context.Context
	Cancel       context.CancelFunc
}

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	tokens     int
	maxTokens  int
	refillRate int
	lastRefill time.Time
	mu         sync.Mutex
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(limits *ConnectionLimits, logger logging.Logger) *ConnectionManager {
	if limits == nil {
		limits = DefaultConnectionLimits()
	}
	if logger == nil {
		logger = &logging.NoopLogger{}
	}

	cm := &ConnectionManager{
		limits:            limits,
		logger:            logger,
		connections:       make(map[string]*ConnectionInfo),
		rateLimiters:      make(map[string]*RateLimiter),
		ipRateLimiters:    make(map[string]*RateLimiter),
		ipConnectionCount: make(map[string]int),
		cleanupInterval:   time.Minute,
		stopCleanup:       make(chan struct{}),
	}

	// Start cleanup goroutine
	cm.startCleanup()

	return cm
}

// extractIPAddress extracts the IP address from a remote address string.
func (cm *ConnectionManager) extractIPAddress(remoteAddr string) string {
	// Handle X-Forwarded-For and X-Real-IP headers would be done at HTTP level
	// Here we just extract from remoteAddr
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If no port, assume it's just an IP
		return remoteAddr
	}
	return host
}

// checkIPLimits checks if an IP address can accept a new connection.
func (cm *ConnectionManager) checkIPLimits(remoteAddr string) *MCPError {
	if cm.limits.MaxConnectionsPerIP <= 0 {
		return nil // No IP-based limits
	}

	ip := cm.extractIPAddress(remoteAddr)

	cm.ipConnectionCountMu.RLock()
	currentCount := cm.ipConnectionCount[ip]
	cm.ipConnectionCountMu.RUnlock()

	if currentCount >= cm.limits.MaxConnectionsPerIP {
		cm.logger.Warn("IP connection limit exceeded for %s: %d/%d", ip, currentCount, cm.limits.MaxConnectionsPerIP)
		return NewConnectionLimitError(cm.limits.MaxConnectionsPerIP)
	}

	return nil
}

// CanAcceptConnection checks if a new connection can be accepted (global limit only).
func (cm *ConnectionManager) CanAcceptConnection() *MCPError {
	cm.connectionsMu.RLock()
	count := cm.connectionCount
	cm.connectionsMu.RUnlock()

	if count >= cm.limits.MaxConnections {
		cm.logger.Warn("Connection limit exceeded: %d/%d", count, cm.limits.MaxConnections)
		return NewConnectionLimitError(cm.limits.MaxConnections)
	}

	return nil
}

// CanAcceptConnectionFromIP checks if a new connection can be accepted from a specific IP.
func (cm *ConnectionManager) CanAcceptConnectionFromIP(remoteAddr string) *MCPError {
	// Check global limit first
	if err := cm.CanAcceptConnection(); err != nil {
		return err
	}

	// Check IP-specific limit
	return cm.checkIPLimits(remoteAddr)
}

// RegisterConnection registers a new connection.
func (cm *ConnectionManager) RegisterConnection(connectionID, sessionID, remoteAddr string) (*ConnectionInfo, *MCPError) {
	// Check global and IP-based limits
	if err := cm.CanAcceptConnectionFromIP(remoteAddr); err != nil {
		return nil, err
	}

	ip := cm.extractIPAddress(remoteAddr)

	cm.connectionsMu.Lock()
	defer cm.connectionsMu.Unlock()

	// Check again after acquiring lock
	if cm.connectionCount >= cm.limits.MaxConnections {
		return nil, NewConnectionLimitError(cm.limits.MaxConnections)
	}

	// Check IP limit again after lock
	cm.ipConnectionCountMu.Lock()
	ipCount := cm.ipConnectionCount[ip]
	if cm.limits.MaxConnectionsPerIP > 0 && ipCount >= cm.limits.MaxConnectionsPerIP {
		cm.ipConnectionCountMu.Unlock()
		return nil, NewConnectionLimitError(cm.limits.MaxConnectionsPerIP)
	}
	cm.ipConnectionCount[ip] = ipCount + 1
	cm.ipConnectionCountMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cm.limits.ConnectionTimeout)

	connInfo := &ConnectionInfo{
		ID:           connectionID,
		SessionID:    sessionID,
		RemoteAddr:   remoteAddr,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		MessageCount: 0,
		IsActive:     true,
		Context:      ctx,
		Cancel:       cancel,
	}

	cm.connections[connectionID] = connInfo
	cm.connectionCount++

	// Initialize rate limiter for this connection
	if cm.limits.MaxMessagesPerSec > 0 {
		cm.rateLimitersMu.Lock()
		cm.rateLimiters[connectionID] = NewRateLimiter(cm.limits.MaxMessagesPerSec)
		cm.rateLimitersMu.Unlock()
	}

	// Initialize IP-based rate limiter
	if cm.limits.MaxMessagesPerSecPerIP > 0 {
		cm.ipRateLimitersMu.Lock()
		if _, exists := cm.ipRateLimiters[ip]; !exists {
			cm.ipRateLimiters[ip] = NewRateLimiter(cm.limits.MaxMessagesPerSecPerIP)
		}
		cm.ipRateLimitersMu.Unlock()
	}

	cm.logger.Debug("Registered connection %s (session: %s, remote: %s, IP: %s). Total: %d/%d, IP connections: %d/%d",
		connectionID, sessionID, remoteAddr, ip, cm.connectionCount, cm.limits.MaxConnections,
		cm.ipConnectionCount[ip], cm.limits.MaxConnectionsPerIP)

	return connInfo, nil
}

// UnregisterConnection removes a connection.
func (cm *ConnectionManager) UnregisterConnection(connectionID string) {
	cm.connectionsMu.Lock()
	defer cm.connectionsMu.Unlock()

	if connInfo, exists := cm.connections[connectionID]; exists {
		ip := cm.extractIPAddress(connInfo.RemoteAddr)

		if connInfo.Cancel != nil {
			connInfo.Cancel()
		}
		delete(cm.connections, connectionID)
		cm.connectionCount--

		// Decrement IP connection count
		cm.ipConnectionCountMu.Lock()
		if count := cm.ipConnectionCount[ip]; count > 0 {
			cm.ipConnectionCount[ip] = count - 1
			if cm.ipConnectionCount[ip] == 0 {
				delete(cm.ipConnectionCount, ip)

				// Clean up IP rate limiter if no more connections from this IP
				cm.ipRateLimitersMu.Lock()
				delete(cm.ipRateLimiters, ip)
				cm.ipRateLimitersMu.Unlock()
			}
		}
		cm.ipConnectionCountMu.Unlock()

		cm.logger.Debug("Unregistered connection %s (IP: %s). Total: %d/%d",
			connectionID, ip, cm.connectionCount, cm.limits.MaxConnections)
	}

	// Clean up connection rate limiter
	cm.rateLimitersMu.Lock()
	delete(cm.rateLimiters, connectionID)
	cm.rateLimitersMu.Unlock()
}

// UpdateActivity updates the last activity time for a connection.
func (cm *ConnectionManager) UpdateActivity(connectionID string) {
	cm.connectionsMu.Lock()
	defer cm.connectionsMu.Unlock()

	if connInfo, exists := cm.connections[connectionID]; exists {
		connInfo.LastActivity = time.Now()
		connInfo.MessageCount++
	}
}

// CheckRateLimit checks if a connection is within rate limits (both connection and IP-based).
func (cm *ConnectionManager) CheckRateLimit(connectionID string) *MCPError {
	// Check connection-based rate limit
	if cm.limits.MaxMessagesPerSec > 0 {
		cm.rateLimitersMu.RLock()
		rateLimiter, exists := cm.rateLimiters[connectionID]
		cm.rateLimitersMu.RUnlock()

		if exists && !rateLimiter.Allow() {
			cm.logger.Warn("Connection rate limit exceeded for connection %s", connectionID)
			return NewServerBusyError()
		}
	}

	// Check IP-based rate limit
	if cm.limits.MaxMessagesPerSecPerIP > 0 {
		cm.connectionsMu.RLock()
		connInfo, exists := cm.connections[connectionID]
		cm.connectionsMu.RUnlock()

		if exists {
			ip := cm.extractIPAddress(connInfo.RemoteAddr)

			cm.ipRateLimitersMu.RLock()
			ipRateLimiter, ipExists := cm.ipRateLimiters[ip]
			cm.ipRateLimitersMu.RUnlock()

			if ipExists && !ipRateLimiter.Allow() {
				cm.logger.Warn("IP rate limit exceeded for IP %s (connection %s)", ip, connectionID)
				return NewServerBusyError()
			}
		}
	}

	return nil
}

// GetConnectionInfo gets information about a connection.
func (cm *ConnectionManager) GetConnectionInfo(connectionID string) *ConnectionInfo {
	cm.connectionsMu.RLock()
	defer cm.connectionsMu.RUnlock()

	if connInfo, exists := cm.connections[connectionID]; exists {
		// Return a copy to avoid race conditions
		return &ConnectionInfo{
			ID:           connInfo.ID,
			SessionID:    connInfo.SessionID,
			RemoteAddr:   connInfo.RemoteAddr,
			CreatedAt:    connInfo.CreatedAt,
			LastActivity: connInfo.LastActivity,
			MessageCount: connInfo.MessageCount,
			IsActive:     connInfo.IsActive,
			Context:      connInfo.Context,
		}
	}

	return nil
}

// GetConnectionCount gets the current number of connections.
func (cm *ConnectionManager) GetConnectionCount() int {
	cm.connectionsMu.RLock()
	defer cm.connectionsMu.RUnlock()
	return cm.connectionCount
}

// GetConnectionsBySession returns all connections for a session.
func (cm *ConnectionManager) GetConnectionsBySession(sessionID string) []*ConnectionInfo {
	cm.connectionsMu.RLock()
	defer cm.connectionsMu.RUnlock()

	var connections []*ConnectionInfo
	for _, connInfo := range cm.connections {
		if connInfo.SessionID == sessionID {
			// Return a copy to avoid race conditions
			connections = append(connections, &ConnectionInfo{
				ID:           connInfo.ID,
				SessionID:    connInfo.SessionID,
				RemoteAddr:   connInfo.RemoteAddr,
				CreatedAt:    connInfo.CreatedAt,
				LastActivity: connInfo.LastActivity,
				MessageCount: connInfo.MessageCount,
				IsActive:     connInfo.IsActive,
				Context:      connInfo.Context,
			})
		}
	}

	return connections
}

// Close closes the connection manager and cleans up resources.
func (cm *ConnectionManager) Close() error {
	// Stop cleanup goroutine
	cm.cleanupOnce.Do(func() {
		close(cm.stopCleanup)
		if cm.cleanupTicker != nil {
			cm.cleanupTicker.Stop()
		}
	})

	// Close all connections
	cm.connectionsMu.Lock()
	for connectionID, connInfo := range cm.connections {
		if connInfo.Cancel != nil {
			connInfo.Cancel()
		}
		delete(cm.connections, connectionID)
	}
	cm.connectionCount = 0
	cm.connectionsMu.Unlock()

	// Clear rate limiters
	cm.rateLimitersMu.Lock()
	cm.rateLimiters = make(map[string]*RateLimiter)
	cm.rateLimitersMu.Unlock()

	return nil
}

// startCleanup starts the background cleanup goroutine.
func (cm *ConnectionManager) startCleanup() {
	ticker := time.NewTicker(cm.cleanupInterval)
	cm.cleanupTicker = ticker

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cm.cleanupExpiredConnections()
			case <-cm.stopCleanup:
				return
			}
		}
	}()
}

// cleanupExpiredConnections removes expired connections.
func (cm *ConnectionManager) cleanupExpiredConnections() {
	now := time.Now()
	var toRemove []string

	cm.connectionsMu.RLock()
	for connectionID, connInfo := range cm.connections {
		if now.Sub(connInfo.LastActivity) > cm.limits.ConnectionTimeout {
			toRemove = append(toRemove, connectionID)
		}
	}
	cm.connectionsMu.RUnlock()

	// Remove expired connections
	for _, connectionID := range toRemove {
		cm.logger.Debug("Cleaning up expired connection: %s", connectionID)
		cm.UnregisterConnection(connectionID)
	}

	if len(toRemove) > 0 {
		cm.logger.Debug("Cleaned up %d expired connections", len(toRemove))
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(maxTokens int) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: maxTokens, // Refill at the same rate as max tokens per second
		lastRefill: time.Now(),
	}
}

// Allow checks if an operation is allowed under the rate limit.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	// Refill tokens based on elapsed time
	if elapsed >= time.Second {
		tokensToAdd := int(elapsed.Seconds()) * rl.refillRate
		rl.tokens = min(rl.maxTokens, rl.tokens+tokensToAdd)
		rl.lastRefill = now
	}

	// Check if we have tokens available
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
