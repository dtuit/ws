package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dtuit/ws/internal/command"
	"github.com/dtuit/ws/internal/manifest"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "help"
	var wsHomeOverride string

	// Parse global flags before command
	for len(args) > 0 {
		if (args[0] == "-w" || args[0] == "--workspace") && len(args) > 1 {
			wsHomeOverride = args[1]
			args = args[2:]
		} else {
			break
		}
	}

	// Handle "ws -- [filter] <command...>" before normal dispatch.
	if len(args) > 0 && args[0] == "--" {
		cmd = "--"
		args = args[1:]
	} else if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	if cmd == "init" {
		fmt.Print(`ws() {
  case "$1" in
    cd)
      local dir
      dir="$(command ws cd "${@:2}")" && cd "$dir"
      ;;
    *)
      command ws "$@"
      ;;
  esac
}
`)
		return
	}
	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		usage()
		return
	}
	if cmd == "version" || cmd == "--version" {
		fmt.Println("ws " + version)
		return
	}

	wsHome, err := findWorkspaceHome(wsHomeOverride)
	if err != nil {
		fatal(err)
	}

	m, err := manifest.LoadWithLocal(wsHome)
	if err != nil {
		fatal(err)
	}

	parentDir := m.ResolveRoot(wsHome)

	// Context: use as default filter when no explicit filter is given
	ctx := command.GetContext(wsHome)

	switch cmd {
	case "context":
		if len(args) > 0 {
			if err := command.SetContext(m, wsHome, parentDir, args[0]); err != nil {
				fatal(err)
			}
		} else {
			command.ShowContext(m, wsHome)
		}

	case "cd":
		if len(args) == 0 {
			fmt.Println(wsHome)
		} else {
			name := args[0]
			if _, ok := m.ActiveRepos()[name]; !ok {
				fatal(fmt.Errorf("unknown repo: %s", name))
			}
			fmt.Println(filepath.Join(parentDir, name))
		}

	case "setup":
		installShell := false
		var filterArgs []string
		for _, a := range args {
			if a == "--install-shell" {
				installShell = true
			} else {
				filterArgs = append(filterArgs, a)
			}
		}
		filter := filterArg(filterArgs, ctx)
		if err := command.Setup(m, parentDir, wsHome, filter, installShell); err != nil {
			fatal(err)
		}

	case "code":
		filter := filterArg(args, ctx)
		if err := command.Code(m, parentDir, wsHome, filter); err != nil {
			fatal(err)
		}

	case "list":
		showAll := false
		for _, a := range args {
			if a == "--all" || a == "-a" {
				showAll = true
			}
		}
		if err := command.List(m, parentDir, showAll); err != nil {
			fatal(err)
		}

	case "ll":
		filter := filterArg(args, ctx)
		if err := command.LL(m, parentDir, filter); err != nil {
			fatal(err)
		}

	case "fetch":
		filter := filterArg(args, ctx)
		if err := command.Fetch(m, parentDir, filter); err != nil {
			fatal(err)
		}

	case "pull":
		filter := filterArg(args, ctx)
		if err := command.Pull(m, parentDir, filter); err != nil {
			fatal(err)
		}

	case "--":
		// Explicit escape: "ws -- [filter] <command...>"
		filter, cmdArgs := command.ParseSuperArgs(m, args)
		if filter == "" {
			filter = ctx
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: ws -- [filter] <command...>")
			os.Exit(1)
		}
		if err := command.Super(m, parentDir, filter, cmdArgs); err != nil {
			fatal(err)
		}

	default:
		// Passthrough: treat as command to run across repos
		allArgs := append([]string{cmd}, args...)
		filter, cmdArgs := command.ParseSuperArgs(m, allArgs)
		if filter == "" {
			filter = ctx
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
			usage()
			os.Exit(1)
		}
		if err := command.Super(m, parentDir, filter, cmdArgs); err != nil {
			fatal(err)
		}
	}
}

// filterArg returns the explicit filter if given, otherwise falls back to context.
func filterArg(args []string, ctx string) string {
	if len(args) > 0 {
		return args[0]
	}
	if ctx != "" {
		return ctx
	}
	return ""
}

func findWorkspaceHome(override string) (string, error) {
	// 0. -w flag takes priority
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "manifest.yml")); err == nil {
			return abs, nil
		}
		return "", fmt.Errorf("-w %s: no manifest.yml found there", override)
	}

	// 1. Check WS_HOME env var
	if home := os.Getenv("WS_HOME"); home != "" {
		abs, err := filepath.Abs(home)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "manifest.yml")); err == nil {
			return abs, nil
		}
		return "", fmt.Errorf("WS_HOME=%s but no manifest.yml found there", home)
	}

	// 2. Walk up from cwd (max 10 levels to avoid picking up stray manifests)
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "manifest.yml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("manifest.yml not found; set WS_HOME or run from within the workspace")
}

func usage() {
	fmt.Print(`Usage: ws [-w <path>] <command> [args]

Commands:
  ll [filter]            Dashboard: branch, dirty, last commit
  cd [repo]              Print repo path (no arg = workspace root)
  setup [filter]         Clone missing repos
  code [filter]          Generate VS Code workspace and open it
  list [--all]           Show repos in manifest (--all includes excluded)
  fetch [filter]         Fetch all repos
  pull [filter]          Pull all repos
  context [filter]       Set default filter (no arg = show, "none" = clear)

Any unrecognized command is run across repos:
  ws git status          Run "git status" in all repos
  ws ai git log -1       Run "git log -1" in a group
  ws ls -la              Any command, not just git

Use -- to escape built-in names:
  ws -- fetch data.json  Run "fetch data.json" (not git fetch)

Filters:
  all                    All repos in any group (default)
  <group>                Group name: ai, eng, db, inf
  <group>,<group>        Comma-separated groups
  <repo>                 Individual repo name
`)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
