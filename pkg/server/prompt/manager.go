package prompt

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/model"
)

// Manager handles prompt operations and maintains prompt state.
type Manager struct {
	mu          sync.RWMutex
	prompts     map[string]*models.Prompt
	promptData  map[string][]models.PromptMessage
	completions map[string]map[string][]string // promptName -> argName -> possible values
}

// NewManager creates a new prompt manager.
func NewManager() *Manager {
	return &Manager{
		prompts:     make(map[string]*models.Prompt),
		promptData:  make(map[string][]models.PromptMessage),
		completions: make(map[string]map[string][]string),
	}
}

// RegisterPrompt adds a new prompt to the manager.
func (m *Manager) RegisterPrompt(prompt models.Prompt, messages []models.PromptMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.prompts[prompt.Name]; exists {
		return fmt.Errorf("prompt already exists: %s", prompt.Name)
	}

	m.prompts[prompt.Name] = &prompt
	m.promptData[prompt.Name] = messages
	m.completions[prompt.Name] = make(map[string][]string)

	return nil
}

// ListPrompts returns all registered prompts.
func (m *Manager) ListPrompts(ctx context.Context, cursor *models.Cursor) ([]models.Prompt, *models.Cursor, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var prompts []models.Prompt
	for _, p := range m.prompts {
		prompts = append(prompts, *p)
	}
	return prompts, nil, nil
}

// GetPrompt retrieves a specific prompt with optional templating.
func (m *Manager) GetPrompt(ctx context.Context, name string, args map[string]string) ([]models.PromptMessage, string, error) {
	m.mu.RLock()
	prompt, exists := m.prompts[name]
	messages := m.promptData[name]
	m.mu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("prompt not found: %s", name)
	}

	// Validate required arguments
	for _, arg := range prompt.Arguments {
		if arg.Required {
			if _, ok := args[arg.Name]; !ok {
				return nil, "", fmt.Errorf("missing required argument: %s", arg.Name)
			}
		}
	}

	// Apply template variables - this is a simplified implementation
	result := make([]models.PromptMessage, len(messages))
	for i, msg := range messages {
		if textContent, ok := msg.Content.(models.TextContent); ok {
			text := textContent.Text

			// Simple variable substitution
			for key, value := range args {
				// Replace {{key}} with value
				text = strings.Replace(text, "{{"+key+"}}", value, -1)
			}

			newContent := models.TextContent{
				Type: "text",
				Text: text,
			}

			result[i] = models.PromptMessage{
				Role:    msg.Role,
				Content: newContent,
			}
		} else {
			// For non-text content, just copy as is
			result[i] = msg
		}
	}

	return result, prompt.Description, nil
}

// RegisterCompletions adds completion options for a prompt argument.
func (m *Manager) RegisterCompletions(promptName, argName string, values []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.prompts[promptName]; !exists {
		return fmt.Errorf("prompt not found: %s", promptName)
	}

	// Find if the argument exists
	argExists := false
	for _, arg := range m.prompts[promptName].Arguments {
		if arg.Name == argName {
			argExists = true
			break
		}
	}

	if !argExists {
		return fmt.Errorf("argument not found: %s", argName)
	}

	m.completions[promptName][argName] = values
	return nil
}

// GetCompletions returns completion options for a prompt argument.
func (m *Manager) GetCompletions(promptName, argName, prefix string) ([]string, bool, *int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.prompts[promptName]; !exists {
		return nil, false, nil, fmt.Errorf("prompt not found: %s", promptName)
	}

	// Check completions
	values, exists := m.completions[promptName][argName]
	if !exists {
		return []string{}, false, nil, nil
	}

	// Filter by prefix
	var results []string
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			results = append(results, value)
		}
	}

	total := len(results)
	return results, false, &total, nil
}
