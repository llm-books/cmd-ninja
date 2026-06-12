// Package wire defines the one-line machine-readable protocol between
// the binary and the shell widgets. The human INFO/RISK block goes to
// stderr (the tty); this line is the only thing on stdout, so the
// widget's command substitution captures exactly one parseable result.
package wire

import (
	"fmt"
	"strings"
)

type Verb string

const (
	VerbFill  Verb = "FILL"  // place the command on the prompt line
	VerbShow  Verb = "SHOW"  // display only; user must copy or retype
	VerbBlock Verb = "BLOCK" // refused; nothing should be placed
)

// Encode renders "VERB\t<command>". Newlines inside the command would
// break the single-line protocol, so they are flattened to spaces —
// a multi-line command should never be FILLed verbatim anyway.
func Encode(v Verb, command string) string {
	flat := strings.ReplaceAll(command, "\n", " ")
	flat = strings.ReplaceAll(flat, "\r", " ")
	return fmt.Sprintf("%s\t%s", v, flat)
}
