// Package contextcol gathers the environment facts injected into the
// prompt and shown next to the risk badge: OS, shell, cwd, and an
// optional one-level directory snapshot so the model can reference real filenames.
package contextcol

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Context struct {
	OS    string // "macOS", "Linux", "Windows"
	Shell string // "zsh", "bash", "fish"
	Cwd   string
	Dir   []string // names in cwd, capped (empty if snapshot disabled)
}

const snapshotCap = 30

func Collect(withSnapshot bool) Context {
	c := Context{
		OS:    osName(),
		Shell: shellName(),
	}
	if cwd, err := os.Getwd(); err == nil {
		c.Cwd = cwd
	}
	if withSnapshot && c.Cwd != "" {
		c.Dir = snapshot(c.Cwd)
	}
	return c
}

// EnvNote is the short trailer on the RISK line: "Shell: zsh (macOS)".
func (c Context) EnvNote() string {
	return fmt.Sprintf("Shell: %s (%s)", c.Shell, c.OS)
}

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	}
	return runtime.GOOS
}

func shellName() string {
	if s := os.Getenv("NINJA_SHELL"); s != "" {
		return s // widgets can pin this so a bash widget never gets zsh syntax
	}
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "sh"
}

func snapshot(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > snapshotCap {
		names = names[:snapshotCap]
	}
	return names
}
