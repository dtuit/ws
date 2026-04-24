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

	// Per-command help: ws <cmd> --help / ws <cmd> -h / ws <cmd> help
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		if help, ok := command.CommandHelpText(cmd); ok {
			fmt.Print(help)
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
		case "refresh":
			if err := command.RefreshContext(m, wsHome, includeWorktrees); err != nil {
				fatal(err)
			}
		case "swap":
			if err := command.SwapContext(m, wsHome, includeWorktrees); err != nil {
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
			repo := m.RepoInfoFor(wsHome, name, cfg, m.RepoGroups()[name])
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
		// --all / -a are sugar for the "all" filter (discoverable alongside
		// `ws repos --all`). Strip them before the positional parse.
		var showAll bool
		args, showAll = stripBoolFlag(args, "--all", "-a")
		if showAll && len(args) > 0 {
			fatal(fmt.Errorf("ws setup --all takes no filter (got %q)", args[0]))
		}

		// With no filter arg and no context, show the setup guide instead
		// of cloning everything — large workspaces shouldn't nuke disk by
		// default.
		if !showAll && len(args) == 0 && rawCtx == "" {
			if err := command.SetupGuide(m, wsHome); err != nil {
				fatal(err)
			}
			return
		}

		filter, err := parseOptionalFilterArg(args, rawCtx, rawCtx != "", "ws setup [filter]")
		if err != nil {
			fatal(err)
		}
		if showAll {
			filter = "all"
		}
		cloned, err := command.Setup(m, wsHome, filter)
		if err != nil {
			fatal(err)
		}
		// If a context is set and we actually cloned something, refresh
		// the scope symlinks and VS Code workspace file so they pick up
		// the newly-cloned paths. Use the same worktree resolution as
		// other commands in this dispatch.
		if cloned > 0 && rawCtx != "" {
			includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees)
			if err := command.RefreshContext(m, wsHome, includeWorktrees); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not refresh context: %v\n", err)
			}
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
		editor, args := parseEditorFlag(args)
		if len(args) > 0 {
			fatal(fmt.Errorf("usage: ws open [--editor <name>]"))
		}
		if err := command.Open(m, wsHome, command.ResolveEditor(editor)); err != nil {
			fatal(err)
		}

	case command.CommandBrowse:
		var opts command.BrowseOptions
		args, opts.Yes = stripBoolFlag(args, "--yes", "-y")
		if len(args) != 1 {
			fatal(fmt.Errorf("usage: ws browse <repo> [-y|--yes]"))
		}
		if err := command.Browse(m, wsHome, args[0], opts); err != nil {
			fatal(err)
		}

	case command.CommandRepos:
		showAll := false
		args, showAll = stripBoolFlag(args, "--all", "-a")
		args, localWorktrees := command.StripWorktreesFlags(args)
		if len(args) > 0 {
			fatal(fmt.Errorf("repos does not take a filter"))
		}
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		if err := command.List(m, wsHome, showAll, includeWorktrees); err != nil {
			fatal(err)
		}

	case command.CommandDirs:
		var root bool
		args, root = stripBoolFlag(args, "--root")
		args, localWorktrees := command.StripWorktreesFlags(args)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, includeWorktrees)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws dirs [--root] [filter] ["+command.WorktreesFlagUsage+"]")
		if err != nil {
			fatal(err)
		}
		if err := command.Dirs(m, wsHome, filter, includeWorktrees, root); err != nil {
			fatal(err)
		}

	case command.CommandLL:
		args, showBranches := command.StripLLBranchesFlags(args)
		args, localWorktrees := command.StripWorktreesFlags(args)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, includeWorktrees)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws ll ["+command.LLBranchesFlagUsage+"] [filter] ["+command.WorktreesFlagUsage+"]")
		if err != nil {
			fatal(err)
		}
		if err := command.LL(m, wsHome, filter, includeWorktrees, command.LLMode{ShowBranches: showBranches}); err != nil {
			fatal(err)
		}

	case command.CommandFetch:
		args, remoteNames := stripRepeatedValueFlag(args, "--remote")
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, false)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws fetch [--remote <name>] [filter]")
		if err != nil {
			fatal(err)
		}
		if err := command.Fetch(m, wsHome, filter, remoteNames); err != nil {
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

	case command.CommandMux:
		parsed, err := parseMuxArgs(args)
		if err != nil {
			fatal(err)
		}
		switch parsed.Action {
		case "attach":
			if err := command.MuxAttachOrCreate(m, wsHome, parsed.SessionName); err != nil {
				fatal(err)
			}
		case "kill":
			if err := command.MuxKill(m, wsHome, parsed.SessionName); err != nil {
				fatal(err)
			}
		case "ls":
			if err := command.MuxList(m, wsHome); err != nil {
				fatal(err)
			}
		case "save":
			if err := command.MuxSave(m, wsHome, parsed.SessionName, parsed.Local); err != nil {
				fatal(err)
			}
		case "dup":
			if err := command.MuxDup(m, wsHome, parsed.WindowName); err != nil {
				fatal(err)
			}
		}

	case command.CommandAgent:
		parsed, err := parseAgentArgs(args)
		if err != nil {
			fatal(err)
		}
		switch parsed.Action {
		case "start":
			if err := command.AgentStart(m, wsHome, parsed.Repo, parsed.Agent, parsed.Passthrough); err != nil {
				fatal(err)
			}
		case "ls":
			limit := parsed.Limit
			if limit == 0 && !parsed.ShowAll {
				limit = command.AgentDefaultLimit
			}
			mode := command.AgentListMode{
				Verbose:   parsed.Verbose,
				ShowLast:  parsed.ShowLast,
				ShowRecap: parsed.ShowRecap,
			}
			if err := command.AgentList(m, wsHome, parsed.Filter, false, limit, mode); err != nil {
				fatal(err)
			}
		case "resume":
			if err := command.AgentResume(m, wsHome, parsed.IndexOrID); err != nil {
				fatal(err)
			}
		case "pin":
			if err := command.AgentPin(m, wsHome, parsed.IndexOrID); err != nil {
				fatal(err)
			}
		case "unpin":
			if err := command.AgentUnpin(m, wsHome, parsed.IndexOrID); err != nil {
				fatal(err)
			}
		}

	case command.CommandRemotes:
		if len(args) == 0 || args[0] != "sync" {
			fatal(fmt.Errorf("usage: ws remotes sync [filter]"))
		}
		args = args[1:]
		defaultFilter, hasDefaultFilter := command.GetDefaultContextForMode(m, wsHome, false)
		filter, err := parseOptionalFilterArg(args, defaultFilter, hasDefaultFilter, "ws remotes sync [filter]")
		if err != nil {
			fatal(err)
		}
		if err := command.RemotesSync(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case command.CommandWorktree:
		parsed, err := parseWorktreeArgs(args)
		if err != nil {
			fatal(err)
		}
		switch parsed.Action {
		case "add":
			if err := command.WorktreeAdd(m, wsHome, parsed.Branch, parsed.Filter); err != nil {
				fatal(err)
			}
		case "remove":
			if err := command.WorktreeRemove(m, wsHome, parsed.Branch, parsed.Filter); err != nil {
				fatal(err)
			}
		case "list":
			if err := command.WorktreeListCmd(m, wsHome, parsed.Filter); err != nil {
				fatal(err)
			}
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
