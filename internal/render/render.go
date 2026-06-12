// Package render prints the human-facing INFO block and risk badge.
// It writes to stderr (the tty) so the widgets' command substitution
// only captures the wire line on stdout.
package render

import (
	"fmt"
	"io"
	"os"

	"github.com/llm-books/cmd-ninja/internal/llm"
	"github.com/llm-books/cmd-ninja/internal/safety"
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
	red    = "\x1b[31m"
	cyan   = "\x1b[36m"
)

type Renderer struct {
	Out   io.Writer
	Color bool
}

func New() *Renderer {
	_, noColor := os.LookupEnv("NO_COLOR")
	return &Renderer{Out: os.Stderr, Color: !noColor}
}

func (r *Renderer) c(code, s string) string {
	if !r.Color {
		return s
	}
	return code + s + reset
}

// Block prints the INFO rows, the risk badge, the environment note,
// and (when the command will not be placed) the copy-or-type line.
// cmdOnStdout means the caller prints the command itself on stdout
// (direct CLI use), so the copy-or-type line would be a duplicate;
// the BLOCKED warning is shown regardless.
func (r *Renderer) Block(rows []llm.Token, sug llm.Suggestion, v safety.Verdict, envNote string, cmdOnStdout bool) {
	w := r.Out
	fmt.Fprintln(w)

	if len(rows) > 0 {
		fmt.Fprintf(w, "  %s\n", r.c(bold, "INFO:"))
		width := 0
		for _, t := range rows {
			if len(t.Text) > width {
				width = len(t.Text)
			}
		}
		for _, t := range rows {
			fmt.Fprintf(w, "    %s : %s\n", r.c(cyan, pad(t.Text, width)), t.Note)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "  %s %s", r.c(bold, "RISK:"), r.badge(v.Risk))
	if envNote != "" {
		fmt.Fprintf(w, "     %s", r.c(dim, envNote))
	}
	fmt.Fprintln(w)
	for _, reason := range v.Reasons {
		fmt.Fprintf(w, "    %s\n", r.c(dim, "· "+reason))
	}

	switch {
	case v.Risk == safety.RiskBlocked:
		fmt.Fprintf(w, "\n  %s  %s\n", r.c(red+bold, "BLOCKED:"), sug.Command)
		fmt.Fprintf(w, "  %s\n", r.c(dim, "Cmd Ninja will not place or run this. If you really mean it, type it yourself."))
	case !v.Place && !cmdOnStdout:
		fmt.Fprintf(w, "\n  %s  %s\n", r.c(bold, "CMD (copy or type it):"), r.c(bold, sug.Command))
	}
	fmt.Fprintln(w)
}

func (r *Renderer) badge(risk safety.RiskLevel) string {
	switch risk {
	case safety.RiskReadOnly:
		return r.c(green, "read-only ✓")
	case safety.RiskModifies:
		return r.c(yellow, "modifies files")
	case safety.RiskNetwork:
		return r.c(blue, "talks to the network")
	case safety.RiskDestructive:
		return r.c(red, "destructive ✗")
	case safety.RiskBlocked:
		return r.c(red+bold, "BLOCKED ✗")
	}
	return risk.String()
}

func pad(s string, w int) string {
	for len(s) < w {
		s += " "
	}
	return s
}
