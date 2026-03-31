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
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		usage()
		return
	}
	if cmd == "version" || cmd == "--version" {
		fmt.Println("ws " + version)
		return
	}

	wsHome, err := findWorkspaceHome()
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
			if err := command.SetContext(m, wsHome, args[0]); err != nil {
				fatal(err)
			}
		} else {
			command.ShowContext(m, wsHome)
		}

	case "setup":
		filter := filterArg(args, ctx)
		if err := command.Setup(m, parentDir, filter); err != nil {
			fatal(err)
		}

	case "focus":
		filter := filterArg(args, ctx)
		if err := command.Focus(m, parentDir, wsHome, filter); err != nil {
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

	case "super":
		filter, cmdArgs := command.ParseSuperArgs(m, args)
		if filter == "" {
			filter = ctx
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: ws super [filter] <command...>")
			os.Exit(1)
		}
		if err := command.Super(m, parentDir, filter, cmdArgs); err != nil {
			fatal(err)
		}

	case "fetch":
		filter := filterArg(args, ctx)
		if err := command.Super(m, parentDir, filter, []string{"git", "fetch"}); err != nil {
			fatal(err)
		}

	case "pull":
		filter := filterArg(args, ctx)
		if err := command.Super(m, parentDir, filter, []string{"git", "pull"}); err != nil {
			fatal(err)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
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

func findWorkspaceHome() (string, error) {
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
	fmt.Print(`Usage: ws <command> [args]

Commands:
  setup [filter]         Clone missing repos (default: all groups)
  focus [filter]         Filter VS Code workspace folders
  list [--all]           Show repos in manifest (--all includes excluded)
  ll [filter]            Dashboard: branch, dirty, last commit
  super [filter] <cmd>   Run command across repos
  fetch [filter]         Fetch all repos (sugar for: super git fetch)
  pull [filter]          Pull all repos (sugar for: super git pull)
  context [filter]       Set default filter for all commands (no arg = show, "none" = clear)

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
