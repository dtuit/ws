package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dtuit/ws/internal/command"
	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/version"
)

const commandFallbackSentinel = "__ws_complete_commands__"

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
		fmt.Print(shellInit())
		return
	}
	if cmd == "__complete" {
		runCompletion(args)
		return
	}
	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		usage()
		return
	}
	if cmd == "version" || cmd == "--version" {
		fmt.Println(version.String())
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

	// Context: use as default filter when no explicit filter is given
	ctx := command.GetContext(wsHome)

	switch cmd {
	case "context":
		action, filter, err := parseContextArgs(args)
		if err != nil {
			fatal(err)
		}
		switch action {
		case "show":
			command.ShowContext(m, wsHome)
		case "set":
			if err := command.SetContext(m, wsHome, filter); err != nil {
				fatal(err)
			}
		case "add":
			if err := command.AddContext(m, wsHome, filter); err != nil {
				fatal(err)
			}
		}

	case "cd":
		if len(args) == 0 {
			fmt.Println(wsHome)
		} else {
			name := args[0]
			cfg, ok := m.ActiveRepos()[name]
			if !ok {
				fatal(fmt.Errorf("unknown repo: %s", name))
			}
			fmt.Println(m.ResolvePath(wsHome, name, cfg))
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
		if err := command.Setup(m, wsHome, filter, installShell); err != nil {
			fatal(err)
		}

	case "code":
		filter, err := parseCodeArgs(args, ctx)
		if err != nil {
			fatal(err)
		}
		if err := command.Code(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case "list":
		showAll := false
		for _, a := range args {
			if a == "--all" || a == "-a" {
				showAll = true
			}
		}
		if err := command.List(m, wsHome, showAll); err != nil {
			fatal(err)
		}

	case "ll":
		filter := filterArg(args, ctx)
		if err := command.LL(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case "fetch":
		filter := filterArg(args, ctx)
		if err := command.Fetch(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case "pull":
		filter := filterArg(args, ctx)
		if err := command.Pull(m, wsHome, filter); err != nil {
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
		if err := command.Super(m, wsHome, filter, cmdArgs); err != nil {
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
		if err := command.Super(m, wsHome, filter, cmdArgs); err != nil {
			fatal(err)
		}
	}
}

func shellInit() string {
	return `ws() {
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
_ws_complete_bash() {
  local cur prev
  local -a completions
  COMPREPLY=()
  completions=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev=""
  if [ "${COMP_CWORD}" -gt 0 ]; then
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  if [ "$prev" = "-w" ] || [ "$prev" = "--workspace" ]; then
    COMPREPLY=( $(compgen -d -- "$cur") )
    return 0
  fi
  while IFS= read -r line; do
    completions+=( "$line" )
  done < <(command ws __complete "$((COMP_CWORD-1))" "${COMP_WORDS[@]:1}")
  if [ "${#completions[@]}" -eq 1 ] && [ "${completions[0]}" = "` + commandFallbackSentinel + `" ]; then
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
    return 0
  fi
  COMPREPLY=( "${completions[@]}" )
}

_ws_complete_zsh() {
  local prev
  local -a ws_words completions
  if (( CURRENT > 1 )); then
    prev="${words[CURRENT-1]}"
  else
    prev=""
  fi
  if [[ "$prev" == "-w" || "$prev" == "--workspace" ]]; then
    _files -/
    return
  fi
  ws_words=()
  local i
  for (( i = 2; i <= ${#words}; i++ )); do
    ws_words+=("${words[i]}")
  done
  completions=("${(@f)$(command ws __complete "$((CURRENT-2))" "${ws_words[@]}")}")
  if (( ${#completions[@]} == 1 )) && [[ "${completions[1]}" == "` + commandFallbackSentinel + `" ]]; then
    _command_names
    return
  fi
  if (( ${#completions[@]} > 0 )); then
    compadd -- "${completions[@]}"
  fi
}

if [ -n "${BASH_VERSION:-}" ]; then
  complete -F _ws_complete_bash ws
elif [ -n "${ZSH_VERSION:-}" ]; then
  if ! whence compdef >/dev/null 2>&1; then
    autoload -Uz compinit
    compinit
  fi
  compdef _ws_complete_zsh ws
fi
`
}

func runCompletion(args []string) {
	if len(args) == 0 {
		return
	}

	current, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}

	words := args[1:]
	m := loadManifestForCompletion(words)
	result := command.Complete(m, words, current)
	if result.FallbackCommands {
		fmt.Println(commandFallbackSentinel)
		return
	}
	for _, value := range result.Values {
		fmt.Println(value)
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

func parseCodeArgs(args []string, ctx string) (string, error) {
	var filterArgs []string

	for _, arg := range args {
		switch arg {
		case "-t", "--worktrees":
			// Worktree inclusion is the current default for ws code.
		default:
			if strings.HasPrefix(arg, "-") {
				return "", fmt.Errorf("unknown code flag: %s", arg)
			}
			filterArgs = append(filterArgs, arg)
		}
	}

	if len(filterArgs) > 1 {
		return "", fmt.Errorf("usage: ws code [-t|--worktrees] [filter]")
	}

	return filterArg(filterArgs, ctx), nil
}

func parseContextArgs(args []string) (action string, filter string, err error) {
	if len(args) == 0 {
		return "show", "", nil
	}
	if args[0] == "add" {
		if len(args) == 1 {
			return "", "", fmt.Errorf("usage: ws context add <filter>")
		}
		return "add", strings.Join(args[1:], ","), nil
	}
	return "set", strings.Join(args, ","), nil
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
  init                   Emit shell integration and completion
  ll [filter]            Dashboard: branch, dirty, last commit
  cd [repo]              Print repo path (no arg = workspace root)
  setup [filter]         Clone missing repos
  code [-t|--worktrees] [filter]
                         Generate VS Code workspace and open it
  list [--all]           Show repos in manifest (--all includes excluded)
  fetch [filter]         Fetch all repos
  pull [filter]          Pull all repos
  context [filter]       Set default filter (no arg = show, "none" = clear)
  context add <filter>   Add groups or repos to the existing context

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

func loadManifestForCompletion(words []string) *manifest.Manifest {
	override := completionWorkspaceOverride(words)
	wsHome, err := findWorkspaceHome(override)
	if err != nil {
		return nil
	}

	m, err := manifest.LoadWithLocal(wsHome)
	if err != nil {
		return nil
	}
	return m
}

func completionWorkspaceOverride(words []string) string {
	for i := 0; i < len(words); i++ {
		switch words[i] {
		case "-w", "--workspace":
			if i+1 >= len(words) {
				return ""
			}
			return strings.TrimSpace(words[i+1])
		default:
			if !strings.HasPrefix(words[i], "-") {
				return ""
			}
		}
	}
	return ""
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
