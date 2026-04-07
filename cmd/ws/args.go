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
		return "", "", fmt.Errorf("cd does not allow both repo@worktree and --worktree")
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

// filterArg returns the explicit filter if given, otherwise falls back to context.
func filterArg(args []string, defaultFilter string, hasDefaultFilter bool) string {
	if len(args) > 0 {
		return args[0]
	}
	if hasDefaultFilter {
		return defaultFilter
	}
	return ""
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
		if strings.HasPrefix(arg, "-") {
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
	case "set", "add", "remove", "save":
		action = tokens[0]
		tokens = tokens[1:]
	}
	parsed.Action = action

	if action == "save" {
		if parsed.WorktreesOverride.Set {
			return contextArgs{}, fmt.Errorf("ws context save does not accept -t|--worktrees|--no-worktrees")
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
	if len(tokens) == 0 {
		return contextArgs{}, fmt.Errorf("usage: ws context %s [-t|--worktrees|--no-worktrees] <filter>", action)
	}

	parsed.Filter = strings.Join(tokens, ",")
	return parsed, nil
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
