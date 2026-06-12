// Package teach turns a suggestion's token explanations into the rows
// of the INFO block. The goal is a readable map of the command, not
// a man page.
package teach

import (
	"strings"

	"github.com/llm-books/cmd-ninja/internal/llm"
)

type Mode string

const (
	ModeOff     Mode = "off"     // no INFO block
	ModeCompact Mode = "compact" // flags and non-obvious args only
	ModeFull    Mode = "full"    // annotate every token
)

func ParseMode(s string) Mode {
	switch s {
	case "off":
		return ModeOff
	case "full":
		return ModeFull
	default:
		return ModeCompact
	}
}

// Rows filters the explanation tokens for the chosen mode. The command
// itself stays in full mode so beginners see what the tool is; compact
// drops it (you can read the first word yourself) along with empty notes.
func Rows(sug llm.Suggestion, mode Mode) []llm.Token {
	if mode == ModeOff {
		return nil
	}
	baseCmd := firstWord(sug.Command)
	var rows []llm.Token
	for _, t := range sug.Explanation {
		if strings.TrimSpace(t.Note) == "" || strings.TrimSpace(t.Text) == "" {
			continue
		}
		if mode == ModeCompact && t.Text == baseCmd {
			continue
		}
		rows = append(rows, llm.Token{
			Text: t.Text,
			Note: firstLine(t.Note),
		})
	}
	return rows
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// firstLine keeps each note to one short line, whatever the model sent.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const max = 80
	if len(s) > max {
		s = s[:max-3] + "..."
	}
	return s
}
