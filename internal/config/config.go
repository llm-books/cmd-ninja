// Package config loads ~/.config/cmd-ninja/config.yaml (or the file named by $NINJA_CONFIG). 
// A missing file is not an error: every field has a working default, and the API key always 
// comes from the environment never from the file.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/llm-books/cmd-ninja/internal/safety"
	"github.com/llm-books/cmd-ninja/internal/teach"
)

type Config struct {
	Provider  string       `yaml:"provider"`
	Model     string       `yaml:"model"`
	APIKeyEnv string       `yaml:"api_key_env"`
	Hotkey    string       `yaml:"hotkey"`
	Teach     string       `yaml:"teach"`
	Safety    SafetyConfig `yaml:"safety"`
}

type SafetyConfig struct {
	Autofill []string `yaml:"autofill"`
	Block    []string `yaml:"block"`
	Paranoid bool     `yaml:"paranoid"`
}

// Default leaves Model and APIKeyEnv empty on purpose: each provider
// fills in its own defaults (anthropic: claude-haiku-4-5 /
// ANTHROPIC_API_KEY, gemini: gemini-2.5-flash / GEMINI_API_KEY,
// openai: gpt-5-mini / OPENAI_API_KEY, mistral: mistral-small-latest /
// MISTRAL_API_KEY), so switching provider does not drag another
// provider's model along.
func Default() Config {
	return Config{
		Provider: "anthropic",
		Hotkey:   "ctrl-g",
		Teach:    "compact",
		Safety: SafetyConfig{
			Autofill: []string{"read_only", "modifies", "network"},
		},
	}
}

// Load and reads the config file if present and overlays it on Default.
func Load() Config {
	cfg := Default()
	path := os.Getenv("NINJA_CONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return cfg
		}
		path = filepath.Join(home, ".config", "cmd-ninja", "config.yaml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	// Unmarshal over the defaults, absent keys keep their default value.
	_ = yaml.Unmarshal(raw, &cfg)
	if cfg.Provider == "" {
		cfg.Provider = "anthropic"
	}
	if cfg.Hotkey == "" {
		cfg.Hotkey = "ctrl-g"
	}
	return cfg
}

// Policy converts the YAML tier names into the engine's policy.
// Destructive and blocked are stripped even if someone lists them
// the engine refuses them anyway (rething this!), but the policy should not pretend.
func (c Config) Policy() safety.Policy {
	p := safety.Policy{Block: c.Safety.Block, Paranoid: c.Safety.Paranoid}
	for _, name := range c.Safety.Autofill {
		level := safety.ParseRisk(name)
		if level == safety.RiskDestructive || level == safety.RiskBlocked {
			continue
		}
		p.Autofill = append(p.Autofill, level)
	}
	if len(p.Autofill) == 0 {
		p.Autofill = safety.DefaultPolicy().Autofill
	}
	return p
}

func (c Config) TeachMode() teach.Mode {
	return teach.ParseMode(c.Teach)
}
