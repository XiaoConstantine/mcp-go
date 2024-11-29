package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

// Manager handles resource operations and maintains resource state.
type Manager struct {
	mu          sync.RWMutex
	resources   map[string]*models.Resource
	contents    map[string]string // Store resource contents in memory
	roots       []models.Root
	subscribers map[string]map[chan<- *models.ResourceUpdatedNotification]struct{}
	subMu       sync.RWMutex
}

// NewManager creates a new resource manager instance.
func NewManager() *Manager {
	return &Manager{
		resources:   make(map[string]*models.Resource),
		contents:    make(map[string]string),
		subscribers: make(map[string]map[chan<- *models.ResourceUpdatedNotification]struct{}),
	}
}

// AddRoot adds a new root path to the manager.
func (m *Manager) AddRoot(root models.Root) error {
	if !strings.HasPrefix(root.URI, "file://") {
		return fmt.Errorf("only file:// URIs are supported for roots")
	}

	path := strings.TrimPrefix(root.URI, "file://")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("root path not accessible: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.roots {
		if existing.URI == root.URI {
			return fmt.Errorf("root already exists: %s", root.URI)
		}
	}

	m.roots = append(m.roots, root)
	return m.scanRoot(root)
}

func (m *Manager) scanRoot(root models.Root) error {
	path := strings.TrimPrefix(root.URI, "file://")
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(path, filePath)
		if err != nil {
			return err
		}

		uri := fmt.Sprintf("file://%s", filepath.Join(path, relPath))
		mimeType := detectMimeType(filePath)

		// Read initial content
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read initial content: %w", err)
		}

		m.resources[uri] = &models.Resource{
			Name:     filepath.Base(filePath),
			URI:      uri,
			MimeType: mimeType,
		}
		m.contents[uri] = string(data)

		return nil
	})
}

// ListResources returns a list of available resources.
func (m *Manager) ListResources(ctx context.Context, cursor *models.Cursor) ([]models.Resource, *models.Cursor, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var resources []models.Resource
	for _, r := range m.resources {
		resources = append(resources, *r)
	}
	return resources, nil, nil
}

// ReadResource reads the contents of a specific resource.
func (m *Manager) ReadResource(ctx context.Context, uri string) ([]models.ResourceContent, error) {
	m.mu.RLock()
	resource, exists := m.resources[uri]
	content, contentExists := m.contents[uri]
	m.mu.RUnlock()

	if !exists || !contentExists {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	resourceContent := models.TextResourceContents{
		ResourceContents: models.ResourceContents{
			URI:      resource.URI,
			MimeType: resource.MimeType,
		},
		Text: content,
	}

	return []models.ResourceContent{resourceContent}, nil
}

// UpdateResource updates the content of a resource and notifies subscribers.
func (m *Manager) UpdateResource(uri string, content string) error {
	m.mu.Lock()
	_, exists := m.resources[uri]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("resource not found: %s", uri)
	}

	m.contents[uri] = content
	m.mu.Unlock()

	// Notify subscribers about the change
	m.notifyResourceChanged(uri)
	return nil
}

// Subscribe adds a subscriber for resource updates.
func (m *Manager) Subscribe(uri string) (*Subscription, error) {
	m.mu.RLock()
	_, exists := m.resources[uri]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	ch := make(chan *models.ResourceUpdatedNotification, 10)
	m.subMu.Lock()
	if _, exists := m.subscribers[uri]; !exists {
		m.subscribers[uri] = make(map[chan<- *models.ResourceUpdatedNotification]struct{})
	}
	m.subscribers[uri][ch] = struct{}{}
	m.subMu.Unlock()

	return &Subscription{
		ch:      ch,
		uri:     uri,
		manager: m,
	}, nil
}

// Subscription represents an active subscription to resource updates.
type Subscription struct {
	ch      chan *models.ResourceUpdatedNotification
	uri     string
	manager *Manager
	once    sync.Once
	closed  bool
	mu      sync.RWMutex
}

// Close closes the subscription and removes it from the manager.
func (s *Subscription) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		s.closed = true
		s.mu.Unlock()

		s.manager.subMu.Lock()
		if subscribers, exists := s.manager.subscribers[s.uri]; exists {
			delete(subscribers, s.ch)
			if len(subscribers) == 0 {
				delete(s.manager.subscribers, s.uri)
			}
		}
		s.manager.subMu.Unlock()
		close(s.ch)
	})
}

// Channel returns the notification channel for this subscription.
func (s *Subscription) Channel() <-chan *models.ResourceUpdatedNotification {
	return s.ch
}

// notifyResourceChanged notifies subscribers of resource changes.
func (m *Manager) notifyResourceChanged(uri string) {
	notification := &models.ResourceUpdatedNotification{
		BaseNotification: models.BaseNotification{
			NotificationMethod: "notifications/resources/updated",
		},
		Params: struct {
			URI string `json:"uri"`
		}{
			URI: uri,
		},
	}

	m.subMu.RLock()
	subscribers := make([]chan<- *models.ResourceUpdatedNotification, 0)
	if subs, exists := m.subscribers[uri]; exists {
		for ch := range subs {
			subscribers = append(subscribers, ch)
		}
	}
	m.subMu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- notification:
		default:
			// Channel full, skip notification
		}
	}
}
