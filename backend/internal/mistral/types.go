package mistral

// chatCompletionRequest is the request body for POST /v1/chat/completions.
// Only the fields the app uses are modelled.
type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Message is one turn of the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the subset of the API response the app reads.
type chatCompletionResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// apiError models the JSON error envelope returned on non-2xx responses.
// Mistral returns two shapes depending on the failure: a bare {"message": ...}
// for auth and rate limits, and a nested {"error": {"message": ...}} for
// request validation.
type apiError struct {
	Message string `json:"message"`
	Error   struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}
