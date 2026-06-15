package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func compatServer(t *testing.T, wantModel string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing Bearer authorization header")
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req.Model != wantModel {
			t.Errorf("model = %q, want %q", req.Model, wantModel)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Errorf("malformed messages: %+v", req.Messages)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{
					"role":    "assistant",
					"content": `{"command": "ls -la", "explanation": [{"token": "ls", "note": "list files"}], "risk": "read_only"}`,
				}},
			},
		})
	}))
}

func TestOpenAIProvider(t *testing.T) {
	srv := compatServer(t, "gpt-5-mini")
	defer srv.Close()

	t.Setenv("TEST_NINJA_OPENAI_KEY", "test-key")
	p := NewOpenAI("", "TEST_NINJA_OPENAI_KEY")
	p.BaseURL = srv.URL

	sug, err := p.Translate(context.Background(), Request{System: "sys", User: "show files"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if sug.Command != "ls -la" || sug.ClaimedRisk != "read_only" {
		t.Errorf("unexpected suggestion: %+v", sug)
	}
}

func TestMistralProvider(t *testing.T) {
	srv := compatServer(t, "mistral-small-latest")
	defer srv.Close()

	t.Setenv("TEST_NINJA_MISTRAL_KEY", "test-key")
	p := NewMistral("", "TEST_NINJA_MISTRAL_KEY")
	p.BaseURL = srv.URL

	sug, err := p.Translate(context.Background(), Request{System: "sys", User: "show files"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if sug.Command != "ls -la" {
		t.Errorf("unexpected suggestion: %+v", sug)
	}
}

func TestGroqProvider(t *testing.T) {
	srv := compatServer(t, "llama-3.1-8b-instant")
	defer srv.Close()

	t.Setenv("TEST_NINJA_GROQ_KEY", "test-key")
	p := NewGroq("", "TEST_NINJA_GROQ_KEY")
	p.BaseURL = srv.URL

	sug, err := p.Translate(context.Background(), Request{System: "sys", User: "show files"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if sug.Command != "ls -la" {
		t.Errorf("unexpected suggestion: %+v", sug)
	}
}

func TestCompatAPIErrorShapes(t *testing.T) {
	// OpenAI-style nested error
	openaiErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"type": "invalid_request_error", "message": "bad key"},
		})
	}))
	defer openaiErr.Close()

	// Mistral-style top-level message
	mistralErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"message": "Unauthorized", "object": "error"})
	}))
	defer mistralErr.Close()

	t.Setenv("TEST_NINJA_COMPAT_KEY", "test-key")
	for _, srv := range []*httptest.Server{openaiErr, mistralErr} {
		p := NewOpenAI("", "TEST_NINJA_COMPAT_KEY")
		p.BaseURL = srv.URL
		if _, err := p.Translate(context.Background(), Request{User: "x"}); err == nil {
			t.Errorf("expected an error from %s", srv.URL)
		}
	}
}

func TestCompatNoKey(t *testing.T) {
	t.Setenv("TEST_NINJA_COMPAT_EMPTY", "")
	for _, p := range []*OpenAICompatProvider{
		NewOpenAI("", "TEST_NINJA_COMPAT_EMPTY"),
		NewMistral("", "TEST_NINJA_COMPAT_EMPTY"),
		NewGroq("", "TEST_NINJA_COMPAT_EMPTY"),
	} {
		if _, err := p.Translate(context.Background(), Request{User: "x"}); err == nil {
			t.Errorf("%s: expected an error when the API key env is empty", p.Brand)
		}
	}
}

func TestCompatDefaults(t *testing.T) {
	o := NewOpenAI("", "")
	if o.Model != "gpt-5-mini" || o.APIKeyEnv != "OPENAI_API_KEY" || o.Name() != "openai" {
		t.Errorf("openai defaults wrong: %+v", o)
	}
	m := NewMistral("", "")
	if m.Model != "mistral-small-latest" || m.APIKeyEnv != "MISTRAL_API_KEY" || m.Name() != "mistral" {
		t.Errorf("mistral defaults wrong: %+v", m)
	}
	g := NewGroq("", "")
	if g.Model != "llama-3.1-8b-instant" || g.APIKeyEnv != "GROQ_API_KEY" || g.Name() != "groq" || g.BaseURL != "https://api.groq.com/openai" {
		t.Errorf("groq defaults wrong: %+v", g)
	}
}
