package gemini

// generateContentRequest is the request body for the Gemini
// models/{model}:generateContent endpoint. Only the fields the app uses are
// modelled.
type generateContentRequest struct {
	Contents          []Content         `json:"contents"`
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
}

// Content is one turn of the conversation (a role plus its parts).
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

// Part is a single chunk of content: either text or inline binary data
// (e.g. a label image).
type Part struct {
	Text       string `json:"text,omitempty"`
	InlineData *Blob  `json:"inlineData,omitempty"`
}

// Blob carries inline binary data as base64, with its MIME type.
type Blob struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

// GenerationConfig tunes the model's output.
type GenerationConfig struct {
	Temperature      float64 `json:"temperature,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	ResponseMIMEType string  `json:"responseMimeType,omitempty"`
}

// generateContentResponse is the subset of the API response the app reads.
type generateContentResponse struct {
	Candidates []struct {
		Content      Content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback,omitempty"`
}

// apiError models the JSON error envelope returned on non-2xx responses.
type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}
