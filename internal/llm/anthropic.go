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

// AnthropicProvider calls the Claude Messages API directly over HTTP, no SDK dependency, keeping the binary lean.
type AnthropicProvider struct {
	Model     string
	APIKeyEnv string // name of the env var holding the key; never stored in config
	BaseURL   string // overridable for tests
	Client    *http.Client
}

func NewAnthropic(model, apiKeyEnv string) *AnthropicProvider {
	if model == "" {
		model = "claude-haiku-4-5"
	}
	if apiKeyEnv == "" {
		apiKeyEnv = "ANTHROPIC_API_KEY"
	}
	return &AnthropicProvider{
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		BaseURL:   "https://api.anthropic.com",
		Client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *AnthropicProvider) Translate(ctx context.Context, req Request) (Suggestion, error) {
	apiKey := os.Getenv(p.APIKeyEnv)
	if apiKey == "" {
		return Suggestion{}, fmt.Errorf("no API key: set %s (or use --provider stub to try without a model)", p.APIKeyEnv)
	}

	body, err := json.Marshal(anthropicRequest{
		Model:     p.Model,
		MaxTokens: 1024,
		System:    req.System,
		Messages:  []anthropicMessage{{Role: "user", Content: req.User}},
	})
	if err != nil {
		return Suggestion{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Suggestion{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return Suggestion{}, fmt.Errorf("calling Anthropic API: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Suggestion{}, err
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Suggestion{}, fmt.Errorf("decoding API response (HTTP %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return Suggestion{}, fmt.Errorf("Anthropic API error (%s): %s", parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return Suggestion{}, fmt.Errorf("Anthropic API returned HTTP %d", resp.StatusCode)
	}

	var text string
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return ParseSuggestion(text)
}
