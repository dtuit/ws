package command

// WorktreesOverride is a tri-state override for worktree expansion.
// If Set is false, callers should use their configured default.
type WorktreesOverride struct {
	Set   bool
	Value bool
}

// Resolve applies the override to a default value.
func (o WorktreesOverride) Resolve(defaultValue bool) bool {
	if o.Set {
		return o.Value
	}
	return defaultValue
}

// ParseWorktreesFlag parses a single worktree-related CLI flag.
func ParseWorktreesFlag(token string) (WorktreesOverride, bool) {
	switch token {
	case "-t", "--worktrees":
		return WorktreesOverride{Set: true, Value: true}, true
	case "--no-worktrees":
		return WorktreesOverride{Set: true, Value: false}, true
	default:
		return WorktreesOverride{}, false
	}
}

// StripWorktreesFlags removes any worktree-related flags and returns the last override seen.
func StripWorktreesFlags(args []string) ([]string, WorktreesOverride) {
	if len(args) == 0 {
		return args, WorktreesOverride{}
	}

	rest := make([]string, 0, len(args))
	var override WorktreesOverride
	for _, arg := range args {
		if parsed, ok := ParseWorktreesFlag(arg); ok {
			override = parsed
			continue
		}
		rest = append(rest, arg)
	}
	return rest, override
}

func isWorktreesFlag(token string) bool {
	_, ok := ParseWorktreesFlag(token)
	return ok
}

func worktreesFlagSuggestions() []string {
	return []string{"-t", "--worktrees", "--no-worktrees"}
}
