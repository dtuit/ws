package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dtuit/ws/internal/command"
	"github.com/dtuit/ws/internal/manifest"
)

// stripRepeatedValueFlag extracts every occurrence of `<name> <value>` and
// `<name>=<value>` from args, returning the remaining args and the collected
// values in order. Unknown trailing `<name>` with no value is treated as a
// fatal-in-caller error (we keep parsing lenient here and let the caller
// surface the usage string).
func stripRepeatedValueFlag(args []string, name string) ([]string, []string) {
	var values []string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == name {
			if i+1 < len(args) {
				values = append(values, args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, name+"=") {
			values = append(values, strings.TrimPrefix(arg, name+"="))
			continue
		}
		rest = append(rest, arg)
	}
	return rest, values
}

func stripBoolFlag(args []string, names ...string) ([]string, bool) {
	if len(args) == 0 {
		return args, false
	}
	nameSet := make(map[string]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}

	rest := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if nameSet[arg] {
			found = true
			continue
		}
		rest = append(rest, arg)
	}
	return rest, found
}

func parseOptionalFilterArg(args []string, defaultFilter string, hasDefaultFilter bool, usage string) (string, error) {
	switch len(args) {
	case 0:
		if hasDefaultFilter {
			return defaultFilter, nil
		}
		return "", nil
	case 1:
		return args[0], nil
	default:
		return "", fmt.Errorf("usage: %s", usage)
	}
}

func parseCDArgs(args []string) (name, selector string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--worktree", "-t":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("cd requires a selector after %s", args[i])
			}
			selector = args[i+1]
			i++
		default:
			if name != "" {
				return "", "", fmt.Errorf("cd takes one repo name")
			}
			name = args[i]
		}
	}
	if name == "" {
		return "", "", fmt.Errorf("cd requires a repo name")
	}
	return name, selector, nil
}

func resolveCDTarget(name, selector string, active map[string]manifest.RepoConfig) (string, string, error) {
	if _, ok := active[name]; ok {
		return name, selector, nil
	}

	repoName, inlineSelector, ok := splitCDInlineTarget(name, active)
	if !ok {
		return name, selector, nil
	}
	if selector != "" {
		return "", "", fmt.Errorf("cd does not allow both repo@worktree and %s <selector>", command.CDWorktreeFlagUsage)
	}
	if inlineSelector == "" {
		return "", "", fmt.Errorf("cd requires a worktree name after @")
	}
	return repoName, inlineSelector, nil
}

func splitCDInlineTarget(target string, active map[string]manifest.RepoConfig) (string, string, bool) {
	if !strings.Contains(target, "@") {
		return "", "", false
	}

	bestRepo := ""
	for repoName := range active {
		if !strings.HasPrefix(target, repoName+"@") {
			continue
		}
		if len(repoName) > len(bestRepo) {
			bestRepo = repoName
		}
	}
	if bestRepo == "" {
		return "", "", false
	}

	return bestRepo, strings.TrimPrefix(target, bestRepo+"@"), true
}

type contextArgs struct {
	Action            string
	Filter            string
	Group             string
	Local             bool
	WorktreesOverride command.WorktreesOverride
}

type shellArgs struct {
	Action string
}

type workspaceArgs struct {
	Action string
	Name   string
}

func parseShellArgs(args []string) (shellArgs, error) {
	if len(args) != 1 {
		return shellArgs{}, fmt.Errorf("usage: ws shell <init|install>")
	}

	switch args[0] {
	case "init", "install":
		return shellArgs{Action: args[0]}, nil
	default:
		return shellArgs{}, fmt.Errorf("usage: ws shell <init|install>")
	}
}

func parseContextArgs(args []string) (contextArgs, error) {
	var parsed contextArgs
	filtered, worktreesOverride := command.StripWorktreesFlags(args)
	parsed.WorktreesOverride = worktreesOverride

	var tokens []string
	for _, arg := range filtered {
		if arg == "--local" {
			if parsed.Local {
				return contextArgs{}, fmt.Errorf("--local may only be provided once")
			}
			parsed.Local = true
			continue
		}
		if arg != "-" && strings.HasPrefix(arg, "-") {
			return contextArgs{}, fmt.Errorf("unknown context flag: %s", arg)
		}
		tokens = append(tokens, arg)
	}

	if len(tokens) == 0 {
		if parsed.Local {
			return contextArgs{}, fmt.Errorf("--local is only valid with ws context save")
		}
		parsed.Action = "show"
		return parsed, nil
	}

	action := "set"
	switch tokens[0] {
	case "set", "add", "remove", "save", "refresh":
		action = tokens[0]
		tokens = tokens[1:]
	case ".":
		action = "refresh"
		tokens = tokens[1:]
	case "-", "prev", "previous":
		if len(tokens) != 1 {
			return contextArgs{}, fmt.Errorf("%q cannot be combined with other tokens", tokens[0])
		}
		if parsed.Local {
			return contextArgs{}, fmt.Errorf("--local is only valid with ws context save")
		}
		parsed.Action = "swap"
		return parsed, nil
	}
	parsed.Action = action

	if action == "save" {
		if parsed.WorktreesOverride.Set {
			return contextArgs{}, fmt.Errorf("ws context save does not accept %s", command.WorktreesFlagUsage)
		}
		if len(tokens) != 1 {
			return contextArgs{}, fmt.Errorf("usage: ws context save [--local] <group>")
		}
		parsed.Group = strings.TrimSpace(tokens[0])
		return parsed, nil
	}

	if parsed.Local {
		return contextArgs{}, fmt.Errorf("--local is only valid with ws context save")
	}
	if action == "refresh" {
		if len(tokens) != 0 {
			return contextArgs{}, fmt.Errorf("usage: ws context refresh [%s]", command.WorktreesFlagUsage)
		}
		return parsed, nil
	}
	if len(tokens) == 0 {
		return contextArgs{}, fmt.Errorf("usage: ws context %s [%s] <filter>", action, command.WorktreesFlagUsage)
	}

	parsed.Filter = strings.Join(tokens, ",")
	return parsed, nil
}

func parseWorkspaceArgs(args []string) (workspaceArgs, error) {
	if len(args) == 0 {
		return workspaceArgs{Action: "show"}, nil
	}

	switch args[0] {
	case "list", "ls":
		if len(args) != 1 {
			return workspaceArgs{}, fmt.Errorf("usage: ws workspace list")
		}
		return workspaceArgs{Action: "list"}, nil
	case "use":
		if len(args) != 2 {
			return workspaceArgs{}, fmt.Errorf("usage: ws workspace use <name>")
		}
		return workspaceArgs{Action: "use", Name: strings.TrimSpace(args[1])}, nil
	case "clear", "unset":
		if len(args) != 1 {
			return workspaceArgs{}, fmt.Errorf("usage: ws workspace clear")
		}
		return workspaceArgs{Action: "clear"}, nil
	default:
		return workspaceArgs{}, fmt.Errorf("usage: ws workspace [list|use <name>|clear]")
	}
}

func parseEditorFlag(args []string) (string, []string) {
	var editor string
	var rest []string
	for i := 0; i < len(args); i++ {
		if (args[i] == "--editor" || args[i] == "-e") && i+1 < len(args) {
			editor = args[i+1]
			i++
		} else {
			rest = append(rest, args[i])
		}
	}
	return editor, rest
}

type agentArgs struct {
	Action      string   // "start", "ls", "resume"
	Repo        string   // target repo (for start)
	Agent       string   // agent profile name (--agent)
	IndexOrID   string   // for resume: numeric index or session ID prefix
	Filter      string   // for ls: filter expression
	Limit       int      // for ls: max sessions (0 = default)
	ShowAll     bool     // for ls: show all sessions
	Verbose     bool     // for ls: show full prompt text
	ShowLast    bool     // for ls: compact view shows last user prompt
	ShowRecap   bool     // for ls: compact view shows recap (fallback last/first)
	Passthrough []string // args after -- to pass to the agent CLI
}

func parseAgentArgs(args []string) (agentArgs, error) {
	// Split on "--" to separate ws args from agent passthrough
	var wsArgs, passthrough []string
	for i, arg := range args {
		if arg == "--" {
			wsArgs = args[:i]
			passthrough = args[i+1:]
			break
		}
	}
	if passthrough == nil {
		wsArgs = args
	}

	if len(wsArgs) == 0 {
		return agentArgs{Action: "start", Passthrough: passthrough}, nil
	}

	switch wsArgs[0] {
	case "ls", "list":
		return parseAgentLSArgs(wsArgs[1:])
	case "resume":
		if len(wsArgs) != 2 {
			return agentArgs{}, fmt.Errorf("usage: ws agent resume <#|session-id>")
		}
		return agentArgs{Action: "resume", IndexOrID: wsArgs[1]}, nil
	case "pin", "unpin":
		if len(wsArgs) > 2 {
			return agentArgs{}, fmt.Errorf("usage: ws agent %s [<#|session-id>]", wsArgs[0])
		}
		a := agentArgs{Action: wsArgs[0]}
		if len(wsArgs) == 2 {
			a.IndexOrID = wsArgs[1]
		}
		return a, nil
	default:
		return parseAgentStartArgs(wsArgs, passthrough)
	}
}

func parseAgentStartArgs(args, passthrough []string) (agentArgs, error) {
	parsed := agentArgs{Action: "start", Passthrough: passthrough}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent", "-a":
			if i+1 >= len(args) {
				return agentArgs{}, fmt.Errorf("--agent requires a name")
			}
			parsed.Agent = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return agentArgs{}, fmt.Errorf("unknown flag: %s", args[i])
			}
			if parsed.Repo != "" {
				return agentArgs{}, fmt.Errorf("usage: ws agent [--agent name] [repo] [-- args...]")
			}
			parsed.Repo = args[i]
		}
	}
	return parsed, nil
}

func parseAgentLSArgs(args []string) (agentArgs, error) {
	parsed := agentArgs{Action: "ls"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			parsed.ShowAll = true
		case "-v", "--verbose":
			parsed.Verbose = true
		case "-l", "--last":
			parsed.ShowLast = true
		case "-r", "--recap":
			parsed.ShowRecap = true
		case "-n":
			if i+1 >= len(args) {
				return agentArgs{}, fmt.Errorf("-n requires a number")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				return agentArgs{}, fmt.Errorf("-n requires a positive number")
			}
			parsed.Limit = n
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return agentArgs{}, fmt.Errorf("unknown flag: %s", args[i])
			}
			if parsed.Filter != "" {
				return agentArgs{}, fmt.Errorf("usage: ws agent ls [-v] [-l|-r] [-n N | --all] [filter]")
			}
			parsed.Filter = args[i]
		}
	}
	if parsed.ShowLast && parsed.ShowRecap {
		return agentArgs{}, fmt.Errorf("--last and --recap are mutually exclusive")
	}
	return parsed, nil
}

type muxArgs struct {
	Action      string // "attach", "kill", "ls", "save", "dup"
	SessionName string // named session config (empty = default)
	WindowName  string // for "dup": window to duplicate (empty = active window)
	Local       bool   // save to manifest.local.yml instead of manifest.yml
}

func parseMuxArgs(args []string) (muxArgs, error) {
	if len(args) == 0 {
		return muxArgs{Action: "attach"}, nil
	}
	switch args[0] {
	case "kill":
		if len(args) > 2 {
			return muxArgs{}, fmt.Errorf("usage: ws mux kill [session]")
		}
		parsed := muxArgs{Action: "kill"}
		if len(args) > 1 {
			parsed.SessionName = args[1]
		}
		return parsed, nil
	case "ls", "list":
		return muxArgs{Action: "ls"}, nil
	case "dup", "duplicate":
		if len(args) > 2 {
			return muxArgs{}, fmt.Errorf("usage: ws mux dup [window]")
		}
		parsed := muxArgs{Action: "dup"}
		if len(args) > 1 {
			parsed.WindowName = args[1]
		}
		return parsed, nil
	case "save":
		parsed := muxArgs{Action: "save"}
		for _, arg := range args[1:] {
			if arg == "--local" {
				if parsed.Local {
					return muxArgs{}, fmt.Errorf("--local may only be provided once")
				}
				parsed.Local = true
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return muxArgs{}, fmt.Errorf("unknown flag: %s", arg)
			}
			if parsed.SessionName != "" {
				return muxArgs{}, fmt.Errorf("usage: ws mux save [--local] [session]")
			}
			parsed.SessionName = arg
		}
		return parsed, nil
	default:
		// Not a subcommand — treat as a session name
		parsed := muxArgs{Action: "attach", SessionName: args[0]}
		if len(args) > 1 {
			return muxArgs{}, fmt.Errorf("usage: ws mux [session]")
		}
		return parsed, nil
	}
}

type worktreeArgs struct {
	Action string // "add", "remove", "list"
	Branch string // branch name (for add/remove)
	Filter string // optional filter
}

func parseWorktreeArgs(args []string) (worktreeArgs, error) {
	if len(args) == 0 {
		return worktreeArgs{Action: "list"}, nil
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			return worktreeArgs{}, fmt.Errorf("usage: ws worktree add <branch> [filter]")
		}
		parsed := worktreeArgs{Action: "add", Branch: args[1]}
		if len(args) > 2 {
			parsed.Filter = strings.Join(args[2:], ",")
		}
		return parsed, nil
	case "remove", "rm":
		if len(args) < 2 {
			return worktreeArgs{}, fmt.Errorf("usage: ws worktree remove <branch> [filter]")
		}
		parsed := worktreeArgs{Action: "remove", Branch: args[1]}
		if len(args) > 2 {
			parsed.Filter = strings.Join(args[2:], ",")
		}
		return parsed, nil
	case "list", "ls":
		parsed := worktreeArgs{Action: "list"}
		if len(args) > 1 {
			parsed.Filter = strings.Join(args[1:], ",")
		}
		return parsed, nil
	default:
		return worktreeArgs{}, fmt.Errorf("unknown worktree subcommand: %s", args[0])
	}
}

func isWorktreesOverrideToken(token string) bool {
	_, ok := command.ParseWorktreesFlag(token)
	return ok
}

func resolveWorktreesOverride(defaultValue bool, overrides ...command.WorktreesOverride) bool {
	value := defaultValue
	for _, override := range overrides {
		value = override.Resolve(value)
	}
	return value
}
