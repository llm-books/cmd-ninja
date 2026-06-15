package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/llm-books/cmd-ninja/internal/config"
	"github.com/llm-books/cmd-ninja/internal/contextcol"
	"github.com/llm-books/cmd-ninja/internal/llm"
	"github.com/llm-books/cmd-ninja/internal/prompt"
	"github.com/llm-books/cmd-ninja/internal/render"
	"github.com/llm-books/cmd-ninja/internal/safety"
	"github.com/llm-books/cmd-ninja/internal/teach"
	"github.com/llm-books/cmd-ninja/internal/wire"
)

//go:embed shell/ninja.zsh shell/ninja.bash shell/ninja.fish
var shellFS embed.FS

// version is stamped by GoReleaser via -ldflags at release time.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "ninja",
		Short:         "Plain English in, a safe shell command on your prompt line",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newTranslateCmd(), newInitCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "ninja: %v\n", err)
		os.Exit(1)
	}
}

func newTranslateCmd() *cobra.Command {
	var wireMode, dryRun bool
	var providerName string
	cmd := &cobra.Command{
		Use:   "translate [flags] -- <request>",
		Short: "Translate a plain-English request into a shell command",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				return fmt.Errorf("empty request")
			}
			return translate(cmd.Context(), query, providerName, wireMode, dryRun)
		},
	}
	cmd.Flags().BoolVar(&wireMode, "wire", false, "emit the machine-readable FILL/SHOW line on stdout (for shell widgets)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the INFO/RISK block but never emit a fillable command")
	cmd.Flags().StringVar(&providerName, "provider", "", "override the configured provider (anthropic|gemini|openai|mistral|groq|stub)")
	return cmd
}

func translate(ctx context.Context, query, providerName string, wireMode, dryRun bool) error {
	cfg := config.Load()
	provider := pickProvider(providerName, cfg)
	env := contextcol.Collect(true)

	sug, err := provider.Translate(ctx, prompt.Build(query, env))
	if err != nil {
		return err
	}

	// The safety engine re-classifies locally; sug.ClaimedRisk is deliberately ignored here.
	engine := safety.New(cfg.Policy())
	verdict := engine.Classify(sug.Command)

	rows := teach.Rows(sug, cfg.TeachMode())
	render.New().Block(rows, sug, verdict, env.EnvNote(), !wireMode)

	if wireMode {
		verb := wire.VerbShow
		if verdict.Place && !dryRun {
			verb = wire.VerbFill
		}
		if verdict.Risk == safety.RiskBlocked {
			verb = wire.VerbBlock
		}
		fmt.Println(wire.Encode(verb, sug.Command))
		return nil
	}

	// Direct CLI use (no widget capturing stdout): print the command
	// itself so it can be read, copied, or command-substituted
	// unless it is blocked, in which case stdout stays empty.
	if verdict.Risk != safety.RiskBlocked {
		fmt.Println(sug.Command)
	}
	return nil
}

func pickProvider(name string, cfg config.Config) llm.Provider {
	model, keyEnv := cfg.Model, cfg.APIKeyEnv
	if name == "" {
		name = cfg.Provider
	} else if name != cfg.Provider {
		// --provider overrides to a different provider: the configured
		// model and api_key_env belong to cfg.Provider, so fall back to
		// this provider's own defaults instead of dragging them along.
		model, keyEnv = "", ""
	}
	switch name {
	case "stub":
		return llm.StubProvider{}
	case "anthropic":
		p := llm.NewAnthropic(model, keyEnv)
		return fallbackIfNoKey(p, p.APIKeyEnv)
	case "gemini":
		p := llm.NewGemini(model, keyEnv)
		return fallbackIfNoKey(p, p.APIKeyEnv)
	case "openai":
		p := llm.NewOpenAI(model, keyEnv)
		return fallbackIfNoKey(p, p.APIKeyEnv)
	case "mistral":
		p := llm.NewMistral(model, keyEnv)
		return fallbackIfNoKey(p, p.APIKeyEnv)
	case "groq":
		p := llm.NewGroq(model, keyEnv)
		return fallbackIfNoKey(p, p.APIKeyEnv)
	default:
		fmt.Fprintf(os.Stderr, "  (unknown provider %q, want anthropic|gemini|openai|mistral|groq|stub — using the offline stub provider)\n", name)
		return llm.StubProvider{}
	}
}

// fallbackIfNoKey keeps the widgets usable without a key: warn once on
// stderr and serve the offline stub instead of erroring on every keypress.
func fallbackIfNoKey(p llm.Provider, keyEnv string) llm.Provider {
	if os.Getenv(keyEnv) == "" {
		fmt.Fprintf(os.Stderr, "  (no %s set — using the offline stub provider)\n", keyEnv)
		return llm.StubProvider{}
	}
	return p
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "init <shell>",
		Short:     "Print the shell hook (eval it in your rc file)",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"zsh", "bash", "fish"},
		RunE: func(_ *cobra.Command, args []string) error {
			shell := args[0]
			raw, err := shellFS.ReadFile("shell/ninja." + shell)
			if err != nil {
				return fmt.Errorf("unsupported shell %q (zsh|bash|fish)", shell)
			}
			hook := strings.ReplaceAll(string(raw), "%HOTKEY%", hotkeyFor(shell, config.Load().Hotkey))
			fmt.Print(hook)
			return nil
		},
	}
	return cmd
}

// hotkeyFor converts a config-style key name ("ctrl-g") into each shell's binding notation.
func hotkeyFor(shell, key string) string {
	letter := "g"
	if strings.HasPrefix(key, "ctrl-") && len(key) == len("ctrl-")+1 {
		letter = strings.ToLower(key[len("ctrl-"):])
	}
	switch shell {
	case "zsh":
		return "^" + strings.ToUpper(letter)
	case "bash":
		return `\C-` + letter
	case "fish":
		return `\c` + letter
	}
	return "^" + strings.ToUpper(letter)
}
