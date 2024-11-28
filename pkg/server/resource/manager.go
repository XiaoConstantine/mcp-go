package resource

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

// Manager handles resource operations and maintains resource state
type Manager struct {
	// Protects access to resources map
	mu sync.RWMutex

	// Map of resource URIs to their metadata
	resources map[string]*models.Resource

	// Root paths where resources can be found
	roots []models.Root

	// Map of subscribers to their subscription details
	subscribers map[string]map[chan<- *models.ResourceUpdatedNotification]struct{}
	subMu       sync.RWMutex
}

// subscription represents an active subscription
type subscription struct {
	ch      chan *models.ResourceUpdatedNotification
	uri     string
	manager *Manager
	once    sync.Once // Ensures we only close once
}

// NewManager creates a new resource manager instance
func NewManager() *Manager {
	return &Manager{
		resources:   make(map[string]*models.Resource),
		subscribers: make(map[string]map[chan<- *models.ResourceUpdatedNotification]struct{}),
	}
}

// AddRoot adds a new root path to the manager
func (m *Manager) AddRoot(root models.Root) error {
	// Verify the root path exists and is accessible
	if !strings.HasPrefix(root.URI, "file://") {
		return fmt.Errorf("only file:// URIs are supported for roots")
	}

	path := strings.TrimPrefix(root.URI, "file://")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("root path not accessible: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if root already exists
	for _, existing := range m.roots {
		if existing.URI == root.URI {
			return fmt.Errorf("root already exists: %s", root.URI)
		}
	}

	m.roots = append(m.roots, root)

	// Scan the root for resources
	return m.scanRoot(root)
}

// scanRoot scans a root path for resources and adds them to the manager
func (m *Manager) scanRoot(root models.Root) error {
	path := strings.TrimPrefix(root.URI, "file://")

	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Create resource URI
		relPath, err := filepath.Rel(path, filePath)
		if err != nil {
			return err
		}

		uri := fmt.Sprintf("file://%s", filepath.Join(path, relPath))

		// Create resource metadata
		resource := &models.Resource{
			Name:     filepath.Base(filePath),
			URI:      uri,
			MimeType: detectMimeType(filePath),
		}

		// Add resource to map
		m.resources[uri] = resource

		return nil
	})
}

// ListResources returns a list of available resources
func (m *Manager) ListResources(ctx context.Context, cursor *models.Cursor) ([]models.Resource, *models.Cursor, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var resources []models.Resource
	for _, r := range m.resources {
		resources = append(resources, *r)
	}

	return resources, nil, nil
}

// ReadResource reads the contents of a specific resource
func (m *Manager) ReadResource(ctx context.Context, uri string) ([]models.ResourceContent, error) {
	m.mu.RLock()
	resource, exists := m.resources[uri]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	// Handle file:// URIs
	if strings.HasPrefix(uri, "file://") {
		path := strings.TrimPrefix(uri, "file://")
		return m.readFileResource(path, resource)
	}

	return nil, fmt.Errorf("unsupported URI scheme: %s", uri)
}

// readFileResource reads a file-based resource
func (m *Manager) readFileResource(path string, resource *models.Resource) ([]models.ResourceContent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open resource: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}

	// Determine if content should be treated as text or binary
	if isTextMimeType(resource.MimeType) {
		content := models.TextResourceContents{
			ResourceContents: models.ResourceContents{
				URI:      resource.URI,
				MimeType: resource.MimeType,
			},
			Text: string(data),
		}
		return []models.ResourceContent{content}, nil
	}

	content := models.BlobResourceContents{
		ResourceContents: models.ResourceContents{
			URI:      resource.URI,
			MimeType: resource.MimeType,
		},
		Blob: base64.StdEncoding.EncodeToString(data),
	}
	return []models.ResourceContent{content}, nil
}

// Subscribe adds a subscriber for resource updates
func (m *Manager) Subscribe(uri string) (*subscription, error) {
	m.subMu.Lock()
	defer m.subMu.Unlock()

	// Verify resource exists
	m.mu.RLock()
	if _, exists := m.resources[uri]; !exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
	m.mu.RUnlock()

	// Create notification channel
	ch := make(chan *models.ResourceUpdatedNotification, 10)

	// Initialize subscribers map for this URI if needed
	if _, exists := m.subscribers[uri]; !exists {
		m.subscribers[uri] = make(map[chan<- *models.ResourceUpdatedNotification]struct{})
	}

	// Add subscriber
	m.subscribers[uri][ch] = struct{}{}

	return &subscription{
		ch:      ch,
		uri:     uri,
		manager: m,
	}, nil
}

// Close closes the subscription and removes it from the manager
func (s *subscription) Close() {
	s.once.Do(func() {
		s.manager.subMu.Lock()
		defer s.manager.subMu.Unlock()

		if subscribers, exists := s.manager.subscribers[s.uri]; exists {
			delete(subscribers, s.ch)
			if len(subscribers) == 0 {
				delete(s.manager.subscribers, s.uri)
			}
		}
		close(s.ch)
	})
}

// Channel returns the notification channel for this subscription
func (s *subscription) Channel() <-chan *models.ResourceUpdatedNotification {
	return s.ch
}

// notifyResourceChanged notifies subscribers of resource changes
func (m *Manager) notifyResourceChanged(uri string) {
	m.subMu.RLock()
	subscribers := m.subscribers[uri]
	m.subMu.RUnlock()

	notification := &models.ResourceUpdatedNotification{}
	notification.Params.URI = uri

	for ch := range subscribers {
		select {
		case ch <- notification:
		default:
			// If channel is full, skip notification
		}
	}
}

// Helper functions for MIME type detection and text content checking
func detectMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

func isTextMimeType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "text/") ||
		mimeType == "application/json" ||
		mimeType == "application/javascript"
}
