package command

const (
	CommandHelp    = "help"
	CommandVersion = "version"
	CommandLL      = "ll"
	CommandCD      = "cd"
	CommandSetup   = "setup"
	CommandShell   = "shell"
	CommandOpen    = "open"
	CommandBrowse  = "browse"
	CommandRepos   = "repos"
	CommandFetch   = "fetch"
	CommandPull    = "pull"
	CommandAgent   = "agent"
	CommandContext  = "context"
	CommandDirs    = "dirs"
	CommandMux      = "mux"
	CommandWorktree = "worktree"
	CommandRemotes  = "remotes"
)

// Category groups related commands under one heading in `ws help`.
type Category string

const (
	CategoryInspect Category = "Inspect"
	CategorySync    Category = "Sync"
	CategoryScope   Category = "Scope"
	CategoryTools   Category = "Tools"
	CategoryInstall Category = "Install"
)

// categoryOrder is the order categories appear in `ws help`.
var categoryOrder = []Category{
	CategoryInspect,
	CategorySync,
	CategoryScope,
	CategoryTools,
	CategoryInstall,
}

// HelpEntry is a single usage line plus its description.
type HelpEntry struct {
	Usage       string
	Description string
}

// BuiltinCommand describes one top-level built-in command.
type BuiltinCommand struct {
	Name         string
	Aliases      []string // short or alternate names, rendered as name|alias in help
	Category     Category // grouping heading in `ws help`; empty hides the command
	ShowInUsage  bool
	Summary      HelpEntry // single-line summary for `ws help`; Usage is args only (no name)
	Help         []HelpEntry
	DetailedHelp string // full help shown by `ws <cmd> --help`
	complete     CompletionHandler
}

var builtinCommands = []BuiltinCommand{
	{
		Name:     CommandHelp,
		complete: completeNoopCommand,
	},
	{
		Name:     CommandVersion,
		complete: completeNoopCommand,
	},
	{
		Name:        CommandLL,
		Category:    CategoryInspect,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[filter]", Description: "Dashboard: branch, dirty, last commit"},
		Help: []HelpEntry{
			{Usage: "ll [filter]", Description: "Dashboard: branch, dirty, last commit"},
			{Usage: "ll [" + LLBranchesFlagUsage + "] [filter]", Description: "Show all local branches in ll format"},
		},
		DetailedHelp: `Usage: ws ll [options] [filter]

Show a dashboard of repo status: branch, dirty state, sync status,
and last commit message.

Options:
  -b, --branches     Show all local branches instead of just current
  -t, --worktrees    Expand filters to include linked worktrees
  --no-worktrees     Force primary checkouts only

Filters:
  <group>            Show repos in a named group
  <repo>             Show a single repo
  dirty              Repos with uncommitted changes
  active[:dur]       Recently active repos (default: 14d)
  mine:<dur>         Repos with your commits within duration
`,
		complete: completeLLCommand,
	},
	{
		Name:        CommandCD,
		Category:    CategoryInspect,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[repo[@worktree]]", Description: "Print/cd to a repo directory"},
		Help: []HelpEntry{
			{Usage: "cd [repo[@worktree]] [" + CDWorktreeFlagUsage + " <selector>]", Description: "Print repo path (no arg = workspace root)"},
		},
		DetailedHelp: `Usage: ws cd [repo[@worktree]] [-t <selector>]

Print the absolute path to a repo directory. With shell integration,
changes the working directory.

  ws cd                    Workspace root
  ws cd api-server         Repo directory
  ws cd api@feature        Worktree by name, branch, or path
  ws cd api -t feature     Same, using flag syntax
`,
		complete: completeCDCommand,
	},
	{
		Name:        CommandSetup,
		Category:    CategorySync,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[filter|all]", Description: "Clone missing repos"},
		Help: []HelpEntry{
			{Usage: "setup", Description: "Show a setup guide (or clone context repos if set)"},
			{Usage: "setup <filter>", Description: "Clone repos matching the filter"},
			{Usage: "setup all", Description: "Clone every active repo in the manifest"},
			{Usage: "setup [--all|-a]", Description: "Alias for `setup all`"},
		},
		DetailedHelp: `Usage: ws setup [filter|all]

Clone missing repos. By default:
  - With no filter and no context set: prints a setup guide listing
    available groups and suggested commands. Clones nothing.
  - With a context set: clones the repos in the context.
  - With a filter: clones repos matching the filter.

If a context is set and new repos are cloned, the scope symlinks and
VS Code workspace file are refreshed to include the new paths.

Forms:
  ws setup                  Show guide (or clone context if set)
  ws setup <repo>           Clone a single repo
  ws setup <group>          Clone all repos in a group
  ws setup all              Clone every active repo
  ws setup --all            Alias for ` + "`" + `setup all` + "`" + `

Note: activity filters (active, dirty, mine) need local git state, so
they return nothing on a fresh workspace.
`,
		complete: completeSetupCommand,
	},
	{
		Name:        CommandShell,
		Category:    CategoryInstall,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "init|install", Description: "Shell integration and completion"},
		Help: []HelpEntry{
			{Usage: "shell init", Description: "Emit shell integration and completion"},
			{Usage: "shell install", Description: "Write shell config for ws cd and completion"},
		},
		complete: completeShellCommand,
	},
	{
		Name:        CommandOpen,
		Category:    CategoryTools,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[--editor <name>]", Description: "Open the workspace in an editor"},
		Help: []HelpEntry{
			{Usage: "open [--editor <name>]", Description: "Open workspace (default: code, or WS_EDITOR)"},
		},
		complete: completeNoopCommand,
	},
	{
		Name:        CommandBrowse,
		Category:    CategoryTools,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "<repo> [-y]", Description: "Open a repo's remote URL in the default browser"},
		Help: []HelpEntry{
			{Usage: "browse <repo>", Description: "Open the repo's origin URL in a browser"},
			{Usage: "browse .", Description: "Open the URL of the repo containing the current directory"},
			{Usage: "browse <repo> [-y|--yes]", Description: "Skip the VS Code confirmation prompt"},
		},
		DetailedHelp: `Usage: ws browse <repo> [-y|--yes]

Resolves the given repo's origin URL from the manifest, converts it
to an https:// URL, prints it, and opens it in the default browser.

If the repo is not declared in the manifest, the URL is built from
the top-level remotes.origin prefix.

If the arg is "." the repo containing the current working directory
is used.

When run inside VS Code's integrated terminal (TERM_PROGRAM=vscode)
and stdin is a TTY, prompts before launching. Use -y to skip the
prompt.

Examples:
  ws browse api-server
  ws browse .
`,
		complete: completeBrowseCommand,
	},
	{
		Name:        CommandRepos,
		Aliases:     []string{"list"},
		Category:    CategoryInspect,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[--all]", Description: "Show repos in the manifest"},
		Help: []HelpEntry{
			{Usage: "repos [--all]", Description: "Show repos in manifest (--all includes excluded; alias: list)"},
		},
		complete: completeReposCommand,
	},
	{
		Name:        CommandDirs,
		Category:    CategoryInspect,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[filter]", Description: "Print repo paths (scripts/agents)"},
		Help: []HelpEntry{
			{Usage: "dirs [filter]", Description: "List repo directories (name and absolute path)"},
			{Usage: "dirs --root", Description: "Print the workspace root path"},
		},
		DetailedHelp: `Usage: ws dirs [options] [filter]

Print tab-separated repo name and absolute path pairs, one per line.
Only shows cloned repos. Useful for scripts and AI agents.

Options:
  --root               Print only the workspace root path
  -t, --worktrees      Include linked worktrees
  --no-worktrees       Exclude worktrees

Examples:
  ws dirs                    All cloned repos
  ws dirs backend            Repos in the backend group
  ws dirs --root             Workspace root path only
  ws -w /path dirs           From anywhere, targeting a workspace
`,
		complete: completeDirsCommand,
	},
	{
		Name:        CommandFetch,
		Category:    CategorySync,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[--remote <name>] [filter]", Description: "Fetch all remotes (or specific ones)"},
		Help: []HelpEntry{
			{Usage: "fetch [filter]", Description: "Fetch every configured remote per repo"},
			{Usage: "fetch --remote <name> [filter]", Description: "Fetch only the named remote; repeatable"},
		},
		DetailedHelp: `Usage: ws fetch [--remote <name>]... [filter]

By default, fetches every configured remote per repo (git fetch --all --prune).
With --remote, fetches only the named remotes; repos that don't declare a
requested remote are skipped with a warning. The flag can be given multiple
times to fetch a specific set of remotes.

Examples:
  ws fetch                            Fetch all remotes everywhere
  ws fetch backend                    Fetch all remotes in the backend group
  ws fetch --remote origin            Fetch only origin
  ws fetch --remote origin --remote upstream ai
`,
		complete: completeFetchCommand,
	},
	{
		Name:        CommandPull,
		Category:    CategorySync,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[filter]", Description: "Pull repos in scope"},
		Help: []HelpEntry{
			{Usage: "pull [filter]", Description: "Pull repos in scope"},
		},
		complete: completeLLOrPullCommand,
	},
	{
		Name:        CommandContext,
		Aliases:     []string{"ctx"},
		Category:    CategoryScope,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[subcommand]", Description: "Set/show the default filter"},
		Help: []HelpEntry{
			{Usage: "context [filter]", Description: "Set default filter (no arg = show, " + ContextClearUsage + " = clear)"},
			{Usage: "context set <filter>", Description: "Explicit form of context set"},
			{Usage: "context refresh [" + WorktreesFlagUsage + "]", Description: "Re-resolve the stored context and rebuild scope"},
			{Usage: "context . [" + WorktreesFlagUsage + "]", Description: "Shorthand for context refresh"},
			{Usage: "context -", Description: "Swap to the previous context (like cd -)"},
			{Usage: "context add <filter>", Description: "Add groups or repos to the existing context"},
			{Usage: "context remove <filter>", Description: "Remove groups or repos from the existing context"},
			{Usage: "context save [--local] <group>", Description: "Persist the current context as a named group"},
		},
		DetailedHelp: `Usage: ws context [subcommand] [options] [filter]

Manage the default filter scope. When set, commands like ll, pull, and
passthrough operate on the context repos by default.

Subcommands:
  (no subcommand)            Show current context
  set <filter>               Set the context (also: ws context <filter>)
  refresh                    Re-resolve and rebuild scope symlinks
  .                          Shorthand for refresh
  -                          Swap to the previous context (like cd -)
  add <filter>               Add groups/repos to existing context
  remove <filter>            Remove groups/repos from context
  save [--local] <group>     Save context as a named group in manifest
  none, reset                Clear the context

Options:
  -t, --worktrees            Include worktrees when resolving
  --no-worktrees             Exclude worktrees

Alias: ctx
`,
		complete: completeContextCommand,
	},
	{
		Name:        CommandMux,
		Category:    CategoryTools,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[subcommand]", Description: "Terminal multiplexer sessions"},
		Help: []HelpEntry{
			{Usage: "mux [session]", Description: "Attach or create a terminal session (tmux/zellij)"},
			{Usage: "mux kill [session]", Description: "Kill a session"},
			{Usage: "mux list", Description: "List multiplexer sessions (alias: ls)"},
			{Usage: "mux dup [window]", Description: "Duplicate a window/tab in the active session"},
			{Usage: "mux save [--local] [session]", Description: "Save session layout to manifest"},
		},
		DetailedHelp: `Usage: ws mux [subcommand] [options]

Manage terminal multiplexer sessions (tmux or zellij).

Subcommands:
  (no subcommand)            Attach or create the default session
  [session]                  Attach or create a named session
  kill [session]             Kill a session
  list|ls                    List active sessions
  dup [window]               Duplicate a window/tab in the active session
  save [--local] [session]   Persist current layout to manifest

Sessions are configured in manifest.yml under the mux: key.
`,
		complete: completeMuxCommand,
	},
	{
		Name:        CommandWorktree,
		Aliases:     []string{"wt"},
		Category:    CategoryScope,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[subcommand]", Description: "Manage linked git worktrees"},
		Help: []HelpEntry{
			{Usage: "worktree add <branch> [filter]", Description: "Create worktrees across repos"},
			{Usage: "worktree remove <branch> [filter]", Description: "Remove worktrees across repos"},
			{Usage: "worktree list [filter]", Description: "List worktrees per repo"},
		},
		DetailedHelp: `Usage: ws worktree [subcommand] [options]

Manage git worktrees across repos.

Subcommands:
  add <branch> [filter]      Create linked worktrees for a branch
  remove|rm <branch> [filter] Remove linked worktrees
  list|ls [filter]           List worktrees per repo

Alias: wt
`,
		complete: completeWorktreeCommand,
	},
	{
		Name:        CommandRemotes,
		Category:    CategorySync,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "sync [filter]", Description: "Reconcile declared remotes against checkouts"},
		Help: []HelpEntry{
			{Usage: "remotes sync [filter]", Description: "Add missing remotes on disk; warn on URL drift"},
		},
		DetailedHelp: `Usage: ws remotes sync [filter]

Reconcile the remotes declared in manifest.yml against what's on disk.
For each repo in the filter:
  - missing remote      added via git remote add
  - URL differs         warn with current vs manifest, leave unchanged
  - unknown on-disk     ignored (may be a user-added remote)

Never removes or renames remotes. Use when the manifest gains new remotes
after a repo was already cloned.
`,
		complete: completeNoopCommand,
	},
	{
		Name:        CommandAgent,
		Category:    CategoryTools,
		ShowInUsage: true,
		Summary:     HelpEntry{Usage: "[subcommand]", Description: "AI agent sessions (start/list/resume)"},
		Help: []HelpEntry{
			{Usage: "agent [--agent name] [repo] [-- args...]", Description: "Start an AI agent session"},
			{Usage: "agent list [-v] [-l|-r] [-n N | --all] [filter]", Description: "List agent sessions (alias: ls)"},
			{Usage: "agent resume <#|id>", Description: "Resume a previous agent session"},
			{Usage: "agent pin [<#|id>]", Description: "Pin a session (no arg = current)"},
			{Usage: "agent unpin [<#|id>]", Description: "Unpin a session (no arg = current)"},
		},
		DetailedHelp: `Usage: ws agent [subcommand] [options]

Start, list, and resume AI agent sessions across workspace repos.

Subcommands:
  (default)                          Start an agent session
  list|ls [-v] [-l|-r] [-n N|--all] [filter]
                                     List sessions
  resume <#|id>                      Resume a previous session
  pin [<#|id>]                       Pin a session (no arg = current)
  unpin [<#|id>]                     Unpin a session (no arg = current)

Start options:
  --agent, -a <name>   Select an agent profile (default: $WS_AGENT or "claude")
  [repo]               Start in a repo directory (default: current dir)
  -- <args...>         Pass remaining args to the agent CLI

List options:
  -v, --verbose        Show recap, full prompts, and last message
  -l, --last           Show last user prompt instead of first (compact view)
  -r, --recap          Show recap, falling back to last/first (compact view)
  -n <N>               Limit output to N sessions (default: 20)
  --all                Show all sessions
  [filter]             Filter by group or repo name
                       Special values:
                         .         Current directory's repo (or root)
                         root      Only sessions started in the workspace root
                         external  Sessions started outside this workspace

Resume / pin / unpin:
  <#>                  Numeric index from the most recent listing
  <id>                 Partial session ID prefix
  (pin/unpin, no arg)  Target the session the shell is running inside

Pinned sessions sort to the top of ` + "`ws agent ls`" + ` (marked P) and are
kept visible even when the output is limited by -n. Pins are stored in
` + "`.ws/agent-pins.yml`" + ` at the workspace root.

Agent profiles are configured in manifest (typically manifest.local.yml):

  agents:
    default: claude
    claude: claude
    cc: IS_SANDBOX=1 claude --dangerously-skip-permissions
    codex: codex --yolo

Resume automatically detects --dangerously-skip-permissions from the
original session and includes it in the resume command.

Environment:
  WS_AGENT             Default agent profile name (overrides agents.default)
`,
		complete: completeAgentCommand,
	},
}

// BuiltinCommands returns the registered top-level command metadata.
func BuiltinCommands() []BuiltinCommand {
	out := make([]BuiltinCommand, 0, len(builtinCommands))
	for _, cmd := range builtinCommands {
		help := append([]HelpEntry(nil), cmd.Help...)
		out = append(out, BuiltinCommand{
			Name:        cmd.Name,
			ShowInUsage: cmd.ShowInUsage,
			Help:        help,
		})
	}
	return out
}

// BuiltinCommandNames returns the names of all top-level built-in commands.
func BuiltinCommandNames() []string {
	names := make([]string, 0, len(builtinCommands))
	for _, cmd := range builtinCommands {
		names = append(names, cmd.Name)
	}
	return names
}

// BuiltinCommandSuggestions returns canonical command names plus shorthand aliases.
func BuiltinCommandSuggestions() []string {
	names := BuiltinCommandNames()
	for _, cmd := range builtinCommands {
		names = append(names, cmd.Aliases...)
	}
	return names
}

// BuiltinUsageEntries returns the help entries shown in `ws help`.
func BuiltinUsageEntries() []HelpEntry {
	var entries []HelpEntry
	for _, cmd := range builtinCommands {
		if !cmd.ShowInUsage {
			continue
		}
		entries = append(entries, cmd.Help...)
	}
	return entries
}

// ResolveBuiltinCommandName maps a shorthand alias to its canonical built-in name.
func ResolveBuiltinCommandName(name string) string {
	for _, cmd := range builtinCommands {
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd.Name
			}
		}
	}
	return name
}

func builtinCommandByName(name string) (BuiltinCommand, bool) {
	name = ResolveBuiltinCommandName(name)
	for _, cmd := range builtinCommands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return BuiltinCommand{}, false
}
