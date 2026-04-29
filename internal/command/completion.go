package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

// CompletionResult describes completion candidates and any shell fallback.
//
// Values is the authoritative list of candidates, prefix-filtered against the
// current word. Groups and Descriptions are optional metadata keyed by value:
// when the consuming shell can render groups (zsh via _describe) we surface
// them; bash ignores both and just shows values.
type CompletionResult struct {
	Values           []string
	Groups           map[string]string // value -> group label (e.g. "flags", "repos")
	Descriptions     map[string]string // value -> short help text shown next to value
	FallbackCommands bool
	DelegateCommands bool
	DelegateStart    int
}

// Standard group labels used by the migrated handlers. Free-form strings work
// too; these constants just keep ordering consistent across commands.
const (
	GroupSubcommands = "subcommands"
	GroupFlags       = "flags"
	GroupFilters     = "filter tokens"
	GroupGroups      = "groups"
	GroupRepos       = "repos"
	GroupRemotes     = "remotes"
)

// CompletionHandler returns shell completion candidates for one built-in command.
type CompletionHandler func(m *manifest.Manifest, args []string, current int) CompletionResult

// CompletionOutput renders completion lines, including shell delegation markers.
//
// Output protocol:
//   - Single sentinel line for delegate/fallback (unchanged).
//   - Otherwise one line per candidate, tab-separated:
//     <group>\t<value>\t<description>
//     Empty group/description fields are allowed. Bash strips to <value>;
//     zsh groups by <group> and shows <description> via _describe.
//
// Lines are emitted in (groupPriority, value) order so the zsh script's
// "discover groups in order of first appearance" rule produces a stable
// section ordering that puts the most useful sections first.
func CompletionOutput(m *manifest.Manifest, words []string, current int) []string {
	result := Complete(m, words, current)
	if result.DelegateCommands {
		return []string{fmt.Sprintf("%s:%d", CompletionCommandFallbackSentinel, result.DelegateStart)}
	}
	if result.FallbackCommands {
		return []string{CompletionCommandFallbackSentinel}
	}

	type row struct {
		group, value, desc string
		priority           int
	}
	rows := make([]row, 0, len(result.Values))
	for _, v := range result.Values {
		group := ""
		desc := ""
		if result.Groups != nil {
			group = result.Groups[v]
		}
		if result.Descriptions != nil {
			desc = result.Descriptions[v]
		}
		rows = append(rows, row{
			group:    group,
			value:    v,
			desc:     desc,
			priority: groupPriority(group),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].priority != rows[j].priority {
			return rows[i].priority < rows[j].priority
		}
		return rows[i].value < rows[j].value
	})

	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s\t%s\t%s", r.group, r.value, r.desc))
	}
	return out
}

// groupPriority returns the display rank for a group label. Lower numbers
// surface first in the completion menu. Unknown labels sort after the
// known ones (in alphabetic order via the value tiebreaker).
func groupPriority(group string) int {
	switch group {
	case GroupFlags:
		return 0
	case GroupSubcommands:
		return 1
	case GroupFilters:
		return 2
	case GroupGroups:
		return 3
	case GroupRemotes:
		return 4
	case GroupRepos:
		return 5
	case "":
		return 99
	default:
		return 50
	}
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
		flags := globalFlagSuggestions()
		commands := BuiltinCommandSuggestions()
		filters, fGroups, fDescs := filterSuggestionsWithMeta(m)

		values := append([]string{}, flags...)
		values = append(values, commands...)
		values = append(values, filters...)

		groups := map[string]string{}
		descriptions := map[string]string{}
		for _, f := range flags {
			groups[f] = GroupFlags
		}
		descriptions["-w"] = "Workspace dir override"
		descriptions["--workspace"] = "Workspace dir override"
		descriptions["-t"] = "Include linked worktrees"
		descriptions["--worktrees"] = "Include linked worktrees"
		descriptions["--no-worktrees"] = "Force primary checkouts only"
		descriptions["-h"] = "Show usage"
		descriptions["--help"] = "Show usage"
		descriptions["--version"] = "Print ws version"

		for _, c := range commands {
			groups[c] = GroupSubcommands
			if entry, ok := builtinCommandByName(c); ok && entry.Summary.Description != "" {
				descriptions[c] = entry.Summary.Description
			}
		}
		for k, v := range fGroups {
			groups[k] = v
		}
		for k, v := range fDescs {
			descriptions[k] = v
		}
		return withMetadata(finalizeCompletion(values, currentWord, true), groups, descriptions)
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
		return reposOnlySuggestion(m, currentWord)
	case 1:
		flags := []string{"--worktree", "-t"}
		return withMetadata(
			finalizeCompletion(flags, currentWord, false),
			map[string]string{"--worktree": GroupFlags, "-t": GroupFlags},
			map[string]string{"--worktree": "Select a worktree by name/branch", "-t": "Select a worktree by name/branch"},
		)
	default:
		return CompletionResult{}
	}
}

func completeReposCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	flags := append([]string{"--all", "-a"}, worktreesFlagSuggestions()...)
	groups := map[string]string{}
	for _, f := range flags {
		groups[f] = GroupFlags
	}
	descs := map[string]string{
		"--all":          "Include excluded repos",
		"-a":             "Include excluded repos",
		"-t":             "Include linked worktrees",
		"--worktrees":    "Include linked worktrees",
		"--no-worktrees": "Force primary checkouts only",
	}
	return withMetadata(finalizeCompletion(flags, completionWord(args, current), false), groups, descs)
}

func completeBrowseCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	currentWord := completionWord(args, current)
	if current == 0 {
		flags := []string{".", "--yes", "-y"}
		repos := repoSuggestions(m)
		values := append(append([]string{}, flags...), repos...)
		groups := map[string]string{
			".":      GroupFilters,
			"--yes":  GroupFlags,
			"-y":     GroupFlags,
		}
		descs := map[string]string{
			".":     "Current directory's repo",
			"--yes": "Skip confirmation prompt",
			"-y":    "Skip confirmation prompt",
		}
		for _, r := range repos {
			groups[r] = GroupRepos
		}
		return withMetadata(finalizeCompletion(values, currentWord, false), groups, descs)
	}
	return withMetadata(
		finalizeCompletion([]string{"--yes", "-y"}, currentWord, false),
		map[string]string{"--yes": GroupFlags, "-y": GroupFlags},
		map[string]string{"--yes": "Skip confirmation prompt", "-y": "Skip confirmation prompt"},
	)
}

// reposOnlySuggestion returns just the repo list with the GroupRepos label.
func reposOnlySuggestion(m *manifest.Manifest, currentWord string) CompletionResult {
	repos := repoSuggestions(m)
	groups := map[string]string{}
	for _, r := range repos {
		groups[r] = GroupRepos
	}
	return withMetadata(finalizeCompletion(repos, currentWord, false), groups, nil)
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
	if current != 0 {
		return CompletionResult{}
	}
	flags := []string{"--all", "-a"}
	currentWord := completionWord(args, current)
	result := filterAndFlagsSuggestion(m, flags, currentWord)
	if _, ok := indexValues(result.Values, "--all"); ok {
		if result.Descriptions == nil {
			result.Descriptions = map[string]string{}
		}
		result.Descriptions["--all"] = "Clone every active repo"
		result.Descriptions["-a"] = "Clone every active repo"
	}
	return result
}

func completeFetchCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	currentWord := completionWord(args, current)

	// Position immediately after a `--remote` token: complete the remote name.
	if current > 0 && args[current-1] == "--remote" {
		names := remoteNameSuggestions(m)
		groups := map[string]string{}
		for _, n := range names {
			groups[n] = GroupRemotes
		}
		return withMetadata(finalizeCompletion(names, currentWord, false), groups, nil)
	}

	// --remote flag + filter tokens. --remote can repeat so it's valid at any
	// later position too.
	flags := []string{"--remote"}
	descs := map[string]string{"--remote": "Fetch a specific remote (repeatable)"}
	result := filterAndFlagsSuggestion(m, flags, currentWord)
	for v, d := range descs {
		if result.Descriptions == nil {
			result.Descriptions = map[string]string{}
		}
		if _, ok := indexValues(result.Values, v); ok {
			result.Descriptions[v] = d
		}
	}
	return result
}

// indexValues reports whether v is in values (linear scan; values are short).
func indexValues(values []string, v string) (int, bool) {
	for i, item := range values {
		if item == v {
			return i, true
		}
	}
	return 0, false
}

// remoteNameSuggestions returns the union of remote names declared across all
// active repos in the manifest (e.g. origin, upstream, …).
func remoteNameSuggestions(m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for name, cfg := range m.ActiveRepos() {
		for k := range m.ResolveRemotes(name, cfg) {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
		return filterAndFlagsSuggestion(m, flags, completionWord(args, current))
	}

	seenFlag := false
	filterIndex := 0
	for i, arg := range args {
		if hasFlag(flags, arg) {
			if i == current {
				return flagsOnlySuggestion(flags, completionWord(args, current))
			}
			seenFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return CompletionResult{}
		}
		if i == current {
			if filterIndex == 0 {
				return filterOnlySuggestion(m, completionWord(args, current))
			}
			return CompletionResult{}
		}
		filterIndex++
	}

	if seenFlag && current == len(args) {
		return filterOnlySuggestion(m, "")
	}

	return CompletionResult{}
}

// filterAndFlagsSuggestion combines filter tokens (with group metadata) and
// the given flag list.
func filterAndFlagsSuggestion(m *manifest.Manifest, flags []string, currentWord string) CompletionResult {
	filters, fGroups, fDescs := filterSuggestionsWithMeta(m)
	values := append([]string{}, flags...)
	values = append(values, filters...)
	groups := map[string]string{}
	for _, f := range flags {
		groups[f] = GroupFlags
	}
	for k, v := range fGroups {
		groups[k] = v
	}
	return withMetadata(finalizeCompletion(values, currentWord, false), groups, fDescs)
}

func flagsOnlySuggestion(flags []string, currentWord string) CompletionResult {
	groups := map[string]string{}
	for _, f := range flags {
		groups[f] = GroupFlags
	}
	return withMetadata(finalizeCompletion(flags, currentWord, false), groups, nil)
}

func filterOnlySuggestion(m *manifest.Manifest, currentWord string) CompletionResult {
	values, groups, descs := filterSuggestionsWithMeta(m)
	return withMetadata(finalizeCompletion(values, currentWord, false), groups, descs)
}

func completeShellCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	if current < 0 {
		return CompletionResult{}
	}
	if current == 0 {
		return withMetadata(
			finalizeCompletion([]string{"init", "install"}, completionWord(args, current), false),
			map[string]string{"init": GroupSubcommands, "install": GroupSubcommands},
			map[string]string{
				"init":    "Emit shell integration to stdout",
				"install": "Write shell integration into ~/.bashrc or ~/.zshrc",
			},
		)
	}
	return CompletionResult{}
}

func completeUpgradeCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	if current != 0 {
		return CompletionResult{}
	}
	return withMetadata(
		finalizeCompletion([]string{"--check"}, completionWord(args, current), false),
		map[string]string{"--check": GroupFlags},
		map[string]string{"--check": "Compare against latest release"},
	)
}

func completeRemotesCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		return withMetadata(
			finalizeCompletion([]string{"sync"}, completionWord(args, current), false),
			map[string]string{"sync": GroupSubcommands},
			map[string]string{"sync": "Reconcile manifest remotes against checkouts"},
		)
	}
	if len(args) > 0 && args[0] == "sync" && current == 1 {
		return filterOnlySuggestion(m, completionWord(args, current))
	}
	return CompletionResult{}
}

func completeRepairRefspecsCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		return filterOnlySuggestion(m, completionWord(args, current))
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

	subcommands := []struct{ name, desc string }{
		{"set", "Set the default filter scope"},
		{"add", "Extend the current context"},
		{"remove", "Remove repos from the current context"},
		{"refresh", "Re-resolve and rebuild scope symlinks"},
		{".", "Shorthand for refresh"},
		{"save", "Persist the current context as a named group"},
		{"none", "Clear the context"},
		{"reset", "Clear the context"},
		{"-", "Swap to the previous context"},
		{"prev", "Swap to the previous context"},
	}

	if len(nonFlags) == 0 {
		filterValues, fGroups, fDescs := filterSuggestionsWithMeta(m)
		values := append([]string{}, flags...)
		for _, sc := range subcommands {
			values = append(values, sc.name)
		}
		values = append(values, filterValues...)

		groups := map[string]string{}
		descriptions := map[string]string{}
		for _, f := range flags {
			groups[f] = GroupFlags
		}
		descriptions["-t"] = "Include linked worktrees"
		descriptions["--worktrees"] = "Include linked worktrees"
		descriptions["--no-worktrees"] = "Force primary checkouts only"
		for _, sc := range subcommands {
			groups[sc.name] = GroupSubcommands
			descriptions[sc.name] = sc.desc
		}
		for k, v := range fGroups {
			groups[k] = v
		}
		for k, v := range fDescs {
			descriptions[k] = v
		}
		return withMetadata(finalizeCompletion(values, currentWord, false), groups, descriptions)
	}

	if nonFlags[0] == "save" {
		if seenLocal {
			return CompletionResult{}
		}
		return withMetadata(
			finalizeCompletion([]string{"--local"}, currentWord, false),
			map[string]string{"--local": GroupFlags},
			map[string]string{"--local": "Save into manifest.local.yml"},
		)
	}

	if nonFlags[0] == "add" || nonFlags[0] == "remove" {
		return filterAndFlagsSuggestion(m, flags, currentWord)
	}
	if nonFlags[0] == "refresh" || nonFlags[0] == "." {
		return flagsOnlySuggestion(flags, currentWord)
	}

	return filterAndFlagsSuggestion(m, flags, currentWord)
}

func completeMuxCommand(_ *manifest.Manifest, args []string, current int) CompletionResult {
	if current != 0 {
		return CompletionResult{}
	}
	subs := []string{"dup", "kill", "list", "ls", "save"}
	descs := map[string]string{
		"dup":  "Duplicate a window/tab in the active session",
		"kill": "Kill a session",
		"list": "List multiplexer sessions",
		"ls":   "List multiplexer sessions",
		"save": "Persist current layout to manifest",
	}
	groups := map[string]string{}
	for _, s := range subs {
		groups[s] = GroupSubcommands
	}
	return withMetadata(finalizeCompletion(subs, completionWord(args, current), false), groups, descs)
}

func completeWorktreeCommand(m *manifest.Manifest, args []string, current int) CompletionResult {
	if current == 0 {
		subs := []string{"add", "remove", "list", "ls"}
		descs := map[string]string{
			"add":    "Create linked worktrees for a branch",
			"remove": "Remove linked worktrees",
			"list":   "List worktrees per repo",
			"ls":     "List worktrees per repo",
		}
		groups := map[string]string{}
		for _, s := range subs {
			groups[s] = GroupSubcommands
		}
		return withMetadata(finalizeCompletion(subs, completionWord(args, current), false), groups, descs)
	}
	if current >= 2 && len(args) > 0 {
		action := args[0]
		if action == "add" || action == "remove" || action == "list" || action == "ls" {
			return filterOnlySuggestion(m, completionWord(args, current))
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

// withMetadata is a helper for handlers that want to attach group/description
// metadata to a result built via finalizeCompletion. It only retains entries
// whose value survived prefix filtering.
func withMetadata(r CompletionResult, groups, descriptions map[string]string) CompletionResult {
	if r.FallbackCommands || r.DelegateCommands || len(r.Values) == 0 {
		return r
	}
	keep := make(map[string]struct{}, len(r.Values))
	for _, v := range r.Values {
		keep[v] = struct{}{}
	}
	if groups != nil {
		r.Groups = make(map[string]string, len(r.Values))
		for v, g := range groups {
			if _, ok := keep[v]; ok && g != "" {
				r.Groups[v] = g
			}
		}
	}
	if descriptions != nil {
		r.Descriptions = make(map[string]string, len(r.Values))
		for v, d := range descriptions {
			if _, ok := keep[v]; ok && d != "" {
				r.Descriptions[v] = d
			}
		}
	}
	return r
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
	values, _, _ := filterSuggestionsWithMeta(m)
	return values
}

// filterSuggestionsWithMeta returns the filter suggestion list plus
// group/description maps suitable for `withMetadata`.
//
// Groups are emitted as "filter tokens" (the activity-style filters), "groups"
// (manifest groups), and "repos". Descriptions for groups list the first few
// member repos so the user can see what's behind a group name.
func filterSuggestionsWithMeta(m *manifest.Manifest) (values []string, groups, descriptions map[string]string) {
	groups = map[string]string{}
	descriptions = map[string]string{}

	tokens := []struct{ value, desc string }{
		{"all", "Every active repo"},
		{activeFilterToken, "Recently active repos (default 14d)"},
		{activeFilterToken + ":1d", "Active within last day"},
		{dirtyFilterToken, "Repos with uncommitted changes"},
		{mineFilterToken + ":1d", "Repos with your commits within 1 day"},
	}
	for _, t := range tokens {
		values = append(values, t.value)
		groups[t.value] = GroupFilters
		descriptions[t.value] = t.desc
	}
	if m == nil {
		return values, groups, descriptions
	}

	groupNames := make([]string, 0, len(m.Groups))
	for g := range m.Groups {
		groupNames = append(groupNames, g)
	}
	sort.Strings(groupNames)
	for _, g := range groupNames {
		values = append(values, g)
		groups[g] = GroupGroups
		descriptions[g] = describeGroupMembers(m.Groups[g])
	}

	repoNames := make([]string, 0, len(m.ActiveRepos()))
	for name := range m.ActiveRepos() {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)
	for _, r := range repoNames {
		values = append(values, r)
		groups[r] = GroupRepos
	}
	return values, groups, descriptions
}

// describeGroupMembers builds a short "a, b, c, +N more" summary so users
// can tell groups apart in completion menus.
func describeGroupMembers(members []string) string {
	if len(members) == 0 {
		return ""
	}
	const max = 3
	if len(members) <= max {
		return strings.Join(members, ", ")
	}
	return strings.Join(members[:max], ", ") + fmt.Sprintf(", +%d more", len(members)-max)
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
