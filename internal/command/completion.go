package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

// CompletionResult describes completion candidates and any shell fallback.
type CompletionResult struct {
	Values           []string
	FallbackCommands bool
	DelegateCommands bool
	DelegateStart    int
}

// CompletionHandler returns shell completion candidates for one built-in command.
type CompletionHandler func(m *manifest.Manifest, args []string, current int) CompletionResult

// CompletionOutput renders completion lines, including shell delegation markers.
func CompletionOutput(m *manifest.Manifest, words []string, current int) []string {
	result := Complete(m, words, current)
	if result.DelegateCommands {
		return []string{fmt.Sprintf("%s:%d", CompletionCommandFallbackSentinel, result.DelegateStart)}
	}
	if result.FallbackCommands {
		return []string{CompletionCommandFallbackSentinel}
	}
	return result.Values
}

// Complete returns shell completion candidates for ws arguments.
func Complete(m *manifest.Manifest, words []string, current int) CompletionResult {
	if current < 0 {
		current = 0
	}
	if current >= len(words) {
		words = append(words, "")
		current = len(words) - 1
	}

	commandIndex := firstCommandIndex(words)
	currentWord := words[current]

	if current < commandIndex {
		return CompletionResult{}
	}

	if commandIndex >= len(words) || current == commandIndex {
		values := append(globalFlagSuggestions(), BuiltinCommandSuggestions()...)
		values = append(values, filterSuggestions(m)...)
		return finalizeCompletion(values, currentWord, true)
	}

	cmd := ResolveBuiltinCommandName(words[commandIndex])
	args := words[commandIndex+1:]
	argIndex := current - commandIndex - 1

	if cmd == "--" {
		return completePassthrough(m, args, argIndex)
	}
	if builtin, ok := builtinCommandByName(cmd); ok {
		if builtin.complete == nil {
			return CompletionResult{}
		}
		return builtin.complete(m, args, argIndex)
	}
	return completeDefaultPassthrough(m, append([]string{cmd}, args...), current-commandIndex)
}

func completeDefaultPassthrough(m *manifest.Manifest, words []string, current int) CompletionResult {
	if len(words) == 0 {
		return CompletionResult{}
	}
	if current == 0 {
		if isFilterToken(m, words[0]) {
			return CompletionResult{FallbackCommands: true}
		}
		return finalizeCompletion(nil, words[0], true)
	}
	if isFilterToken(m, words[0]) && current == 1 {
		return CompletionResult{FallbackCommands: true}
	}
	if isFilterToken(m, words[0]) && current > 1 {
		return CompletionResult{DelegateCommands: true, DelegateStart: 1}
	}
	if !isFilterToken(m, words[0]) && current > 0 {
		return CompletionResult{DelegateCommands: true, DelegateStart: 0}
	}
	return CompletionResult{}
}

func completeNoopCommand(_ *manifest.Manifest, _ []string, _ int) CompletionResult {
	return CompletionResult{}
}

func completeCDCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	currentWord := completionWord(args, current)
	switch current {
	case 0:
		return finalizeCompletion(repoSuggestions(m), currentWord, false)
	case 1:
		return finalizeCompletion([]string{"--worktree", "-t"}, currentWord, false)
	default:
		return CompletionResult{}
	}
}

func completeReposCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	return finalizeCompletion(append([]string{"--all", "-a"}, worktreesFlagSuggestions()...), completionWord(args, current), false)
}

func completeBrowseCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	currentWord := completionWord(args, current)
	if current == 0 {
		values := append([]string{".", "--yes", "-y"}, repoSuggestions(m)...)
		return finalizeCompletion(values, currentWord, false)
	}
	return finalizeCompletion([]string{"--yes", "-y"}, currentWord, false)
}

func completeAgentCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	currentWord := completionWord(args, current)
	if current == 0 {
		values := []string{"list", "ls", "resume", "pin", "unpin", "--agent", "-a"}
		values = append(values, repoSuggestions(m)...)
		return finalizeCompletion(values, currentWord, false)
	}
	if len(args) > 0 && (args[0] == "list" || args[0] == "ls") {
		result := completeFilterCommand(m, args[1:], current-1, []string{"-n", "--all", "-v", "--verbose"})
		// Add agent-specific filter tokens when completing a filter position
		if current-1 >= 0 {
			for _, extra := range []string{".", "root", "external"} {
				if strings.HasPrefix(extra, currentWord) {
					result.Values = append(result.Values, extra)
				}
			}
		}
		return result
	}
	return finalizeCompletion(repoSuggestions(m), currentWord, false)
}

func completeDirsCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	flags := append([]string{"--root"}, worktreesFlagSuggestions()...)
	return completeFilterCommand(m, args, current, flags)
}

func completeSetupCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		values := append([]string{"--all", "-a"}, filterSuggestions(m)...)
		return finalizeCompletion(values, completionWord(args, current), false)
	}
	return CompletionResult{}
}

func completeFetchCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		return finalizeCompletion(filterSuggestions(m), completionWord(args, current), false)
	}
	return CompletionResult{}
}

func completeLLCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	return completeFilterCommand(m, args, current, llFlagSuggestions())
}

func completeLLOrPullCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	return completeFilterCommand(m, args, current, worktreesFlagSuggestions())
}

func completeFilterCommand(m *manifest.Manifest, args []string, current int, flags []string) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	if current == 0 {
		values := append(flags, filterSuggestions(m)...)
		return finalizeCompletion(values, completionWord(args, current), false)
	}

	seenFlag := false
	filterIndex := 0
	for i, arg := range args {
		if hasFlag(flags, arg) {
			if i == current {
				return finalizeCompletion(flags, completionWord(args, current), false)
			}
			seenFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return CompletionResult{}
		}
		if i == current {
			if filterIndex == 0 {
				return finalizeCompletion(filterSuggestions(m), completionWord(args, current), false)
			}
			return CompletionResult{}
		}
		filterIndex++
	}

	if seenFlag && current == len(args) {
		return finalizeCompletion(filterSuggestions(m), "", false)
	}

	return CompletionResult{}
}

func completeShellCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	if current == 0 {
		return finalizeCompletion([]string{"init", "install"}, completionWord(args, current), false)
	}
	return CompletionResult{}
}

func completeContextCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}

	flags := worktreesFlagSuggestions()
	currentWord := completionWord(args, current)

	var nonFlags []string
	seenLocal := false
	for i, arg := range args {
		if i == current {
			continue
		}
		if hasFlag(flags, arg) {
			continue
		}
		if arg == "--local" {
			seenLocal = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return CompletionResult{}
		}
		nonFlags = append(nonFlags, arg)
	}

	if len(nonFlags) == 0 {
		values := append(flags, filterSuggestions(m)...)
		values = append(values, "none", "reset", "add", "remove", "save", "refresh", ".", "-", "prev")
		return finalizeCompletion(values, currentWord, false)
	}

	if nonFlags[0] == "save" {
		values := []string{}
		if !seenLocal {
			values = append(values, "--local")
		}
		return finalizeCompletion(values, currentWord, false)
	}

	if nonFlags[0] == "add" || nonFlags[0] == "remove" {
		values := append(flags, filterSuggestions(m)...)
		return finalizeCompletion(values, currentWord, false)
	}
	if nonFlags[0] == "refresh" || nonFlags[0] == "." {
		return finalizeCompletion(flags, currentWord, false)
	}

	values := append(flags, filterSuggestions(m)...)
	return finalizeCompletion(values, currentWord, false)
}

func completeMuxCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		return finalizeCompletion([]string{"dup", "kill", "list", "ls", "save"}, completionWord(args, current), false)
	}
	return CompletionResult{}
}

func completeWorktreeCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		return finalizeCompletion([]string{"add", "remove", "list", "ls"}, completionWord(args, current), false)
	}
	if current >= 2 && len(args) > 0 {
		action := args[0]
		if action == "add" || action == "remove" || action == "list" || action == "ls" {
			return finalizeCompletion(filterSuggestions(m), completionWord(args, current), false)
		}
	}
	return CompletionResult{}
}

func completePassthrough(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	start := superCommandStart(m, args)
	if start < 0 {
		return CompletionResult{}
	}
	if current < start {
		if current == 0 {
			values := append(worktreesFlagSuggestions(), filterSuggestions(m)...)
			return finalizeCompletion(values, args[0], true)
		}
		if len(args) > 0 && isWorktreesFlag(args[0]) && current == 1 {
			return finalizeCompletion(filterSuggestions(m), args[1], true)
		}
		return CompletionResult{}
	}
	if current == start {
		return CompletionResult{FallbackCommands: true}
	}
	return CompletionResult{DelegateCommands: true, DelegateStart: start + 1}
}

func superCommandStart(m *manifest.Manifest, args []string) int {
	for i, arg := range args {
		switch {
		case isWorktreesFlag(arg):
			continue
		case isFilterToken(m, arg):
			continue
		default:
			return i
		}
	}
	return -1
}

func firstCommandIndex(words []string) int {
	for i := 0; i < len(words); i++ {
		switch words[i] {
		case "-w", "--workspace":
			if i+1 >= len(words) {
				return len(words)
			}
			i++
		case "-t", "--worktrees", "--no-worktrees":
			continue
		default:
			return i
		}
	}
	return len(words)
}

func finalizeCompletion(values []string, currentWord string, allowCommandFallback bool) CompletionResult {
	filtered := matchPrefix(values, currentWord)
	if len(filtered) == 0 && allowCommandFallback && currentWord != "" && !strings.HasPrefix(currentWord, "-") {
		return CompletionResult{FallbackCommands: true}
	}
	return CompletionResult{Values: filtered}
}

func completionWord(args []string, current int) string {
	if current < 0 || current >= len(args) {
		return ""
	}
	return args[current]
}

func matchPrefix(values []string, currentWord string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		if currentWord == "" || strings.HasPrefix(value, currentWord) {
			out = append(out, value)
			seen[value] = true
		}
	}
	sort.Strings(out)
	return out
}

func globalFlagSuggestions() []string {
	return []string{"-w", "--workspace", "-t", "--worktrees", "--no-worktrees", "-h", "--help", "--version"}
}

func filterSuggestions(m *manifest.Manifest) []string {
	values := []string{
		"all",
		activeFilterToken,
		activeFilterToken + ":1d",
		dirtyFilterToken,
		mineFilterToken + ":1d",
	}
	if m == nil {
		return values
	}

	groupNames := make([]string, 0, len(m.Groups))
	for group := range m.Groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)
	values = append(values, groupNames...)

	repoNames := make([]string, 0, len(m.ActiveRepos()))
	for name := range m.ActiveRepos() {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)
	values = append(values, repoNames...)

	return values
}

func repoSuggestions(m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}

	repoNames := make([]string, 0, len(m.ActiveRepos()))
	for name := range m.ActiveRepos() {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)
	return repoNames
}

func isFilterToken(m *manifest.Manifest, token string) bool {
	if token == "" {
		return false
	}

	var active map[string]manifest.RepoConfig
	if m != nil {
		active = m.ActiveRepos()
	}

	for _, part := range strings.Split(token, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "all" {
			return true
		}
		if _, ok, _ := parseActivityFilterToken(part); ok {
			return true
		}

		if m == nil {
			continue
		}
		if _, ok := active[part]; ok {
			return true
		}
		if repoName, selector, ok := splitWorktreeToken(part, active); ok && repoName != "" && selector != "" {
			return true
		}
		if _, ok := m.Groups[part]; ok {
			return true
		}
	}

	return false
}

func hasFlag(flags []string, token string) bool {
	for _, flag := range flags {
		if flag == token {
			return true
		}
	}
	return false
}
