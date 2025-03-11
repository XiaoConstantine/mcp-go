package errors

import (
	"errors"
	"testing"
	"time"
)

func TestTimeoutError(t *testing.T) {
	// Create a timeout error with a specific duration
	duration := 5 * time.Second
	err := &TimeoutError{Duration: duration}

	// Test the Error() method
	expected := "request timed out after 5s"
	if err.Error() != expected {
		t.Errorf("TimeoutError.Error() = %q, want %q", err.Error(), expected)
	}

	// Test IsTimeout function
	if !IsTimeout(err) {
		t.Errorf("IsTimeout(err) = false, want true")
	}

	// Test IsTimeout with a non-timeout error
	otherErr := errors.New("some other error")
	if IsTimeout(otherErr) {
		t.Errorf("IsTimeout(otherErr) = true, want false")
	}
}

func TestProtocolError(t *testing.T) {
	// Test without data
	err1 := &ProtocolError{
		Code:    400,
		Message: "Bad Request",
	}
	expected1 := "protocol error 400: Bad Request"
	if err1.Error() != expected1 {
		t.Errorf("ProtocolError.Error() = %q, want %q", err1.Error(), expected1)
	}

	// Test with data
	err2 := &ProtocolError{
		Code:    404,
		Message: "Not Found",
		Data:    "resource",
	}
	expected2 := "protocol error 404: Not Found (data: resource)"
	if err2.Error() != expected2 {
		t.Errorf("ProtocolError.Error() = %q, want %q", err2.Error(), expected2)
	}

	// Test IsProtocolError function
	if !IsProtocolError(err1) {
		t.Errorf("IsProtocolError(err1) = false, want true")
	}

	// Test IsProtocolError with a non-protocol error
	otherErr := errors.New("some other error")
	if IsProtocolError(otherErr) {
		t.Errorf("IsProtocolError(otherErr) = true, want false")
	}
}

func TestSentinelErrors(t *testing.T) {
	// Test ErrNotInitialized
	if ErrNotInitialized.Error() != "client not initialized" {
		t.Errorf("ErrNotInitialized.Error() = %q, want %q", ErrNotInitialized.Error(), "client not initialized")
	}

	// Test ErrConnClosed
	if ErrConnClosed.Error() != "connection closed" {
		t.Errorf("ErrConnClosed.Error() = %q, want %q", ErrConnClosed.Error(), "connection closed")
	}
}
