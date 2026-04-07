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
const commandHelpSummaryIndent = 25

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
			fmt.Print(shellInit())
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
	defaultFilter, hasDefaultFilter := command.GetDefaultContext(wsHome)
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
		if err := command.Setup(m, wsHome, filterArg(args, rawCtx, rawCtx != "")); err != nil {
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
		filter := filterArg(args, defaultFilter, hasDefaultFilter)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		if err := command.LL(m, wsHome, filter, includeWorktrees); err != nil {
			fatal(err)
		}

	case command.CommandFetch:
		filter := filterArg(args, defaultFilter, hasDefaultFilter)
		if err := command.Fetch(m, wsHome, filter); err != nil {
			fatal(err)
		}

	case command.CommandPull:
		args, localWorktrees := command.StripWorktreesFlags(args)
		filter := filterArg(args, defaultFilter, hasDefaultFilter)
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		if err := command.Pull(m, wsHome, filter, includeWorktrees); err != nil {
			fatal(err)
		}

	case "--":
		// Explicit escape: "ws -- [filter] <command...>"
		filter, cmdArgs, localWorktrees := command.ParseSuperArgs(m, args)
		if filter == "" && hasDefaultFilter {
			filter = defaultFilter
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: ws -- [-t|--worktrees|--no-worktrees] [filter] <command...>")
			os.Exit(1)
		}
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		if err := command.Super(m, wsHome, filter, cmdArgs, includeWorktrees); err != nil {
			fatal(err)
		}

	default:
		// Passthrough: treat as command to run across repos
		allArgs := append([]string{cmd}, args...)
		filter, cmdArgs, localWorktrees := command.ParseSuperArgs(m, allArgs)
		if filter == "" && hasDefaultFilter {
			filter = defaultFilter
		}
		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
			usage()
			os.Exit(1)
		}
		includeWorktrees := resolveWorktreesOverride(defaultWorktrees, globalWorktrees, localWorktrees)
		if err := command.Super(m, wsHome, filter, cmdArgs, includeWorktrees); err != nil {
			fatal(err)
		}
	}
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
_ws_delegate_bash() {
  local start actual_start cmd cur prev spec func cmdspec line
  start="${1:-0}"
  actual_start=$((start + 1))
  if [ "$actual_start" -gt "${#COMP_WORDS[@]}" ]; then
    return 0
  fi
  cmd="${COMP_WORDS[$actual_start]}"
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev=""
  if [ "$COMP_CWORD" -gt "$actual_start" ]; then
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi
  spec="$(complete -p "$cmd" 2>/dev/null)" || {
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
    return 0
  }

  local -a old_comp_words
  local old_comp_cword old_comp_line old_comp_point old_comp_type old_comp_key
  old_comp_words=( "${COMP_WORDS[@]}" )
  old_comp_cword=$COMP_CWORD
  old_comp_line="${COMP_LINE:-}"
  old_comp_point="${COMP_POINT:-0}"
  old_comp_type="${COMP_TYPE:-}"
  old_comp_key="${COMP_KEY:-}"

  COMP_WORDS=( "${old_comp_words[@]:$actual_start}" )
  COMP_CWORD=$((old_comp_cword - actual_start))
  COMP_LINE="${COMP_WORDS[*]}"
  COMP_POINT=${#COMP_LINE}

  if [[ "$spec" =~ [[:space:]]-F[[:space:]]+([^[:space:]]+) ]]; then
    func="${BASH_REMATCH[1]}"
    COMPREPLY=()
    "$func" "$cmd" "$cur" "$prev"
  elif [[ "$spec" =~ [[:space:]]-C[[:space:]]+([^[:space:]]+) ]]; then
    cmdspec="${BASH_REMATCH[1]}"
    COMPREPLY=( $(command "$cmdspec" "$cmd" "$cur" "$prev") )
  else
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
  fi

  COMP_WORDS=( "${old_comp_words[@]}" )
  COMP_CWORD=$old_comp_cword
  COMP_LINE="$old_comp_line"
  COMP_POINT=$old_comp_point
  COMP_TYPE="$old_comp_type"
  COMP_KEY="$old_comp_key"
}

_ws_complete_bash() {
  local cur prev delegate_start
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
  if [ "${#completions[@]}" -eq 1 ] && [[ "${completions[0]}" == "` + commandFallbackSentinel + `":* ]]; then
    delegate_start="${completions[0]#` + commandFallbackSentinel + `:}"
    _ws_delegate_bash "$delegate_start"
    return 0
  fi
  if [ "${#completions[@]}" -eq 1 ] && [ "${completions[0]}" = "` + commandFallbackSentinel + `" ]; then
    COMPREPLY=( $(compgen -c -- "$cur" | sort -u) )
    return 0
  fi
  COMPREPLY=( "${completions[@]}" )
}

_ws_delegate_zsh() {
  local start actual_start old_current
  local -a old_words
  start="${1:-0}"
  actual_start=$((start + 2))
  old_current=$CURRENT
  old_words=("${words[@]}")
  words=("${old_words[@]:$actual_start}")
  CURRENT=$((old_current - actual_start + 1))
  _normal
  words=("${old_words[@]}")
  CURRENT=$old_current
}

_ws_complete_zsh() {
  local prev delegate_start
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
  if (( ${#completions[@]} == 1 )) && [[ "${completions[1]}" == "` + commandFallbackSentinel + `":* ]]; then
    delegate_start="${completions[1]#` + commandFallbackSentinel + `:}"
    _ws_delegate_zsh "$delegate_start"
    return
  fi
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
	if result.DelegateCommands {
		fmt.Printf("%s:%d\n", commandFallbackSentinel, result.DelegateStart)
		return
	}
	if result.FallbackCommands {
		fmt.Println(commandFallbackSentinel)
		return
	}
	for _, value := range result.Values {
		fmt.Println(value)
	}
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
	fmt.Print(usageText())
}

func usageText() string {
	var b strings.Builder
	b.WriteString("Usage: ws [-w <path>] <command> [args]\n\nCommands:\n")
	for _, entry := range command.BuiltinUsageEntries() {
		writeUsageEntry(&b, entry)
	}
	b.WriteString(`
Any unrecognized command is run across repos:
  ws git status          Run "git status" in all repos
  ws -t git status
                         Run "git status" in all discovered worktrees
  ws ai git log -1       Run "git log -1" in a group
  ws ls -la              Any command, not just git

Use -- to escape built-in names:
  ws -- [-t|--worktrees|--no-worktrees] fetch data.json
                         Run "fetch data.json" (not git fetch)

Filters:
  all                    All active repos (default)
  <group>                Group name: ai, eng, db, inf
  <group>,<group>        Comma-separated groups
  <repo>                 Individual repo name
`)
	return b.String()
}

func writeUsageEntry(b *strings.Builder, entry command.HelpEntry) {
	if len(entry.Usage) <= commandHelpSummaryIndent-2 {
		fmt.Fprintf(b, "  %-*s %s\n", commandHelpSummaryIndent-2, entry.Usage, entry.Description)
		return
	}
	fmt.Fprintf(b, "  %s\n%*s%s\n", entry.Usage, commandHelpSummaryIndent, "", entry.Description)
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
		case "-t", "-W", "--worktrees", "--no-worktrees":
			continue
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
