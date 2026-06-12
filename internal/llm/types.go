package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Token is one annotated part of a suggested command, the ordered list drives the INFO block.
type Token struct {
	Text string `json:"token"` // "-type f"
	Note string `json:"note"`  // "match files only (not directories)"
}

// Suggestion is what a provider returns for a natural language request.
// ClaimedRisk is the model's own label and is advisory only, the safety
// engine re-classifies locally and has the final say.
type Suggestion struct {
	Command     string  `json:"command"`
	Explanation []Token `json:"explanation"`
	ClaimedRisk string  `json:"risk"`
}

// Request carries the fully built prompts to a provider. 
// Query is the user's raw request before prompt assembly, 
// offline providers (and logs) want it without the few-shot scaffolding.
type Request struct {
	System string
	User   string
	Query  string
}

// Provider turns a request into a single command suggestion.
type Provider interface {
	Name() string
	Translate(ctx context.Context, req Request) (Suggestion, error)
}

// ParseSuggestion decodes the JSON object a model returns, tolerating surrounding prose or 
// markdown fences by extracting the outermost {...} span.
func ParseSuggestion(raw string) (Suggestion, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return Suggestion{}, fmt.Errorf("no JSON object in model output: %q", truncate(raw, 120))
	}
	var s Suggestion
	if err := json.Unmarshal([]byte(raw[start:end+1]), &s); err != nil {
		return Suggestion{}, fmt.Errorf("decoding model output: %w", err)
	}
	if strings.TrimSpace(s.Command) == "" {
		return Suggestion{}, fmt.Errorf("model returned an empty command")
	}
	s.Command = strings.TrimSpace(s.Command)
	return s, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
