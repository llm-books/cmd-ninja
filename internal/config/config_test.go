package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/llm-books/cmd-ninja/internal/safety"
	"github.com/llm-books/cmd-ninja/internal/teach"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("NINJA_CONFIG", filepath.Join(t.TempDir(), "nope.yaml"))
	cfg := Load()
	if cfg.Provider != "anthropic" || cfg.Hotkey != "ctrl-g" || cfg.Teach != "compact" {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if cfg.Model != "" || cfg.APIKeyEnv != "" {
		t.Errorf("Model/APIKeyEnv must default empty (providers fill their own): %+v", cfg)
	}
}

func TestLoadOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := `
teach: full
hotkey: ctrl-n
safety:
  autofill: [read_only]
  block: [mkfs]
  paranoid: true
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJA_CONFIG", path)
	cfg := Load()

	if cfg.Teach != "full" || cfg.Hotkey != "ctrl-n" {
		t.Errorf("overlay not applied: %+v", cfg)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("absent keys should keep defaults, got provider %q", cfg.Provider)
	}
	if cfg.TeachMode() != teach.ModeFull {
		t.Errorf("TeachMode = %v", cfg.TeachMode())
	}

	p := cfg.Policy()
	if !p.Paranoid || len(p.Autofill) != 1 || p.Autofill[0] != safety.RiskReadOnly {
		t.Errorf("policy conversion wrong: %+v", p)
	}
	if len(p.Block) != 1 || p.Block[0] != "mkfs" {
		t.Errorf("block list wrong: %+v", p.Block)
	}
}

func TestPolicyRefusesDestructiveAutofill(t *testing.T) {
	cfg := Default()
	cfg.Safety.Autofill = []string{"destructive", "blocked", "read_only"}
	p := cfg.Policy()
	for _, lvl := range p.Autofill {
		if lvl == safety.RiskDestructive || lvl == safety.RiskBlocked {
			t.Errorf("destructive/blocked must never enter the autofill policy: %+v", p)
		}
	}
}
