package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseSuggestion(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantCmd string
		wantErr bool
	}{
		{"clean json", `{"command": "ls -la", "explanation": [{"token": "ls", "note": "list"}], "risk": "read_only"}`, "ls -la", false},
		{"fenced", "```json\n{\"command\": \"pwd\", \"explanation\": [], \"risk\": \"read_only\"}\n```", "pwd", false},
		{"prose around", `Sure! {"command": "df -h", "explanation": [], "risk": "read_only"} Hope that helps.`, "df -h", false},
		{"whitespace command", `{"command": "  du -sh  ", "explanation": [], "risk": "read_only"}`, "du -sh", false},
		{"no json", "I cannot help with that.", "", true},
		{"empty command", `{"command": "", "explanation": [], "risk": "read_only"}`, "", true},
		{"broken json", `{"command": "ls", `, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := ParseSuggestion(tc.raw)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && s.Command != tc.wantCmd {
				t.Errorf("Command = %q, want %q", s.Command, tc.wantCmd)
			}
		})
	}
}

func TestAnthropicProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic-version header")
		}
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req.System == "" || len(req.Messages) != 1 {
			t.Errorf("malformed request: %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": `{"command": "ls -la", "explanation": [{"token": "ls", "note": "list files"}], "risk": "read_only"}`},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_NINJA_KEY", "test-key")
	p := NewAnthropic("claude-haiku-4-5", "TEST_NINJA_KEY")
	p.BaseURL = srv.URL

	sug, err := p.Translate(context.Background(), Request{System: "sys", User: "show files"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if sug.Command != "ls -la" || sug.ClaimedRisk != "read_only" || len(sug.Explanation) != 1 {
		t.Errorf("unexpected suggestion: %+v", sug)
	}
}

func TestAnthropicProviderAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"type": "invalid_request_error", "message": "bad model"},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_NINJA_KEY", "test-key")
	p := NewAnthropic("nope", "TEST_NINJA_KEY")
	p.BaseURL = srv.URL

	if _, err := p.Translate(context.Background(), Request{User: "x"}); err == nil {
		t.Fatal("expected an error from API error response")
	}
}

func TestAnthropicNoKey(t *testing.T) {
	t.Setenv("TEST_NINJA_EMPTY", "")
	p := NewAnthropic("", "TEST_NINJA_EMPTY")
	if _, err := p.Translate(context.Background(), Request{User: "x"}); err == nil {
		t.Fatal("expected an error when the API key env is empty")
	}
}
