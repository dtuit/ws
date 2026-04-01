package command

import (
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

var builtInCommands = []string{
	"init",
	"help",
	"version",
	"ll",
	"cd",
	"setup",
	"code",
	"list",
	"fetch",
	"pull",
	"context",
}

// CompletionResult describes completion candidates and any shell fallback.
type CompletionResult struct {
	Values           []string
	FallbackCommands bool
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
		values := append(globalFlagSuggestions(), builtInCommands...)
		values = append(values, filterSuggestions(m)...)
		return finalizeCompletion(values, currentWord, true)
	}

	cmd := words[commandIndex]
	args := words[commandIndex+1:]
	argIndex := current - commandIndex - 1

	switch cmd {
	case "help", "version", "init":
		return CompletionResult{}
	case "cd":
		if argIndex == 0 {
			return finalizeCompletion(repoSuggestions(m), currentWord, false)
		}
		return CompletionResult{}
	case "list":
		return finalizeCompletion([]string{"--all", "-a"}, currentWord, false)
	case "setup":
		values := append([]string{"--install-shell"}, filterSuggestions(m)...)
		return finalizeCompletion(values, currentWord, false)
	case "code":
		return completeCode(m, args, argIndex)
	case "ll", "fetch", "pull":
		if argIndex == 0 {
			return finalizeCompletion(filterSuggestions(m), currentWord, false)
		}
		return CompletionResult{}
	case "context":
		if argIndex == 0 {
			values := append(filterSuggestions(m), "none", "reset", "add")
			return finalizeCompletion(values, currentWord, false)
		}
		if argIndex == 1 && args[0] == "add" {
			return finalizeCompletion(filterSuggestions(m), currentWord, false)
		}
		return CompletionResult{}
	case "--":
		return completePassthrough(m, args, argIndex)
	default:
		return completeDefaultPassthrough(m, append([]string{cmd}, args...), current-commandIndex)
	}
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
	return CompletionResult{}
}

func completeCode(m *manifest.Manifest, args []string, current int) CompletionResult {
	codeFlags := []string{"-t", "--worktrees"}
	if current < 0 {
		return CompletionResult{}
	}
	if current == 0 {
		values := append(codeFlags, filterSuggestions(m)...)
		return finalizeCompletion(values, args[0], false)
	}

	seenWorktreesFlag := false
	filterIndex := 0
	for i, arg := range args {
		if arg == "-t" || arg == "--worktrees" {
			if i == current {
				return finalizeCompletion(codeFlags, args[current], false)
			}
			seenWorktreesFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return CompletionResult{}
		}
		if i == current {
			if filterIndex == 0 {
				return finalizeCompletion(filterSuggestions(m), args[current], false)
			}
			return CompletionResult{}
		}
		filterIndex++
	}

	if seenWorktreesFlag && current == len(args) {
		return finalizeCompletion(filterSuggestions(m), "", false)
	}

	return CompletionResult{}
}

func completePassthrough(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	if current == 0 {
		return finalizeCompletion(filterSuggestions(m), args[0], true)
	}
	if len(args) > 0 && isFilterToken(m, args[0]) && current == 1 {
		return CompletionResult{FallbackCommands: true}
	}
	return CompletionResult{}
}

func firstCommandIndex(words []string) int {
	for i := 0; i < len(words); i++ {
		switch words[i] {
		case "-w", "--workspace":
			if i+1 >= len(words) {
				return len(words)
			}
			i++
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
	return []string{"-w", "--workspace", "-h", "--help", "--version"}
}

func filterSuggestions(m *manifest.Manifest) []string {
	values := []string{"all"}
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
	if token == "all" {
		return true
	}
	if m == nil {
		return false
	}
	return m.IsGroupOrRepo(token)
}
