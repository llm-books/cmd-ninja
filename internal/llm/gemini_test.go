package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("missing x-goog-api-key header")
		}
		if !strings.Contains(r.URL.Path, "gemini-2.5-flash:generateContent") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req.SystemInstruction == nil || len(req.Contents) != 1 {
			t.Errorf("malformed request: %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"role": "model", "parts": []map[string]string{
					{"text": `{"command": "ls -la", "explanation": [{"token": "ls", "note": "list files"}], "risk": "read_only"}`},
				}}},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_NINJA_GEMINI_KEY", "test-key")
	p := NewGemini("", "TEST_NINJA_GEMINI_KEY")
	p.BaseURL = srv.URL

	sug, err := p.Translate(context.Background(), Request{System: "sys", User: "show files"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if sug.Command != "ls -la" || sug.ClaimedRisk != "read_only" || len(sug.Explanation) != 1 {
		t.Errorf("unexpected suggestion: %+v", sug)
	}
}

func TestGeminiProviderAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": 400, "status": "INVALID_ARGUMENT", "message": "bad model"},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_NINJA_GEMINI_KEY", "test-key")
	p := NewGemini("nope", "TEST_NINJA_GEMINI_KEY")
	p.BaseURL = srv.URL

	if _, err := p.Translate(context.Background(), Request{User: "x"}); err == nil {
		t.Fatal("expected an error from API error response")
	}
}

func TestGeminiNoKey(t *testing.T) {
	t.Setenv("TEST_NINJA_GEMINI_EMPTY", "")
	p := NewGemini("", "TEST_NINJA_GEMINI_EMPTY")
	if _, err := p.Translate(context.Background(), Request{User: "x"}); err == nil {
		t.Fatal("expected an error when the API key env is empty")
	}
}

func TestGeminiDefaults(t *testing.T) {
	p := NewGemini("", "")
	if p.Model != "gemini-2.5-flash" || p.APIKeyEnv != "GEMINI_API_KEY" {
		t.Errorf("unexpected defaults: model=%q keyEnv=%q", p.Model, p.APIKeyEnv)
	}
}
