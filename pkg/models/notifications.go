package models

import "github.com/XiaoConstantine/mcp-go/pkg/protocol"

// ProgressToken represents a token used to associate progress notifications with requests.
// It can be either a string or integer to match the JSON-RPC spec.
type ProgressToken interface{}

// Notification represents the base interface for all MCP notifications.
// Every notification type must provide its method name.
type Notification interface {
	Method() string
}

// BaseNotification provides common fields for all notifications.
// It implements the Notification interface and can be embedded in specific notification types.
type BaseNotification struct {
	NotificationMethod string `json:"method"`
}

func (n BaseNotification) Method() string {
	return n.NotificationMethod
}

// CancelledNotification represents a notification that a request is being cancelled.
// This can be sent by either side to indicate that a previously-issued request should be cancelled.
type CancelledNotification struct {
	BaseNotification
	Params struct {
		RequestID protocol.RequestID `json:"requestId"`
		Reason    string             `json:"reason,omitempty"`
	} `json:"params"`
}

// InitializedNotification represents the completion of initialization.
// This is sent from the client to the server after initialization has finished.
type InitializedNotification struct {
	BaseNotification
}

// ProgressNotification represents progress updates for long-running requests.
// It provides both current progress and optional total values for progress tracking.
type ProgressNotification struct {
	BaseNotification
	Params struct {
		Progress      float64     `json:"progress"`
		Total         *float64    `json:"total,omitempty"`
		ProgressToken interface{} `json:"progressToken"`
	} `json:"params"`
}

// ResourceListChangedNotification indicates the server's resource list has changed.
// This notification may be issued without any previous subscription from the client.
type ResourceListChangedNotification struct {
	BaseNotification
}

// ResourceUpdatedNotification indicates a specific resource has been updated.
// This is only sent if the client previously subscribed to resource updates.
type ResourceUpdatedNotification struct {
	BaseNotification
	Params struct {
		URI string `json:"uri"`
	} `json:"params"`
}

// PromptListChangedNotification indicates the server's prompt list has changed.
// This notification may be issued without any previous subscription from the client.
type PromptListChangedNotification struct {
	BaseNotification
}

// ToolListChangedNotification indicates the server's tool list has changed.
// This notification may be issued without any previous subscription from the client.
type ToolListChangedNotification struct {
	BaseNotification
}

// LoggingMessageNotification represents a log message from the server.
// If no logging/setLevel request has been sent, the server may decide which messages to send.
type LoggingMessageNotification struct {
	BaseNotification
	Params struct {
		Level  LoggingLevel `json:"level"`
		Data   interface{}  `json:"data"`
		Logger string       `json:"logger,omitempty"`
	} `json:"params"`
}

// RootsListChangedNotification indicates the client's root list has changed.
// This notification should be followed by the server requesting an updated list of roots.
type RootsListChangedNotification struct {
	BaseNotification
}

// NewCancelledNotification creates a new cancelled notification with the specified request ID and reason.
func NewCancelledNotification(requestID protocol.RequestID, reason string) *CancelledNotification {
	n := &CancelledNotification{}
	n.NotificationMethod = "notifications/cancelled"
	n.Params.RequestID = requestID
	n.Params.Reason = reason
	return n
}

// NewInitializedNotification creates a new initialized notification.
func NewInitializedNotification() *InitializedNotification {
	n := &InitializedNotification{}
	n.NotificationMethod = "notifications/initialized"
	return n
}

// NewProgressNotification creates a new progress notification with the specified progress details.
func NewProgressNotification(progress float64, total *float64, token interface{}) *ProgressNotification {
	n := &ProgressNotification{}
	n.NotificationMethod = "notifications/progress"
	n.Params.Progress = progress
	n.Params.Total = total
	n.Params.ProgressToken = token
	return n
}

// NewResourceListChangedNotification creates a new resource list changed notification.
func NewResourceListChangedNotification() *ResourceListChangedNotification {
	n := &ResourceListChangedNotification{}
	n.NotificationMethod = "notifications/resources/list_changed"
	return n
}

// NewResourceUpdatedNotification creates a new resource updated notification for the specified URI.
func NewResourceUpdatedNotification(uri string) *ResourceUpdatedNotification {
	n := &ResourceUpdatedNotification{}
	n.NotificationMethod = "notifications/resources/updated"
	n.Params.URI = uri
	return n
}

// NewPromptListChangedNotification creates a new prompt list changed notification.
func NewPromptListChangedNotification() *PromptListChangedNotification {
	n := &PromptListChangedNotification{}
	n.NotificationMethod = "notifications/prompts/list_changed"
	return n
}

// NewToolListChangedNotification creates a new tool list changed notification.
func NewToolListChangedNotification() *ToolListChangedNotification {
	n := &ToolListChangedNotification{}
	n.NotificationMethod = "notifications/tools/list_changed"
	return n
}

// NewLoggingMessageNotification creates a new logging message notification with the specified details.
func NewLoggingMessageNotification(level LoggingLevel, data interface{}, logger string) *LoggingMessageNotification {
	n := &LoggingMessageNotification{}
	n.NotificationMethod = "notifications/message"
	n.Params.Level = level
	n.Params.Data = data
	n.Params.Logger = logger
	return n
}

// NewRootsListChangedNotification creates a new roots list changed notification.
func NewRootsListChangedNotification() *RootsListChangedNotification {
	n := &RootsListChangedNotification{}
	n.NotificationMethod = "notifications/roots/list_changed"
	return n
}
