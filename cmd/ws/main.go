package main

import (
	"fmt"
	"os"

	"github.com/dtuit/ws/internal/command"
	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/version"
)

func main() {
	args := os.Args[1:]
	cmd := command.CommandHelp
	var wsHomeOverride string
	var globalWorktrees command.WorktreesOverride

	// Parse global flags before command
	for len(args) > 0 {
		switch {
		case (args[0] == "-w" || args[0] == "--workspace") && len(args) > 1:
			wsHomeOverride = args[1]
			args = args[2:]
		case isWorktreesOverrideToken(args[0]):
			globalWorktrees, _ = command.ParseWorktreesFlag(args[0])
			args = args[1:]
		default:
			goto dispatch
		}
	}

dispatch:
	// Handle "ws -- [filter] <command...>" before normal dispatch.
	if len(args) > 0 && args[0] == "--" {
		cmd = "--"
		args = args[1:]
	} else if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	if cmd == "__complete" {
		runCompletion(args)
		return
	}
	cmd = command.ResolveBuiltinCommandName(cmd)
	if cmd == command.CommandHelp || cmd == "--help" || cmd == "-h" {
		usage()
		return
	}
	if cmd == "init" {
		fatal(fmt.Errorf("ws init has moved to `ws shell init`"))
	}
	if cmd == command.CommandVersion || cmd == "--version" {
		fmt.Println(version.String())
		return
	}
	if cmd == command.CommandShell {
		parsed, err := parseShellArgs(args)
		if err != nil {
			fatal(err)
		}
		if parsed.Action == "init" {
			fmt.Print(command.ShellInitScript())
			return
		}
	}

	wsHome, err := findWorkspaceHome(wsHomeOverride)
	if err != nil {
		fatal(err)
	}

	m, err := manifest.LoadWithLocal(wsHome)
	if err != nil {
		fatal(err)
	}

	// Context: use the resolved scoped repos as the default filter when present.
	rawCtx := command.GetContext(wsHome)
	defaultWorktrees := m.Worktrees

	switch cmd {
	case command.CommandContext:
		parsed, err := parseContextArgs(args)
		if err != nil {
			fatal(err)
		}
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, parsed.WorktreesOverride)
		switch parsed.Action {
		case "show":
			command.ShowContext(m, wsHome)
		case "set":
			if err := command.SetContext(m, wsHome, parsed.Filter, includeWorktrees); err != nil {
				fatal(err)
			}
		case "add":
			if err := command.AddContext(m, wsHome, parsed.Filter, includeWorktrees); err != nil {
				fatal(err)
			}
		case "remove":
			if err := command.RemoveContext(m, wsHome, parsed.Filter, includeWorktrees); err != nil {
				fatal(err)
			}
		case "save":
			if err := command.SaveContextGroup(m, wsHome, parsed.Group, parsed.Local); err != nil {
				fatal(err)
			}
		}

	case command.CommandCD:
		if len(args) == 0 {
			fmt.Println(wsHome)
		} else {
			name, selector, err := parseCDArgs(args)
			if err != nil {
				fatal(err)
			}
			active := m.ActiveRepos()
			name, selector, err = resolveCDTarget(name, selector, active)
			if err != nil {
				fatal(err)
			}
			cfg, ok := active[name]
			if !ok {
				fatal(fmt.Errorf("unknown repo: %s", name))
			}
			repo := manifest.RepoInfo{
				Name:   name,
				URL:    m.ResolveURL(name, cfg),
				Branch: m.ResolveBranch(cfg),
				Groups: m.RepoGroups()[name],
				Path:   m.ResolvePath(wsHome, name, cfg),
			}
			path, err := command.CDPath(repo, selector)
			if err != nil {
				fatal(err)
			}
			fmt.Println(path)
		}

	case command.CommandSetup:
		for _, arg := range args {
			if arg == "--install-shell" {
				fatal(fmt.Errorf("ws setup --install-shell has moved to `ws shell install`"))
			}
		}
		filter, err := parseOptionalFilterArg(args, rawCtx, rawCtx != "", "ws setup [filter]")
		if err != nil {
			fatal(err)
		}
		if err := command.Setup(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case command.CommandShell:
		parsed, err := parseShellArgs(args)
		if err != nil {
			fatal(err)
		}
		if parsed.Action != "install" {
			fatal(fmt.Errorf("usage: ws shell <init|install>"))
		}
		if err := command.InstallShellConfig(wsHome); err != nil {
			fatal(err)
		}

	case command.CommandOpen:
		if len(args) > 0 {
			fatal(fmt.Errorf("usage: ws open"))
		}
		if err := command.Open(m, wsHome); err != nil {
			fatal(err)
		}

	case command.CommandList:
		showAll := false
		args, showAll = stripBoolFlag(args, "--all", "-a")
		args, localWorktrees := command.StripWorktreesFlags(args)
		if len(args) > 0 {
			fatal(fmt.Errorf("list does not take a filter"))
		}
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		if err := command.List(m, wsHome, showAll, includeWorktrees); err != nil {
			fatal(err)
		}

	case command.CommandLL:
		args, localWorktrees := command.StripWorktreesFlags(args)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, includeWorktrees)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws ll [filter] ["+command.WorktreesFlagUsage+"]")
		if err != nil {
			fatal(err)
		}
		if err := command.LL(m, wsHome, filter, includeWorktrees); err != nil {
			fatal(err)
		}

	case command.CommandFetch:
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, false)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws fetch [filter]")
		if err != nil {
			fatal(err)
		}
		if err := command.Fetch(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case command.CommandPull:
		args, localWorktrees := command.StripWorktreesFlags(args)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, includeWorktrees)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws pull [filter] ["+command.WorktreesFlagUsage+"]")
		if err != nil {
			fatal(err)
		}
		if err := command.Pull(m, wsHome, filter, includeWorktrees); err != nil {
			fatal(err)
		}

	case "--":
		// Explicit escape: "ws -- [filter] <command...>"
		filter, cmdArgs, localWorktrees := command.ParseSuperArgs(m, args)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, includeWorktrees)
		if filter == "" && hasDefaultFilter {
			filter = defaultFilter
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: ws -- [%s] [filter] <command...>\n", command.WorktreesFlagUsage)
			os.Exit(1)
		}
		if err := command.Super(m, wsHome, filter, cmdArgs, includeWorktrees); err != nil {
			fatal(err)
		}

	default:
		// Passthrough: treat as command to run across repos
		allArgs := append([]string{cmd}, args...)
		filter, cmdArgs, localWorktrees := command.ParseSuperArgs(m, allArgs)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, includeWorktrees)
		if filter == "" && hasDefaultFilter {
			filter = defaultFilter
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
			usage()
			os.Exit(1)
		}
		if err := command.Super(m, wsHome, filter, cmdArgs, includeWorktrees); err != nil {
			fatal(err)
		}
	}
}
