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

// OpenAICompatProvider speaks the chat-completions protocol shared by
// OpenAI, Mistral, and many self-hosted gateways: POST
// /v1/chat/completions with a Bearer key. One client, several brands.
type OpenAICompatProvider struct {
	Brand     string // "openai", "mistral", ...
	Model     string
	APIKeyEnv string // name of the env var holding the key; never stored in config
	BaseURL   string // overridable for tests and gateways
	Client    *http.Client
}

func NewOpenAI(model, apiKeyEnv string) *OpenAICompatProvider {
	if model == "" {
		model = "gpt-5-mini"
	}
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	return &OpenAICompatProvider{
		Brand:     "openai",
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		BaseURL:   "https://api.openai.com",
		Client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func NewMistral(model, apiKeyEnv string) *OpenAICompatProvider {
	if model == "" {
		model = "mistral-small-latest"
	}
	if apiKeyEnv == "" {
		apiKeyEnv = "MISTRAL_API_KEY"
	}
	return &OpenAICompatProvider{
		Brand:     "mistral",
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		BaseURL:   "https://api.mistral.ai",
		Client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenAICompatProvider) Name() string { return p.Brand }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// No token cap on purpose: the OpenAI gpt-5 family rejects the legacy
// max_tokens field while Mistral still expects it, and a one-command
// JSON reply is short anyway. Omitting it keeps a single code path.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	// OpenAI nests errors under "error"; Mistral sometimes returns a
	// top-level message. Capture both.
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

func (p *OpenAICompatProvider) Translate(ctx context.Context, req Request) (Suggestion, error) {
	apiKey := os.Getenv(p.APIKeyEnv)
	if apiKey == "" {
		return Suggestion{}, fmt.Errorf("no API key: set %s (or use --provider stub to try without a model)", p.APIKeyEnv)
	}

	var messages []chatMessage
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
	}
	messages = append(messages, chatMessage{Role: "user", Content: req.User})

	body, err := json.Marshal(chatRequest{Model: p.Model, Messages: messages})
	if err != nil {
		return Suggestion{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Suggestion{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return Suggestion{}, fmt.Errorf("calling %s API: %w", p.Brand, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Suggestion{}, err
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Suggestion{}, fmt.Errorf("decoding %s API response (HTTP %d): %w", p.Brand, resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return Suggestion{}, fmt.Errorf("%s API error (%s): %s", p.Brand, parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Message != "" {
			return Suggestion{}, fmt.Errorf("%s API error (HTTP %d): %s", p.Brand, resp.StatusCode, parsed.Message)
		}
		return Suggestion{}, fmt.Errorf("%s API returned HTTP %d", p.Brand, resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return Suggestion{}, fmt.Errorf("%s returned no choices", p.Brand)
	}

	return ParseSuggestion(parsed.Choices[0].Message.Content)
}
