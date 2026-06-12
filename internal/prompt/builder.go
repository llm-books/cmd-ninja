// Package prompt builds the system and user prompts. The model is asked for honest risk labels
// but they are advisory only the safety engine has the final say.
package prompt

import (
	"fmt"
	"strings"

	"github.com/llm-books/cmd-ninja/internal/contextcol"
	"github.com/llm-books/cmd-ninja/internal/llm"
)

const systemTemplate = `You translate a natural-language request into a single shell command for the user's environment.

Output ONLY a JSON object, no prose, no markdown fences:
{"command": "...", "explanation": [{"token": "...", "note": "..."}], "risk": "read_only|modifies|network|destructive"}

Rules:
- Target %s on %s. Use flags valid for that platform (BSD userland on macOS, GNU on Linux).
- Prefer the safest command that accomplishes the goal.
- Break "explanation" into the command's parts, in order; one short note per token.
- "risk" must be your honest label: read_only (only reads), modifies (creates/changes files or processes), network (talks to other machines), destructive (deletes or overwrites data).
- If the request is ambiguous or dangerous, return your safest reasonable interpretation and label risk honestly.
- Never invent flags. If the request cannot be one command, pick the most useful single step.`

// fewShot examples span the risk tiers to lock the JSON shape and the honesty of the labels.
var fewShot = []struct{ user, assistant string }{
	{
		"show hidden files here",
		`{"command": "ls -la", "explanation": [{"token": "ls", "note": "list files"}, {"token": "-l", "note": "long format with details"}, {"token": "-a", "note": "include hidden dotfiles"}], "risk": "read_only"}`,
	},
	{
		"make a folder called drafts with a notes subfolder",
		`{"command": "mkdir -p drafts/notes", "explanation": [{"token": "mkdir", "note": "create directories"}, {"token": "-p", "note": "create parents as needed, no error if present"}, {"token": "drafts/notes", "note": "the nested path to create"}], "risk": "modifies"}`,
	},
	{
		"download the latest release tarball from example.com/app.tgz",
		`{"command": "curl -fLO https://example.com/app.tgz", "explanation": [{"token": "curl", "note": "transfer data from a URL"}, {"token": "-f", "note": "fail on HTTP errors instead of saving an error page"}, {"token": "-L", "note": "follow redirects"}, {"token": "-O", "note": "save with the remote filename"}, {"token": "https://example.com/app.tgz", "note": "the file to fetch"}], "risk": "network"}`,
	},
	{
		"wipe the build directory",
		`{"command": "rm -rf build", "explanation": [{"token": "rm", "note": "remove"}, {"token": "-r", "note": "recurse into subfolders"}, {"token": "-f", "note": "force, no prompts"}, {"token": "build", "note": "the directory to delete"}], "risk": "destructive"}`,
	},
}

// Build assembles the request for a provider. Few-shot pairs are inlined into the user 
// prompt so the provider interface stays a simple system+user pair.
func Build(query string, env contextcol.Context) llm.Request {
	system := fmt.Sprintf(systemTemplate, env.Shell, env.OS)

	var b strings.Builder
	b.WriteString("Examples:\n")
	for _, ex := range fewShot {
		fmt.Fprintf(&b, "Request: %s\nAnswer: %s\n\n", ex.user, ex.assistant)
	}
	fmt.Fprintf(&b, "Environment: shell=%s os=%s cwd=%s\n", env.Shell, env.OS, env.Cwd)
	if len(env.Dir) > 0 {
		fmt.Fprintf(&b, "Current directory contains: %s\n", strings.Join(env.Dir, ", "))
	}
	fmt.Fprintf(&b, "\nRequest: %s\nAnswer:", query)

	return llm.Request{System: system, User: b.String(), Query: query}
}
