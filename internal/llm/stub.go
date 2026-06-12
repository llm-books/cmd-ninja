package llm

import (
	"context"
	"strings"
)

// StubProvider returns canned suggestions without any network call.
// It exists so the placement and safety layers can be built, demoed, and tested before (and without) a real model.
type StubProvider struct{}

func (StubProvider) Name() string { return "stub" }

func (StubProvider) Translate(_ context.Context, req Request) (Suggestion, error) {
	q := strings.ToLower(req.Query)
	if q == "" {
		q = strings.ToLower(req.User)
	}
	switch {
	case strings.Contains(q, "delete") || strings.Contains(q, "remove"):
		return Suggestion{
			Command: "rm -rf my-folder",
			Explanation: []Token{
				{Text: "rm", Note: "remove"},
				{Text: "-r", Note: "recurse into subfolders"},
				{Text: "-f", Note: "force, no prompts"},
				{Text: "my-folder", Note: "the target"},
			},
			ClaimedRisk: "destructive",
		}, nil
	default:
		return Suggestion{
			Command: "find . -type f -size +100M",
			Explanation: []Token{
				{Text: "find", Note: "start a file search"},
				{Text: ".", Note: "...in the current folder and below"},
				{Text: "-type f", Note: "match files only (not directories)"},
				{Text: "-size", Note: "filter by size"},
				{Text: "+100M", Note: "larger than 100 megabytes"},
			},
			ClaimedRisk: "read_only",
		}, nil
	}
}
