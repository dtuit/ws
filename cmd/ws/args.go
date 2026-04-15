package main

import (
	"fmt"
	"strings"

	"github.com/dtuit/ws/internal/command"
	"github.com/dtuit/ws/internal/manifest"
)

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
