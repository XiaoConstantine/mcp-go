package models

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCancelledNotification(t *testing.T) {
	// Test creating a cancelled notification with both request ID and reason
	requestID := "test-request-123"
	reason := "Operation timed out"
	notification := NewCancelledNotification(requestID, reason)

	if notification.Method() != "notifications/cancelled" {
		t.Errorf("Expected method notifications/cancelled, got %s", notification.Method())
	}

	if notification.Params.RequestID != requestID {
		t.Errorf("Expected request ID %s, got %v", requestID, notification.Params.RequestID)
	}

	if notification.Params.Reason != reason {
		t.Errorf("Expected reason %s, got %s", reason, notification.Params.Reason)
	}

	// Test JSON serialization
	data, err := json.Marshal(notification)
	if err != nil {
		t.Fatalf("Failed to marshal notification: %v", err)
	}

	var decoded CancelledNotification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal notification: %v", err)
	}

	if decoded.Method() != notification.Method() {
		t.Errorf("Methods don't match after marshal/unmarshal")
	}
}

func TestProgressNotification(t *testing.T) {
	// Test creating a progress notification with all fields
	progress := 0.75
	total := 1.0
	token := "progress-token-123"
	notification := NewProgressNotification(progress, &total, token)

	if notification.Method() != "notifications/progress" {
		t.Errorf("Expected method notifications/progress, got %s", notification.Method())
	}

	if notification.Params.Progress != progress {
		t.Errorf("Expected progress %f, got %f", progress, notification.Params.Progress)
	}

	if *notification.Params.Total != total {
		t.Errorf("Expected total %f, got %f", total, *notification.Params.Total)
	}

	if notification.Params.ProgressToken != token {
		t.Errorf("Expected token %v, got %v", token, notification.Params.ProgressToken)
	}

	// Test with nil total
	notificationNoTotal := NewProgressNotification(progress, nil, token)
	if notificationNoTotal.Params.Total != nil {
		t.Error("Expected nil total, got non-nil value")
	}
}

func TestLoggingMessageNotification(t *testing.T) {
	// Test creating a logging message notification
	level := LogLevelInfo
	data := "Test log message"
	logger := "test-logger"
	notification := NewLoggingMessageNotification(level, data, logger)

	if notification.Method() != "notifications/message" {
		t.Errorf("Expected method notifications/message, got %s", notification.Method())
	}

	if notification.Params.Level != level {
		t.Errorf("Expected level %s, got %s", level, notification.Params.Level)
	}

	if notification.Params.Data != data {
		t.Errorf("Expected data %v, got %v", data, notification.Params.Data)
	}

	if notification.Params.Logger != logger {
		t.Errorf("Expected logger %s, got %s", logger, notification.Params.Logger)
	}

	// Test with structured data
	structuredData := map[string]interface{}{
		"code":    404,
		"message": "Not found",
	}
	notificationStructured := NewLoggingMessageNotification(LogLevelError, structuredData, logger)
	if diff := cmp.Diff(notificationStructured.Params.Data, structuredData); diff != "" {
		t.Errorf("Capabilities mismatch (-want +got):\n%s", diff)
	}
}

func TestResourceNotifications(t *testing.T) {
	// Test ResourceListChangedNotification
	listChanged := NewResourceListChangedNotification()
	if listChanged.Method() != "notifications/resources/list_changed" {
		t.Errorf("Expected method notifications/resources/list_changed, got %s", listChanged.Method())
	}

	// Test ResourceUpdatedNotification
	uri := "file:///test/resource.txt"
	updated := NewResourceUpdatedNotification(uri)
	if updated.Method() != "notifications/resources/updated" {
		t.Errorf("Expected method notifications/resources/updated, got %s", updated.Method())
	}
	if updated.Params.URI != uri {
		t.Errorf("Expected URI %s, got %s", uri, updated.Params.URI)
	}

	// Test JSON serialization of resource notifications
	data, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("Failed to marshal ResourceUpdatedNotification: %v", err)
	}

	var decoded ResourceUpdatedNotification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ResourceUpdatedNotification: %v", err)
	}

	if decoded.Params.URI != uri {
		t.Errorf("URI doesn't match after marshal/unmarshal")
	}
}

func TestListChangedNotifications(t *testing.T) {
	// Test all list changed notifications
	tests := []struct {
		name     string
		factory  func() Notification
		expected string
	}{
		{
			name:     "PromptListChanged",
			factory:  func() Notification { return NewPromptListChangedNotification() },
			expected: "notifications/prompts/list_changed",
		},
		{
			name:     "ToolListChanged",
			factory:  func() Notification { return NewToolListChangedNotification() },
			expected: "notifications/tools/list_changed",
		},
		{
			name:     "RootsListChanged",
			factory:  func() Notification { return NewRootsListChangedNotification() },
			expected: "notifications/roots/list_changed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			notification := test.factory()
			if notification.Method() != test.expected {
				t.Errorf("Expected method %s, got %s", test.expected, notification.Method())
			}

			// Test JSON serialization
			data, err := json.Marshal(notification)
			if err != nil {
				t.Fatalf("Failed to marshal notification: %v", err)
			}

			var decoded map[string]interface{}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal notification: %v", err)
			}

			if method, ok := decoded["method"].(string); !ok || method != test.expected {
				t.Errorf("Expected method %s in JSON, got %v", test.expected, decoded["method"])
			}
		})
	}
}
