package command

const (
	CommandHelp    = "help"
	CommandVersion = "version"
	CommandLL      = "ll"
	CommandCD      = "cd"
	CommandSetup   = "setup"
	CommandShell   = "shell"
	CommandOpen    = "open"
	CommandList    = "list"
	CommandFetch   = "fetch"
	CommandPull    = "pull"
	CommandAgent   = "agent"
	CommandContext  = "context"
	CommandDirs    = "dirs"
	CommandMux      = "mux"
	CommandWorktree = "worktree"
)

type builtinCommandAlias struct {
	Alias string
	Name  string
}

var builtinCommandAliases = []builtinCommandAlias{
	{Alias: "ctx", Name: CommandContext},
	{Alias: "wt", Name: CommandWorktree},
}

// HelpEntry is a single usage line plus its description.
type HelpEntry struct {
	Usage       string
	Description string
}

// BuiltinCommand describes one top-level built-in command.
type BuiltinCommand struct {
	Name         string
	ShowInUsage  bool
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
		ShowInUsage: true,
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
		ShowInUsage: true,
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
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "setup [filter]", Description: "Clone missing repos"},
		},
		complete: completeSetupCommand,
	},
	{
		Name:        CommandShell,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "shell init", Description: "Emit shell integration and completion"},
			{Usage: "shell install", Description: "Write shell config for ws cd and completion"},
		},
		complete: completeShellCommand,
	},
	{
		Name:        CommandOpen,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "open [--editor <name>]", Description: "Open workspace (default: code, or WS_EDITOR)"},
		},
		complete: completeNoopCommand,
	},
	{
		Name:        CommandList,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "list [--all]", Description: "Show repos in manifest (--all includes excluded)"},
		},
		complete: completeListCommand,
	},
	{
		Name:        CommandDirs,
		ShowInUsage: true,
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
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "fetch [filter]", Description: "Fetch all repos"},
		},
		complete: completeFetchCommand,
	},
	{
		Name:        CommandPull,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "pull [filter]", Description: "Pull repos in scope"},
		},
		complete: completeLLOrPullCommand,
	},
	{
		Name:        CommandContext,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "context [filter]", Description: "Set default filter (no arg = show, " + ContextClearUsage + " = clear)"},
			{Usage: "ctx [filter]", Description: "Alias for context"},
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
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "mux [session]", Description: "Attach or create a terminal session (tmux/zellij)"},
			{Usage: "mux kill [session]", Description: "Kill a session"},
			{Usage: "mux ls", Description: "List multiplexer sessions"},
			{Usage: "mux dup [window]", Description: "Duplicate a window/tab in the active session"},
			{Usage: "mux save [--local] [session]", Description: "Save session layout to manifest"},
		},
		DetailedHelp: `Usage: ws mux [subcommand] [options]

Manage terminal multiplexer sessions (tmux or zellij).

Subcommands:
  (no subcommand)            Attach or create the default session
  [session]                  Attach or create a named session
  kill [session]             Kill a session
  ls                         List active sessions
  dup [window]               Duplicate a window/tab in the active session
  save [--local] [session]   Persist current layout to manifest

Sessions are configured in manifest.yml under the mux: key.
`,
		complete: completeMuxCommand,
	},
	{
		Name:        CommandWorktree,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "worktree add <branch> [filter]", Description: "Create worktrees across repos"},
			{Usage: "worktree remove <branch> [filter]", Description: "Remove worktrees across repos"},
			{Usage: "worktree list [filter]", Description: "List worktrees per repo"},
			{Usage: "wt add <branch> [filter]", Description: "Alias for worktree"},
		},
		DetailedHelp: `Usage: ws worktree <subcommand> [options]

Manage git worktrees across repos.

Subcommands:
  add <branch> [filter]      Create linked worktrees for a branch
  remove <branch> [filter]   Remove linked worktrees
  list [filter]              List worktrees per repo

Alias: wt
`,
		complete: completeWorktreeCommand,
	},
	{
		Name:        CommandAgent,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "agent [--agent name] [repo] [-- args...]", Description: "Start an AI agent session"},
			{Usage: "agent ls [-v] [-n N | --all] [filter]", Description: "List agent sessions across workspace"},
			{Usage: "agent resume <#|id>", Description: "Resume a previous agent session"},
		},
		DetailedHelp: `Usage: ws agent [subcommand] [options]

Start, list, and resume AI agent sessions across workspace repos.

Subcommands:
  (default)                        Start an agent session
  ls [-v] [-n N | --all] [filter]  List sessions across workspace
  resume <#|id>                    Resume a previous session

Start options:
  --agent, -a <name>   Select an agent profile (default: $WS_AGENT or "claude")
  [repo]               Start in a repo directory (default: current dir)
  -- <args...>         Pass remaining args to the agent CLI

List options:
  -v, --verbose        Show recap, full prompts, and last message
  -n <N>               Limit output to N sessions (default: 20)
  --all                Show all sessions
  [filter]             Filter by group or repo name

Resume:
  <#>                  Numeric index from the most recent listing
  <id>                 Partial session ID prefix

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
	for _, alias := range builtinCommandAliases {
		names = append(names, alias.Alias)
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
	for _, alias := range builtinCommandAliases {
		if alias.Alias == name {
			return alias.Name
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
