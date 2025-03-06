package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ConnectionManager handles reconnection logic.
type ConnectionManager struct {
	server     *Server
	retryCount int
	maxRetries int
	retryDelay time.Duration
	mu         sync.Mutex
	connected  bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(server *Server, maxRetries int, retryDelay time.Duration) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConnectionManager{
		server:     server,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins the connection process with retry logic.
func (cm *ConnectionManager) Start() error {
	var lastError error

	for cm.retryCount < cm.maxRetries || cm.maxRetries <= 0 {
		// Check if we're already connected
		cm.mu.Lock()
		if cm.connected {
			cm.mu.Unlock()
			return nil
		}
		cm.mu.Unlock()

		// Create a channel to monitor server errors
		errCh := make(chan error, 1)

		// Start server in a goroutine
		go func() {
			err := cm.server.Start()
			errCh <- err
		}()

		// Wait for error or cancellation
		select {
		case err := <-errCh:
			// If no error or a deliberate shutdown, we're done
			if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				cm.mu.Lock()
				cm.connected = true
				cm.mu.Unlock()
				return nil
			}

			// Log the error
			fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)

			// Keep track of the last error
			lastError = err

			// Increment retry count
			cm.retryCount++

			// Check if we should stop retrying
			if cm.maxRetries > 0 && cm.retryCount >= cm.maxRetries {
				break
			}

			// Wait before retry
			fmt.Fprintf(os.Stderr, "Retrying in %v (attempt %d/%d)...\n",
				cm.retryDelay, cm.retryCount, cm.maxRetries)
			time.Sleep(cm.retryDelay)

		case <-cm.ctx.Done():
			// Externally cancelled
			return cm.ctx.Err()
		}
	}

	return fmt.Errorf("failed to establish connection after %d attempts: %v",
		cm.retryCount, lastError)
}

// Stop cancels any ongoing connection attempts.
func (cm *ConnectionManager) Stop() {
	cm.cancel()
	if err := cm.server.Stop(); err != nil {
		// Log the error or handle it appropriately
		fmt.Fprintf(os.Stderr, "Error while stopping server: %v\n", err)
	}
}
