package command

const LLBranchesFlagUsage = "--branches|-b"

// StripLLBranchesFlags removes any ll branch-mode flags and returns whether branch mode was requested.
func StripLLBranchesFlags(args []string) ([]string, bool) {
	if len(args) == 0 {
		return args, false
	}

	rest := make([]string, 0, len(args))
	showBranches := false
	for _, arg := range args {
		switch arg {
		case "-b", "--branches":
			showBranches = true
		default:
			rest = append(rest, arg)
		}
	}
	return rest, showBranches
}

func llFlagSuggestions() []string {
	values := append([]string{}, worktreesFlagSuggestions()...)
	values = append(values, "-b", "--branches")
	return values
}
