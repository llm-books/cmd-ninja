package safety

// RiskLevel classifies what a command can do to the user's system.
// Ordering matters higher values are more dangerous, and the engine
// always escalates to the highest level any rule produces.
type RiskLevel int

const (
	RiskReadOnly    RiskLevel = iota // ls, find, cat, grep
	RiskModifies                     // mv, touch, mkdir, chmod
	RiskNetwork                      // curl, ssh, scp, git push
	RiskDestructive                  // rm -rf, dd, mkfs, redirect-clobber
	RiskBlocked                      // never placed or run; hard stop
)

var riskNames = map[RiskLevel]string{
	RiskReadOnly:    "read_only",
	RiskModifies:    "modifies",
	RiskNetwork:     "network",
	RiskDestructive: "destructive",
	RiskBlocked:     "blocked",
}

func (r RiskLevel) String() string {
	if n, ok := riskNames[r]; ok {
		return n
	}
	return "unknown"
}

// ParseRisk maps a wire/JSON label to a RiskLevel. Unknown labels map to
// RiskDestructive a label we cannot interpret must not earn autofill.
func ParseRisk(s string) RiskLevel {
	for level, name := range riskNames {
		if name == s {
			return level
		}
	}
	return RiskDestructive
}

// AutoFill reports whether commands at this risk level may be placed on
// the user's prompt line by default. Network commands are placed (the
// user still has to press Enter, and the INFO block surfaces the host)
// destructive and blocked commands never are.
func (r RiskLevel) AutoFill() bool {
	return r == RiskReadOnly || r == RiskModifies || r == RiskNetwork
}

// Verdict is the safety engine's final word on a command.
type Verdict struct {
	Risk    RiskLevel
	Reasons []string // human-readable rule hits, e.g. "recursive force delete"
	Place   bool     // whether the command may be auto-placed on the prompt
}
