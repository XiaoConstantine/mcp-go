package errors

import (
	"errors"
	"fmt"
	"time"
)

// Common sentinel errors
var (
	ErrNotInitialized = errors.New("client not initialized")
	ErrConnClosed     = errors.New("connection closed")
)

// TimeoutError is returned when a request times out.
type TimeoutError struct {
	Duration time.Duration
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("request timed out after %v", e.Duration)
}

// ProtocolError represents a protocol-level error.
type ProtocolError struct {
	Code    int
	Message string
	Data    interface{}
}

// Error implements the error interface.
func (e *ProtocolError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("protocol error %d: %s (data: %v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("protocol error %d: %s", e.Code, e.Message)
}

// IsTimeout checks if an error is a timeout error.
func IsTimeout(err error) bool {
	var timeoutErr *TimeoutError
	return errors.As(err, &timeoutErr)
}

// IsProtocolError checks if an error is a protocol error.
func IsProtocolError(err error) bool {
	var protocolErr *ProtocolError
	return errors.As(err, &protocolErr)
}
