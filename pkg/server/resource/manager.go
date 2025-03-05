package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

// Manager handles resource operations and maintains resource state.
type Manager struct {
	mu          sync.RWMutex
	resources   map[string]*models.Resource
	contents    map[string]string // Store resource contents in memory
	roots       []models.Root
	cancel      context.CancelFunc
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

	resourceContent := &models.TextResourceContents{
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

// GetCompletions returns completion options for a resource argument.
// It analyzes the resource contents and returns possible completions
// that match the provided prefix.
func (m *Manager) GetCompletions(uri string, argName, prefix string) ([]string, bool, *int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if the resource exists
	resource, exists := m.resources[uri]
	content, contentExists := m.contents[uri]
	if !exists || !contentExists {
		return nil, false, nil, fmt.Errorf("resource not found: %s", uri)
	}

	// This is a basic implementation that could be expanded based on
	// the resource type and content analysis
	var results []string

	// If the resource is text-based, perform simple completion
	if strings.HasPrefix(resource.MimeType, "text/") {
		// Split content into words and find those matching the prefix
		words := strings.Fields(content)
		seen := make(map[string]struct{})

		for _, word := range words {
			// Normalize to lowercase for comparison
			normalizedWord := strings.ToLower(word)
			if strings.HasPrefix(normalizedWord, strings.ToLower(prefix)) && len(normalizedWord) > len(prefix) {
				// Avoid duplicates
				if _, ok := seen[normalizedWord]; !ok {
					results = append(results, normalizedWord)
					seen[normalizedWord] = struct{}{}
				}
			}
		}
	}

	// Sort results alphabetically for consistency
	sort.Strings(results)

	// Limit to first 100 results to avoid overwhelming the client
	hasMore := false
	if len(results) > 100 {
		total := len(results)
		results = results[:100]
		hasMore = true
		return results, hasMore, &total, nil
	}

	total := len(results)
	return results, false, &total, nil
}

// Shutdown performs orderly cleanup of all resources
func (m *Manager) Shutdown(ctx context.Context) error {
	// Use timeout to ensure shutdown doesn't block forever
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Close all subscriptions
	m.closeAllSubscriptions()

	// Wait a brief moment for subscription notifications to process
	select {
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	case <-time.After(100 * time.Millisecond):
		// Continue shutdown
	}

	// Release any file system monitors or watchers here

	// Clear internal data structures
	m.mu.Lock()
	m.resources = make(map[string]*models.Resource)
	m.contents = make(map[string]string)
	m.roots = nil
	m.mu.Unlock()

	// Force garbage collection to release resources
	// This is optional and may be excessive for most applications
	runtime.GC()

	return nil
}

// closeAllSubscriptions closes all active subscriptions
func (m *Manager) closeAllSubscriptions() {
	m.subMu.Lock()
	defer m.subMu.Unlock()

	// Create a list of all channels to close
	var subscriptions []chan<- *models.ResourceUpdatedNotification

	// Gather all subscription channels
	for _, subs := range m.subscribers {
		for ch := range subs {
			subscriptions = append(subscriptions, ch)
		}
	}

	// Close all channels and clear the map
	for _, ch := range subscriptions {
		// Use a type assertion with a channel conversion to close it
		// This is necessary because we can't close a channel of chan<- type
		if c, ok := interface{}(ch).(chan *models.ResourceUpdatedNotification); ok {
			close(c)
		}
	}

	// Clear the subscribers map
	m.subscribers = make(map[string]map[chan<- *models.ResourceUpdatedNotification]struct{})
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
