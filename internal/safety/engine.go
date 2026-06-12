package safety

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Policy is the user-configurable part of the verdict: which risk
// tiers may be auto-placed, plus a hard denylist.
type Policy struct {
	Autofill []RiskLevel // tiers allowed onto the prompt
	Block    []string    // substring/prefix patterns forced to RiskBlocked
	Paranoid bool        // if true, only read-only commands autofill
}

func DefaultPolicy() Policy {
	return Policy{Autofill: []RiskLevel{RiskReadOnly, RiskModifies, RiskNetwork}}
}

// Engine re-classifies every suggested command locally and deterministically. 
// The model's own risk label is never consulted.
// (a hallucinating model must not be able to downgrade its own command)
type Engine struct {
	policy Policy
	// fileExists is injectable so redirect-clobber detection is
	// testable without touching the real filesystem.
	fileExists func(string) bool
}

func New(p Policy) *Engine {
	return &Engine{
		policy: p,
		fileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}
}

// Classify is the engine's single entry point.
func (e *Engine) Classify(command string) Verdict {
	risk, reasons := e.classify(command, 0)

	for _, pat := range e.policy.Block {
		if pat != "" && strings.Contains(command, strings.TrimSuffix(pat, "*")) {
			risk = RiskBlocked
			reasons = append(reasons, fmt.Sprintf("matches blocklist pattern %q", pat))
		}
	}

	return Verdict{Risk: risk, Reasons: reasons, Place: e.allowPlace(risk)}
}

func (e *Engine) allowPlace(r RiskLevel) bool {
	if r == RiskDestructive || r == RiskBlocked {
		return false // never, regardless of configuration (todo: rethink! maybe should be configurable)
	}
	if e.policy.Paranoid {
		return r == RiskReadOnly
	}
	for _, allowed := range e.policy.Autofill {
		if r == allowed {
			return true
		}
	}
	return false
}

// classify combines Layer A (pattern rules) and Layer B (argument inspection) and returns the highest risk.
func (e *Engine) classify(command string, depth int) (RiskLevel, []string) {
	risk, reasons := matchRules(command)

	// Command substitution hides arbitrary execution inside an
	// argument: classify the inner text too and never treat the whole as read-only.
	if inner, ok := extractSubstitution(command); ok && depth < 3 {
		risk = maxRisk(risk, RiskModifies)
		innerRisk, innerReasons := e.classify(inner, depth+1)
		risk = maxRisk(risk, innerRisk)
		reasons = append(reasons, prefixAll("in substitution: ", innerReasons)...)
	}

	for _, seg := range splitSegments(command) {
		segRisk, segReasons := e.classifySegment(seg)
		risk = maxRisk(risk, segRisk)
		reasons = append(reasons, segReasons...)
	}
	return risk, reasons
}

// classifySegment inspects one pipeline/list segment: base command, flags, targets, redirects.
func (e *Engine) classifySegment(seg string) (RiskLevel, []string) {
	words, redirects := shellWords(seg)
	words = stripWrappers(words)
	if len(words) == 0 {
		return RiskReadOnly, nil
	}

	var reasons []string
	risk := RiskReadOnly

	// sudo: never autofill elevated commands.
	if words[0] == "sudo" || words[0] == "doas" {
		reasons = append(reasons, "runs with elevated privileges")
		risk = RiskDestructive
		words = stripWrappers(words[1:])
		if len(words) == 0 {
			return risk, reasons
		}
	}

	cmdRisk, cmdReasons := e.classifyCommand(words)
	risk = maxRisk(risk, cmdRisk)
	reasons = append(reasons, cmdReasons...)

	for _, rd := range redirects {
		rdRisk, rdReason := e.classifyRedirect(rd)
		risk = maxRisk(risk, rdRisk)
		if rdReason != "" {
			reasons = append(reasons, rdReason)
		}
	}
	return risk, reasons
}

func (e *Engine) classifyCommand(words []string) (RiskLevel, []string) {
	name := baseName(words[0])
	args := words[1:]

	switch name {
	case "rm", "unlink", "shred", "srm":
		return classifyRm(name, args)
	case "find", "fd":
		return classifyFind(args)
	case "git":
		return classifyGit(args)
	case "sed":
		for _, a := range args {
			if a == "--in-place" || strings.HasPrefix(a, "-i") {
				return RiskModifies, []string{"sed -i edits files in place"}
			}
		}
		return RiskReadOnly, nil
	case "sh", "bash", "zsh", "dash", "ksh":
		if len(nonFlagArgs(args)) == 0 {
			// a bare shell in a pipeline executes whatever streams in
			return RiskDestructive, []string{"executes piped/interactive input as code"}
		}
		return RiskModifies, []string{"runs a script"}
	case "xargs":
		// classify whatever xargs will run
		if rest := dropXargsFlags(args); len(rest) > 0 {
			return e.classifyCommand(rest)
		}
		return RiskModifies, nil
	case "ipconfig":
		// macOS network tool: get* subcommands only query state
		if len(args) == 0 || strings.HasPrefix(args[0], "get") ||
			args[0] == "waitall" || args[0] == "ifcount" {
			return RiskReadOnly, nil
		}
		return RiskModifies, []string{"ipconfig can reconfigure network interfaces"}
	case "ifconfig":
		// listing all interfaces or inspecting one is read-only;
		// anything beyond the interface name changes its config
		if len(nonFlagArgs(args)) <= 1 {
			return RiskReadOnly, nil
		}
		return RiskModifies, []string{"ifconfig with extra arguments changes interface config"}
	}

	if commandTable[name] != 0 || isKnownReadOnly(name) {
		r := commandTable[name]
		var why []string
		if r == RiskDestructive {
			why = []string{name + " is destructive"}
		}
		return r, why
	}
	// Unknown command: it can change things, so it must not be treated
	// as read-only but plain Modifies keeps it fillable for the long tail of harmless tools.
	return RiskModifies, nil
}

func classifyRm(name string, args []string) (RiskLevel, []string) {
	reasons := []string{name + " deletes data"}
	for _, t := range nonFlagArgs(args) {
		if isProtectedPath(t) {
			return RiskBlocked, append(reasons, fmt.Sprintf("delete aimed at protected path %q", t))
		}
	}
	return RiskDestructive, reasons
}

func classifyFind(args []string) (RiskLevel, []string) {
	for i, a := range args {
		switch a {
		case "-delete":
			return RiskDestructive, []string{"find -delete removes matched files"}
		case "-exec", "-execdir", "-ok", "-okdir":
			if i+1 < len(args) && (args[i+1] == "rm" || args[i+1] == "shred") {
				return RiskDestructive, []string{"find -exec deletes matched files"}
			}
			return RiskModifies, []string{"find -exec runs a command per match"}
		}
	}
	return RiskReadOnly, nil
}

func classifyGit(args []string) (RiskLevel, []string) {
	sub := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			sub = a
			break
		}
	}
	switch sub {
	case "status", "log", "diff", "show", "blame", "remote":
		return RiskReadOnly, nil
	case "branch":
		if hasFlag(args, "-D", "-d", "--delete") {
			return RiskModifies, nil
		}
		return RiskReadOnly, nil
	case "push", "pull", "fetch", "clone":
		return RiskNetwork, []string{"talks to a git remote"}
	case "clean":
		return RiskDestructive, []string{"git clean deletes untracked files"}
	case "reset":
		if hasFlag(args, "--hard") {
			return RiskDestructive, []string{"git reset --hard discards working-tree changes"}
		}
		return RiskModifies, nil
	default:
		return RiskModifies, nil
	}
}

type redirect struct {
	op     string // ">" or ">>"
	target string
}

func (e *Engine) classifyRedirect(rd redirect) (RiskLevel, string) {
	t := rd.target
	if t == "/dev/null" || t == "/dev/stderr" || t == "/dev/stdout" || t == "" ||
		strings.HasPrefix(t, "&") { // fd duplication like 2>&1
		return RiskReadOnly, ""
	}
	if rd.op == ">>" {
		return RiskModifies, ""
	}
	if e.fileExists(expandHome(t)) {
		return RiskDestructive, fmt.Sprintf("> overwrites existing file %q", t)
	}
	return RiskModifies, ""
}

// ---- command knowledge ----

var readOnlyCommands = map[string]bool{
	"ls": true, "cat": true, "grep": true, "egrep": true, "fgrep": true,
	"rg": true, "ag": true, "head": true, "tail": true, "less": true,
	"more": true, "wc": true, "file": true, "stat": true, "du": true,
	"df": true, "pwd": true, "echo": true, "printf": true, "which": true,
	"whereis": true, "type": true, "man": true, "ps": true, "top": true,
	"env": true, "printenv": true, "date": true, "cal": true, "uname": true,
	"id": true, "whoami": true, "history": true, "tree": true,
	"basename": true, "dirname": true, "readlink": true, "md5": true,
	"shasum": true, "sha256sum": true, "diff": true, "cmp": true,
	"sort": true, "uniq": true, "cut": true, "tr": true, "awk": true,
	"jq": true, "xxd": true, "od": true, "strings": true, "uptime": true,
	"lsof": true, "netstat": true,
}

var commandTable = map[string]RiskLevel{
	// modifies
	"mv": RiskModifies, "cp": RiskModifies, "touch": RiskModifies,
	"mkdir": RiskModifies, "rmdir": RiskModifies, "chmod": RiskModifies,
	"chown": RiskModifies, "ln": RiskModifies, "tar": RiskModifies,
	"zip": RiskModifies, "unzip": RiskModifies, "gzip": RiskModifies,
	"gunzip": RiskModifies, "tee": RiskModifies, "kill": RiskModifies,
	"killall": RiskModifies, "pkill": RiskModifies, "make": RiskModifies,
	"patch": RiskModifies, "defaults": RiskModifies,

	// network
	"curl": RiskNetwork, "wget": RiskNetwork, "ssh": RiskNetwork,
	"scp": RiskNetwork, "sftp": RiskNetwork, "rsync": RiskNetwork,
	"nc": RiskNetwork, "ncat": RiskNetwork, "netcat": RiskNetwork,
	"telnet": RiskNetwork, "ping": RiskNetwork, "traceroute": RiskNetwork,
	"dig": RiskNetwork, "nslookup": RiskNetwork, "host": RiskNetwork,
	"brew": RiskNetwork, "apt": RiskNetwork, "apt-get": RiskNetwork,
	"yum": RiskNetwork, "dnf": RiskNetwork, "pacman": RiskNetwork,
	"pip": RiskNetwork, "pip3": RiskNetwork, "npm": RiskNetwork,
	"yarn": RiskNetwork, "pnpm": RiskNetwork, "gem": RiskNetwork,
	"cargo": RiskNetwork, "docker": RiskNetwork, "go": RiskModifies,

	// destructive
	"dd": RiskDestructive, "mkfs": RiskDestructive, "fdisk": RiskDestructive,
	"diskutil": RiskDestructive, "truncate": RiskDestructive,
	"shutdown": RiskDestructive, "reboot": RiskDestructive,
	"halt": RiskDestructive, "poweroff": RiskDestructive,
	"launchctl": RiskDestructive, "format": RiskDestructive,
}

func isKnownReadOnly(name string) bool { return readOnlyCommands[name] }

var protectedPaths = map[string]bool{
	"/": true, "/*": true, "~": true, "~/": true, "~/*": true,
	"$HOME": true, "${HOME}": true, "$HOME/": true, "$HOME/*": true,
	"/Users": true, "/Users/*": true, "/home": true, "/home/*": true,
	"/etc": true, "/usr": true, "/var": true, "/bin": true, "/sbin": true,
	"/lib": true, "/boot": true, "/System": true, "/Library": true,
	"/Applications": true, "..": true, "../": true,
}

func isProtectedPath(p string) bool {
	return protectedPaths[strings.TrimSpace(p)]
}

func expandHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if p == "~" {
			return home
		}
		if strings.HasPrefix(p, "~/") {
			return home + p[1:]
		}
	}
	return p
}

// ---- light shell parsing ----

// splitSegments splits a command line on unquoted ; | & && || into the
// sub-commands a shell would actually run.
func splitSegments(s string) []string {
	var segs []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			cur.WriteByte(c)
		case c == '"' && !inSingle:
			inDouble = !inDouble
			cur.WriteByte(c)
		case c == '&' && i > 0 && s[i-1] == '>' && !inSingle && !inDouble:
			cur.WriteByte(c) // fd duplication (2>&1), not a list separator
		case (c == ';' || c == '|' || c == '&') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				segs = append(segs, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			// skip the second char of && and ||
			if i+1 < len(s) && (s[i+1] == '&' || s[i+1] == '|') {
				i++
			}
		default:
			cur.WriteByte(c)
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		segs = append(segs, strings.TrimSpace(cur.String()))
	}
	return segs
}

// shellWords splits one segment into words (honoring quotes and backslash escapes) and extracts output redirects.
func shellWords(s string) ([]string, []redirect) {
	var words []string
	var redirects []redirect
	var cur strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && !inSingle && i+1 < len(s):
			cur.WriteByte(s[i+1])
			i++
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == ' ' || c == '\t':
			if inSingle || inDouble {
				cur.WriteByte(c)
			} else {
				flush()
			}
		case c == '>' && !inSingle && !inDouble:
			// "2>", "&>" prefixes ride along; treat them the same
			flush()
			op := ">"
			if i+1 < len(s) && s[i+1] == '>' {
				op = ">>"
				i++
			}
			words = append(words, op)
		default:
			cur.WriteByte(c)
		}
	}
	flush()

	// pull "op target" pairs out of the word list
	var clean []string
	for i := 0; i < len(words); i++ {
		if words[i] == ">" || words[i] == ">>" {
			rd := redirect{op: words[i]}
			if i+1 < len(words) {
				rd.target = words[i+1]
				i++
			}
			redirects = append(redirects, rd)
			continue
		}
		clean = append(clean, words[i])
	}
	return clean, redirects
}

// stripWrappers drops env assignments and no-op prefix commands so the real command is inspected.
func stripWrappers(words []string) []string {
	envAssign := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	for len(words) > 0 {
		w := words[0]
		if envAssign.MatchString(w) {
			words = words[1:]
			continue
		}
		switch w {
		case "env", "nohup", "time", "nice", "command", "exec", "noglob":
			words = words[1:]
			continue
		}
		break
	}
	return words
}

func dropXargsFlags(args []string) []string {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return args[i:]
		}
		// short flags that consume the next word as a value
		if a == "-n" || a == "-P" || a == "-I" || a == "-L" || a == "-s" {
			i += 2
			continue
		}
		i++
	}
	return nil
}

// extractSubstitution returns the contents of the first $(...) or backtick span, if any.
func extractSubstitution(s string) (string, bool) {
	if i := strings.Index(s, "$("); i >= 0 {
		depth := 0
		for j := i + 1; j < len(s); j++ {
			switch s[j] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return s[i+2 : j], true
				}
			}
		}
	}
	if i := strings.Index(s, "`"); i >= 0 {
		if j := strings.Index(s[i+1:], "`"); j >= 0 {
			return s[i+1 : i+1+j], true
		}
	}
	return "", false
}

// ---- small helpers ----

func baseName(cmd string) string {
	if i := strings.LastIndex(cmd, "/"); i >= 0 {
		return cmd[i+1:]
	}
	return cmd
}

func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n || strings.HasPrefix(a, n+"=") {
				return true
			}
		}
	}
	return false
}

// hasCombinedFlag reports whether a short-flag cluster contains the letter, e.g. 'r' in "-rf", "-fr", "-vrf".
func hasCombinedFlag(args []string, letter byte) bool {
	for _, a := range args {
		if len(a) >= 2 && a[0] == '-' && a[1] != '-' {
			if strings.IndexByte(a[1:], letter) >= 0 {
				return true
			}
		}
	}
	return false
}

func nonFlagArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

func maxRisk(a, b RiskLevel) RiskLevel {
	if a > b {
		return a
	}
	return b
}

func prefixAll(prefix string, ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = prefix + s
	}
	return out
}
