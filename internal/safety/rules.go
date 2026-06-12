package safety

import "regexp"

// Rule is a Layer-A pattern: a fast regex match on a known-dangerous
// command shape. Layer B (argument inspection in engine.go) catches
// what these miss and trims their false positives.
type Rule struct {
	Re   *regexp.Regexp
	Risk RiskLevel
	Why  string
}

var patternRules = []Rule{
	// Hard stops: shapes with no legitimate interactive use.
	{regexp.MustCompile(`:\(\)\s*\{.*\}\s*;\s*:`), RiskBlocked, "fork bomb"},
	{regexp.MustCompile(`\brm\b.*--no-preserve-root`), RiskBlocked, "explicitly disables the root-deletion safety"},
	{regexp.MustCompile(`\bdd\b[^|;&]*\bof=/dev/(disk|rdisk|sd|hd|nvme)`), RiskBlocked, "raw write to a disk device"},
	{regexp.MustCompile(`>\s*/dev/(disk|rdisk|sd|hd|nvme)`), RiskBlocked, "overwrites a disk device"},

	// Destructive shapes.
	{regexp.MustCompile(`\brm\b\s+(-[a-zA-Z]*[rR][a-zA-Z]*f[a-zA-Z]*|-[a-zA-Z]*f[a-zA-Z]*[rR][a-zA-Z]*)\b`), RiskDestructive, "recursive force delete"},
	{regexp.MustCompile(`\bdd\b[^|;&]*\bof=`), RiskDestructive, "dd overwrites its output target"},
	{regexp.MustCompile(`\bmkfs(\.\w+)?\b`), RiskDestructive, "formats a filesystem"},
	{regexp.MustCompile(`\bchmod\b[^|;&]*-[a-zA-Z]*R[a-zA-Z]*[^|;&]*\b777\b`), RiskDestructive, "recursive world-writable permissions"},
	{regexp.MustCompile(`\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(ba|z|da|k)?sh\b`), RiskDestructive, "pipes downloaded content into a shell"},
	{regexp.MustCompile(`\bcrontab\b[^|;&]*\s-[a-zA-Z]*r`), RiskDestructive, "wipes the crontab"},
	{regexp.MustCompile(`\btruncate\b`), RiskDestructive, "truncates a file in place"},
	{regexp.MustCompile(`\b(shutdown|reboot|halt|poweroff)\b`), RiskDestructive, "power-state change"},
	{regexp.MustCompile(`\bgit\b[^|;&]*\bpush\b[^|;&]*(--force\b|-f\b|--force-with-lease)`), RiskDestructive, "force-push rewrites remote history"},
}

// matchRules runs every Layer-A rule and returns the highest risk plus
// all the reasons that fired.
func matchRules(command string) (RiskLevel, []string) {
	risk := RiskReadOnly
	var reasons []string
	for _, r := range patternRules {
		if r.Re.MatchString(command) {
			reasons = append(reasons, r.Why)
			if r.Risk > risk {
				risk = r.Risk
			}
		}
	}
	return risk, reasons
}
