package prompt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
	assert.Empty(t, manager.prompts)
	assert.Empty(t, manager.promptData)
	assert.Empty(t, manager.completions)
}

func TestRegisterPrompt(t *testing.T) {
	manager := NewManager()

	// Create a test prompt
	prompt := models.Prompt{
		Name:        "testPrompt",
		Description: "A test prompt",
		Arguments: []models.PromptArgument{
			{
				Name:        "arg1",
				Description: "First argument",
				Required:    true,
			},
		},
	}

	// Create test messages
	messages := []models.PromptMessage{
		{
			Role: models.RoleUser,
			Content: models.TextContent{
				Type: "text",
				Text: "This is a {{arg1}} prompt",
			},
		},
	}

	// Register prompt
	err := manager.RegisterPrompt(prompt, messages)
	require.NoError(t, err)

	// Verify prompt was registered
	assert.Contains(t, manager.prompts, "testPrompt")
	assert.Contains(t, manager.promptData, "testPrompt")
	assert.Contains(t, manager.completions, "testPrompt")
}

func TestRegisterPromptDuplicate(t *testing.T) {
	manager := NewManager()

	// Create a test prompt
	prompt := models.Prompt{
		Name: "testPrompt",
	}

	// Register prompt
	err := manager.RegisterPrompt(prompt, nil)
	require.NoError(t, err)

	// Try to register again
	err = manager.RegisterPrompt(prompt, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestListPrompts(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Register multiple prompts
	prompt1 := models.Prompt{Name: "prompt1"}
	prompt2 := models.Prompt{Name: "prompt2"}

	err := manager.RegisterPrompt(prompt1, nil)
	require.NoError(t, err)

	err = manager.RegisterPrompt(prompt2, nil)
	require.NoError(t, err)

	// List prompts
	prompts, cursor, err := manager.ListPrompts(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, cursor)
	assert.Len(t, prompts, 2)

	// Verify prompt names
	promptNames := []string{prompts[0].Name, prompts[1].Name}
	assert.Contains(t, promptNames, "prompt1")
	assert.Contains(t, promptNames, "prompt2")
}

func TestGetPrompt(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Create a test prompt with arguments
	prompt := models.Prompt{
		Name:        "testPrompt",
		Description: "A test prompt",
		Arguments: []models.PromptArgument{
			{
				Name:        "arg1",
				Description: "First argument",
				Required:    true,
			},
		},
	}

	// Create test messages with template variables
	messages := []models.PromptMessage{
		{
			Role: models.RoleUser,
			Content: models.TextContent{
				Type: "text",
				Text: "This is a {{arg1}} prompt",
			},
		},
	}

	// Register prompt
	err := manager.RegisterPrompt(prompt, messages)
	require.NoError(t, err)

	// Get prompt with arguments
	args := map[string]string{
		"arg1": "templated",
	}

	resultMessages, description, err := manager.GetPrompt(ctx, "testPrompt", args)
	require.NoError(t, err)
	assert.Equal(t, "A test prompt", description)
	assert.Len(t, resultMessages, 1)

	// Verify templating worked
	textContent, ok := resultMessages[0].Content.(models.TextContent)
	require.True(t, ok)
	assert.Equal(t, "This is a templated prompt", textContent.Text)
}

func TestGetPromptMissingArgument(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Create a test prompt with required argument
	prompt := models.Prompt{
		Name: "testPrompt",
		Arguments: []models.PromptArgument{
			{
				Name:     "arg1",
				Required: true,
			},
		},
	}

	// Register prompt
	err := manager.RegisterPrompt(prompt, nil)
	require.NoError(t, err)

	// Try to get prompt without required argument
	_, _, err = manager.GetPrompt(ctx, "testPrompt", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required argument")
}

func TestGetPromptNotFound(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Try to get nonexistent prompt
	_, _, err := manager.GetPrompt(ctx, "nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegisterCompletions(t *testing.T) {
	manager := NewManager()

	// Create and register a test prompt
	prompt := models.Prompt{
		Name: "testPrompt",
		Arguments: []models.PromptArgument{
			{Name: "arg1"},
		},
	}
	err := manager.RegisterPrompt(prompt, nil)
	require.NoError(t, err)

	// Register completions
	completions := []string{"value1", "value2", "value3"}
	err = manager.RegisterCompletions("testPrompt", "arg1", completions)
	require.NoError(t, err)

	// Verify completions were registered
	assert.Equal(t, completions, manager.completions["testPrompt"]["arg1"])
}

func TestRegisterCompletionsPromptNotFound(t *testing.T) {
	manager := NewManager()

	// Try to register completions for nonexistent prompt
	err := manager.RegisterCompletions("nonexistent", "arg1", []string{"value"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegisterCompletionsArgumentNotFound(t *testing.T) {
	manager := NewManager()

	// Create and register a test prompt
	prompt := models.Prompt{
		Name: "testPrompt",
		Arguments: []models.PromptArgument{
			{Name: "arg1"},
		},
	}
	err := manager.RegisterPrompt(prompt, nil)
	require.NoError(t, err)

	// Try to register completions for nonexistent argument
	err = manager.RegisterCompletions("testPrompt", "nonexistent", []string{"value"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetCompletions(t *testing.T) {
	manager := NewManager()

	// Create and register a test prompt
	prompt := models.Prompt{
		Name: "testPrompt",
		Arguments: []models.PromptArgument{
			{Name: "arg1"},
		},
	}
	err := manager.RegisterPrompt(prompt, nil)
	require.NoError(t, err)

	// Register completions
	completions := []string{"apple", "banana", "apricot", "avocado"}
	err = manager.RegisterCompletions("testPrompt", "arg1", completions)
	require.NoError(t, err)

	// Get completions with prefix
	results, hasMore, total, err := manager.GetCompletions("testPrompt", "arg1", "a")
	require.NoError(t, err)
	assert.Len(t, results, 3) // apple, apricot, avocado
	assert.False(t, hasMore)
	assert.NotNil(t, total)
	assert.Equal(t, 3, *total)

	// Test with different prefix
	results, hasMore, total, err = manager.GetCompletions("testPrompt", "arg1", "b")
	require.NoError(t, err)
	assert.Len(t, results, 1) // banana
	assert.False(t, hasMore)
	assert.NotNil(t, total)
	assert.Equal(t, 1, *total)
}

func TestGetCompletionsPromptNotFound(t *testing.T) {
	manager := NewManager()

	// Try to get completions for nonexistent prompt
	_, _, _, err := manager.GetCompletions("nonexistent", "arg1", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetCompletionsNoCompletions(t *testing.T) {
	manager := NewManager()

	// Create and register a test prompt
	prompt := models.Prompt{
		Name: "testPrompt",
		Arguments: []models.PromptArgument{
			{Name: "arg1"},
		},
	}
	err := manager.RegisterPrompt(prompt, nil)
	require.NoError(t, err)

	// Get completions without registering any
	results, hasMore, total, err := manager.GetCompletions("testPrompt", "arg1", "")
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.False(t, hasMore)
	assert.Nil(t, total)
}
