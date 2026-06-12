package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// GeminiProvider calls the Gemini generateContent REST API directly —
// same no-SDK approach as the Anthropic provider.
type GeminiProvider struct {
	Model     string
	APIKeyEnv string // name of the env var holding the key; never stored in config
	BaseURL   string // overridable for tests
	Client    *http.Client
}

func NewGemini(model, apiKeyEnv string) *GeminiProvider {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if apiKeyEnv == "" {
		apiKeyEnv = "GEMINI_API_KEY"
	}
	return &GeminiProvider{
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		BaseURL:   "https://generativelanguage.googleapis.com",
		Client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *GeminiProvider) Name() string { return "gemini" }

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  struct {
		MaxOutputTokens int `json:"maxOutputTokens"`
	} `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *GeminiProvider) Translate(ctx context.Context, req Request) (Suggestion, error) {
	apiKey := os.Getenv(p.APIKeyEnv)
	if apiKey == "" {
		return Suggestion{}, fmt.Errorf("no API key: set %s (or use --provider stub to try without a model)", p.APIKeyEnv)
	}

	payload := geminiRequest{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: req.User}}}},
	}
	if req.System != "" {
		payload.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}
	payload.GenerationConfig.MaxOutputTokens = 1024

	body, err := json.Marshal(payload)
	if err != nil {
		return Suggestion{}, err
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.BaseURL, p.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Suggestion{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Key goes in a header, not the query string, so it can't leak into
	// logs that record URLs.
	httpReq.Header.Set("x-goog-api-key", apiKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return Suggestion{}, fmt.Errorf("calling Gemini API: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Suggestion{}, err
	}

	var parsed geminiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Suggestion{}, fmt.Errorf("decoding API response (HTTP %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return Suggestion{}, fmt.Errorf("Gemini API error (%s): %s", parsed.Error.Status, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return Suggestion{}, fmt.Errorf("Gemini API returned HTTP %d", resp.StatusCode)
	}
	if len(parsed.Candidates) == 0 {
		return Suggestion{}, fmt.Errorf("Gemini returned no candidates")
	}

	var text string
	for _, part := range parsed.Candidates[0].Content.Parts {
		text += part.Text
	}
	return ParseSuggestion(text)
}
