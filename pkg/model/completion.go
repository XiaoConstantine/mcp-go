package models

import "fmt"

// CompleteRequest represents a request from the client to the server for completion options.
// It is used when the client needs suggestions for completing a partial input.
type CompleteRequest struct {
	Method string `json:"method"`
	Params struct {
		// Argument contains information about the value to be completed
		Argument struct {
			// Name is the identifier of the argument being completed
			Name string `json:"name"`
			// Value is the current partial value to use for completion matching
			Value string `json:"value"`
		} `json:"argument"`
		// Ref can be either a PromptReference or ResourceReference
		Ref interface{} `json:"ref"`
	} `json:"params"`
}

// CompleteResult represents the server's response to a completion request.
// It provides a list of possible completion values and metadata about the results.
type CompleteResult struct {
	Completion struct {
		// Values contains the possible completion options
		Values []string `json:"values"`
		// HasMore indicates whether additional completion options exist beyond those provided
		HasMore bool `json:"hasMore,omitempty"`
		// Total represents the total number of completion options available
		Total *int `json:"total,omitempty"`
	} `json:"completion"`
}

// NewCompleteRequest creates a new completion request with the specified parameters.
// This helper function ensures all required fields are properly initialized.
func NewCompleteRequest(argumentName, argumentValue string, ref interface{}) *CompleteRequest {
	req := &CompleteRequest{
		Method: "completion/complete",
	}
	req.Params.Argument.Name = argumentName
	req.Params.Argument.Value = argumentValue
	req.Params.Ref = ref
	return req
}

// NewCompleteResult creates a new completion result with the specified values.
// This helper function provides a convenient way to create a properly structured response.
func NewCompleteResult(values []string, hasMore bool, total *int) *CompleteResult {
	result := &CompleteResult{}
	result.Completion.Values = values
	result.Completion.HasMore = hasMore
	result.Completion.Total = total
	return result
}

// ValidateCompleteRequest checks if a completion request contains all required fields
// and has valid values. It returns an error if the request is invalid.
func ValidateCompleteRequest(req *CompleteRequest) error {
	if req.Method != "completion/complete" {
		return fmt.Errorf("invalid method: expected 'completion/complete', got '%s'", req.Method)
	}
	if req.Params.Argument.Name == "" {
		return fmt.Errorf("argument name is required")
	}
	if req.Params.Ref == nil {
		return fmt.Errorf("reference is required")
	}

	// Check that ref is either a PromptReference or ResourceReference
	switch ref := req.Params.Ref.(type) {
	case PromptReference:
		if ref.Name == "" {
			return fmt.Errorf("prompt reference name is required")
		}
	case ResourceReference:
		if ref.URI == "" {
			return fmt.Errorf("resource reference URI is required")
		}
	default:
		return fmt.Errorf("reference must be either a PromptReference or ResourceReference")
	}

	return nil
}

// ValidateCompleteResult checks if a completion result contains all required fields
// and has valid values. It returns an error if the result is invalid.
func ValidateCompleteResult(result *CompleteResult) error {
	if result.Completion.Values == nil {
		return fmt.Errorf("completion values array is required")
	}

	// Check that total, if provided, is not less than the number of values
	if result.Completion.Total != nil && *result.Completion.Total < len(result.Completion.Values) {
		return fmt.Errorf("total cannot be less than the number of provided values")
	}

	return nil
}
