# Design notes

Short rationale for non-obvious CLI shape decisions. Keep entries brief — this
is a pointer, not a spec.

## Help layout

`ws help` groups commands under fixed categories (Inspect, Sync, Scope, Tools,
Install), one summary line per top-level command. Subcommand detail lives in
`ws <command> --help`, not in the top-level index.

Why: the flat list grew to ~60 lines as subcommands were added. Categories let
the top page stay ~30 lines and surface the verbs users reach for most; detail
is one `--help` away.

## Command naming

- Top-level command names should be semantically distinct. If two names read
  as near-synonyms but do different things, rename one and keep the old name
  as a back-compat alias rather than relying on users to remember the split.
- Subcommand convention: `list` is canonical, `ls` is an alias
  (`mux list|ls`, `agent list|ls`, `worktree list|ls`). Apply this to any new
  subcommand that lists things.

## Aliases in help

Aliases render parenthetically in the description column, e.g.
`context [subcommand]   Set/show the default filter (alias: ctx)`.

Tried inline `context|ctx` briefly — rejected because the pipe reads like a
regex/union operator and visually merges two distinct names.

## Optional vs required args

Use `[subcommand]` when a command has a default action (e.g. `ws worktree`
defaults to `list`, `ws context` shows the current context). Only use
`<subcommand>` when the command will error without one.

## Filter semantics

Comma-separated filters accept any mix of repo names, group names, and
activity tokens (`active:7d`, `mine:1w`). The help intentionally shows
`<repo>,<group>` to make that explicit — users often assume comma means
"groups only".

## Why not Cobra

The "unrecognized command passes through to git" model
(`ws git status`, `ws ai git log`, `ws -- fetch`) is fundamentally at odds
with Cobra's strict subcommand dispatch. The hand-rolled dispatch in
`cmd/ws/main.go` + `internal/command/commands.go` is small (~300 lines) and
specifically shaped around that passthrough + filter-token-as-command-prefix
UX. Cobra's wins (dispatch, per-command `--help`, completion) are already
covered; the migration would fight the library to preserve what makes `ws`
distinctive.

Revisit if the project grows a lot of external contributors or wants strict
POSIX flag conventions.
