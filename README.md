# ws

`ws` turns a repo containing `manifest.yml` into the control plane for a multi-repo workspace.

At a glance:

- `manifest.yml` declares which repos exist, where they clone, and how they are grouped
- `ws setup` clones missing repos into the configured `root`
- `ws context` regenerates a VS Code workspace and configured scope symlink trees
- `ws <command...>` fans shell commands out across the selected repos

The recommended model is simple:

- create a dedicated repo for your workspace manifest
- use that repo as the root workspace
- clone and operate on the managed repos from there
- use filters and context to scope editor state, operations, and agent-visible paths

`ws` reads `manifest.yml`, clones missing repos, fans commands out across repos, generates a VS Code workspace, and maintains configured scope symlink trees for filesystem-based agents. By default that is a single `.scope/` tree.

## Install

Fast install from release artifacts:

```bash
curl -LsSf https://raw.githubusercontent.com/dtuit/ws/main/install.sh | sh
```

Pinned install:

```bash
curl -LsSf https://raw.githubusercontent.com/dtuit/ws/main/install.sh | sh -s -- --version v0.1.0
```

The installer downloads the matching GitHub Release artifact, verifies its
checksum, and installs `ws` into `~/.local/bin` by default, or `/usr/local/bin`
when run as root. Override that with `WS_INSTALL_DIR=/some/path`.

Install from source with Go:

```bash
go install github.com/dtuit/ws/cmd/ws@latest
```

That install path now relies on Go's embedded module metadata, so `ws version`
reports the resolved module version instead of always falling back to `dev`.

Or build from source:

```bash
git clone git@github.com:dtuit/ws.git
cd ws
make install
```

GitHub Actions builds per-platform artifacts for pushes, pull requests, and tags.
Tag builds stamp the binaries with the tag version, publish a GitHub Release,
and keep downloaded artifacts aligned with `ws version`.

## Requirements

- `git` is required for cloning and repo operations
- the VS Code `code` command is required for `ws open`
- `bash` or `zsh` is required for `ws shell init` and tab completion
- Go is only required when installing or building from source
- release artifacts are published for Linux and macOS on `amd64` and `arm64`

## Workspace Repo Model

Create a repo whose job is to describe and manage your working set.

```text
code/
├── acme-workspace/
│   ├── manifest.yml
│   ├── manifest.local.yml      # optional, ignored
│   ├── .ws/                    # generated state (context, agent pins), ignored
│   ├── .scope/                 # generated symlinks, ignored (default)
│   └── ws.code-workspace       # generated, ignored
├── api-server/
├── auth-service/
└── web-app/
```

`root` is required in `manifest.yml`; there is no implicit default.
The recommended layout is `root: ..`, which keeps the workspace repo separate from the managed repos while still letting `.scope/` act as the default agent-facing tree.
If you prefer a self-contained tree, use `root: repos` instead.

A typical `.gitignore` for the recommended sibling-checkout layout:

```gitignore
.scope/
.ws/
*.code-workspace
manifest.local.yml
```

If you customize `scopes:`, add those generated directories too.

If you use `root: repos`, add `repos/` too.

Commit the control-plane files: `manifest.yml`, shared docs, and any shared scripts.
Keep local overrides, generated workspace files, and cloned repos out of git.

## Bootstrap A Workspace Repo

```bash
mkdir acme-workspace
cd acme-workspace
git init

cat > .gitignore <<'EOF'
.scope/
.ws/
*.code-workspace
manifest.local.yml
EOF

cat > manifest.yml <<'EOF'
root: ..

remotes:
  origin: git@github.com:acme-corp

branch: main

groups:
  backend: [api-server, auth-service, worker]
  frontend: [web-app]

repos:
  api-server:
  auth-service:
  worker:
  web-app:
EOF

ws setup
git add .gitignore manifest.yml
git commit -m "Bootstrap workspace"
```

See [example/README.md](example/README.md) for a runnable self-contained sample workspace that uses `root: repos`.

## Quick Start

1. Create a repo for the workspace manifest.
2. Add `manifest.yml` with an explicit `root`.
3. Run `ws` from inside that repo.

Example `manifest.yml`:

```yaml
root: ..

remotes:
  origin: git@github.com:acme-corp

branch: main
workspace: ws.code-workspace
scopes:
  - dir: .scope
    source: context

groups:
  backend: [api-server, auth-service, worker]
  frontend: [web-app, admin-dashboard]
  ops: [deploy-configs]

repos:
  api-server:
  auth-service: { branch: develop }
  worker:
  web-app:
  admin-dashboard:
  deploy-configs:
```

Then:

```bash
ws setup
ws ll
ws backend git status
ws context backend
ws open
```

If you want shell integration for `ws cd` and tab completion, either add this to your shell config:

```bash
export WS_HOME=/path/to/acme-workspace
eval "$(ws shell init)"
```

or let `ws` write it for you:

```bash
ws shell install
```

With that loaded, `ws` completes built-in commands, filters, repo names, and falls back to your shell's command completion for fan-out commands like `ws backend git ...`.

## Common Tasks

| Task | Command |
|---|---|
| Clone any missing repos | `ws setup` |
| See repo status across the current scope | `ws ll` |
| See all local branches in ll format | `ws ll --branches` |
| Narrow the workspace to a group | `ws context backend` |
| Snapshot active work into the context | `ws context active` |
| Add one more repo to the current scope | `ws context add web-app` |
| Re-resolve the saved context | `ws context refresh` |
| Run a command across a group | `ws backend git status` |
| Open the generated VS Code workspace | `ws open` |
| Open a repo's remote URL in the browser | `ws browse api-server` |
| Print the path to a repo or worktree | `ws cd api-server` |

`ws open` only works after a workspace file exists, so run `ws context <filter>` first if you have not generated one yet.

## Daily Workflow

Operate from the workspace repo. `ws` finds the workspace by:

1. `-w` / `--workspace`
2. `WS_HOME`
3. walking up from the current directory until it finds `manifest.yml`

Core commands:

```text
ws ll [filter]
                          Dashboard: branch, dirty state, last commit
ws ll [--branches|-b] [filter]
                          Show all local branches in ll format
ws repos [--all]
                          Show repos in the manifest (--all includes excluded; alias: list)
ws setup [filter]         Clone missing repos
ws shell init             Emit shell integration and completion
ws shell install          Write shell config for ws cd and completion
ws fetch [filter]         Fetch all repos in scope
ws pull [filter]          Pull repos in scope
ws context [filter]
                          Set or show the default filter (none/reset clears)
ws ctx [filter]           Alias for ws context
ws open                   Open the generated VS Code workspace
ws browse <repo>          Open the repo's origin URL in the default browser
                          ("." = repo containing the current directory)
ws context refresh
                          Re-resolve the stored context
ws context -              Swap to the previous context (like cd -)
ws context add <filter>
                          Extend the current context
ws context remove <filter>
                          Remove repos from the current context
ws context save [--local] <group>
                          Save the current context as a named group
ws cd [repo[@worktree]] [--worktree|-t <selector>]
                          Print repo path (or workspace root)
```

Worktree options:

- `-t`, `--worktrees`: expand repo/group filters to linked worktrees
- `--no-worktrees`: force primary checkouts only

Supported commands share one worktree mode. Set `worktrees: true` in `manifest.local.yml` if you want `repos`, `ll`, `pull`, `context`, and fan-out commands to include linked worktrees by default.

Any unrecognized command is executed across repos automatically:

```bash
ws git status
ws -t git status
ws backend git log --oneline -3
ws ops make plan
ws -- fetch data.json
```

`ws cd` changes your shell directory only when the `ws shell init` shell function is installed. The same shell hook also enables completion for `bash` and `zsh`. Without it, `ws cd` just prints the path.

## Filters

Filters apply to `ll`, `setup`, `fetch`, `pull`, `context`, and fan-out commands.

- `all`: every active repo from the merged manifest
- `dirty`: repos with uncommitted changes
- `active`: repos that are dirty or have a local-user commit in the last 14 days
- `active:1d`: same as `active`, but with a custom recent window
- `mine:1d`: repos with a local-user commit in the given recent window
- `backend`: a group name
- `backend,ops`: multiple groups
- `backend,active:1w`: combine named filters with activity selectors
- `api-server`: an individual repo
- `api-server@api-server-feature`: an explicit worktree target

Recent-duration filters accept positive values with `s`, `m`, `h`, `d`, or `w` suffixes.

For `ws context`, `none` or `reset` clears the saved context.
`ws ctx <filter>` is shorthand for `ws context <filter>`.

Use groups for named subsets. The default `all` filter includes every active repo from `manifest.yml` plus any overrides from `manifest.local.yml`.

## Worktrees

`ws` discovers linked git worktrees at runtime from the manifest checkout. They are not stored as individual repos in `manifest.yml`.

- Set `worktrees: true` in `manifest.local.yml` to make worktree-aware behavior the default across `repos`, `ll`, `pull`, `context`, and fan-out commands.
- Without that setting, commands default to the manifest checkout for each repo.
- `ws repos` shows a `WT` count for each repo; worktree mode expands it to one row per checkout.
- `ws ll`, `ws ll --branches`, `ws pull`, `ws context`, and `ws <command...>` follow the same worktree mode instead of each command behaving differently.
- Use `-t` or `--worktrees` to enable worktree mode for one command, or `--no-worktrees` to disable it for one command.
- `ws cd api-server -t feature/auth` resolves a linked worktree by unique branch, path basename, or exact path.
- `ws cd api-server@api-server-feature` is shorthand for selecting a worktree by its checkout name.
- `ws fetch` remains repo-scoped and runs once per manifest repo.
- In worktree mode, `ws context` includes sibling worktrees in the generated workspace so both checkouts appear in the VS Code Explorer.

## Context And Agents

`ws context <filter>` does three things:

1. stores the raw filter plus resolved default scope in `.ws/context.yml`
2. regenerates the VS Code workspace file for that filter
3. rebuilds the configured scope symlink directories (default: `.scope/`)

For `ws context all`, `ws context none`, and `ws context reset`, the generated scope only includes repos that are already cloned on disk.
`ws context refresh` reruns that resolution against the saved raw filter, which is useful after local activity changes or when linked worktrees were added or removed.

Configure scope directories with `scopes:` in `manifest.yml` or `manifest.local.yml`.
The default is:

```yaml
scopes:
  - dir: .scope
    source: context
```

Available `source` values:

- `context`: the repos in the current `ws context`
- `all`: all cloned repos on disk, independent of the current context

Example:

```yaml
scopes:
  - dir: .scope
    source: context
  - dir: .all-repos
    source: all
```

Set `scopes: []` to disable generated scope directories entirely.

That makes the workspace repo useful as an agent entry point:

- run `ws context backend` before starting focused work
- run `ws context active` when you want a quick scope based on active local work
- run `ws open` when you want to open the current generated workspace in VS Code
- run `ws context add repo-x` when you want to widen that scope without replacing it
- run `ws context add active:1d` when you want to union the current scope with very recent local activity
- run `ws context remove repo-x` when you want to narrow the current scope without rebuilding it from scratch
- run `ws context refresh` when a dynamic filter or worktree-aware context needs to be re-resolved
- run `ws context -` (or `ws context prev`) to swap back to the previous context, like `cd -`
- run `ws context save focus` when you want to snapshot the current scope into `manifest.yml`
- run `ws context save --local scratch` when the saved group should live only in `manifest.local.yml`
- use the workspace repo as the control plane and `.scope/` or another configured scope dir as the narrowed filesystem view for agents
- keep the shared manifest committed while local context stays in ignored files

Recommended operator and agent loop:

1. From the workspace root, run `ws context <filter>`.
   To widen an existing scope, use `ws context add <filter>`.
   To narrow the current scope, use `ws context remove <filter>`.
   To re-resolve the saved filter after activity or worktree changes, use `ws context refresh`.
2. Verify the scope with `ws ll` or `ws repos`.
3. Start the agent from `.scope/` or another configured scope dir when you want filesystem visibility to match that context.
4. Return to the workspace root when you need to change scope or edit `manifest.yml`.

Example:

```bash
# operator shell
ws context backend
ws ll

# agent shell
cd .scope
ls .scope
```

## Manifest

`manifest.yml` is the source of truth for the workspace.

```yaml
root: ..

remotes:
  origin: git@github.com:acme-corp

branch: main
workspace: ws.code-workspace

groups:
  backend: [api-server, auth-service, worker]
  frontend: [web-app, admin-dashboard]

repos:
  api-server:
  auth-service: { branch: develop }
  worker:
  web-app:
  admin-dashboard:

  # Forked OSS repo — origin from the top-level prefix,
  # upstream added as an extra remote pointing at the upstream fork.
  upstream-lib:
    branch: stable
    remotes:
      upstream: git@github.com:open-source-org/upstream-lib.git
    default_compare: upstream

  # Bespoke origin URL that doesn't match the prefix.
  custom-tool:
    remotes:
      origin: git@custom.host:org/repo.git

exclude:
  - legacy-api
```

Field summary:

- `remotes`: named clone URL prefixes at the top level. `/<repo>.git` is appended
  when resolving a repo's URL. `origin` is the clone source and required for every
  repo (either here as a prefix or as a per-repo full URL).
- `repos.<name>.remotes`: per-repo map of extra git remotes. Values are full URLs
  (used as-is). Merged with the top-level map; per-repo keys override.
- `repos.<name>.default_compare`: name of the remote `ll` should compare against
  (parsed today; full `ll` wiring lands in a follow-up).
- `branch`: default branch for repos that do not override it
- `root`: required; where repos live, relative to the manifest directory or absolute
- `workspace`: filename for the generated VS Code workspace
- `scopes`: generated symlink directories for scoped repo views
- `worktrees`: default worktree mode for supported commands
- `groups`: named repo sets used for filters
- `repos`: active repos and per-repo overrides
- `exclude`: catalog entries you do not want in normal operations

Migrating from older manifests: the keys `repos.<name>.url` and
`repos.<name>.remote` are removed — replace them with `repos.<name>.remotes.origin`.
The top-level alias `remotes.default` is renamed to `remotes.origin`. See
[UPGRADING.md](UPGRADING.md).

Start from the full reference file at [manifest.reference.yml](manifest.reference.yml).

## Local Overrides

Use `manifest.local.yml` for personal changes you do not want to commit.

```yaml
scopes:
  - dir: .scope
    source: context
  - dir: .all-repos
    source: all

worktrees: true

repos:
  my-experiment:
    branch: dev
    remotes:
      origin: git@github.com:myuser/my-experiment.git

exclude:
  - worker

groups:
  my-project: [api-server, my-experiment]
```

Merge rules:

- `remotes`: union, local wins on name conflict
- `repos`: union, local wins on name conflict
- `exclude`: additive
- `groups`: local replaces same-name groups, adds new ones
- `root`, `workspace`, `scopes`, and `worktrees`: local overrides when set

`ws context save <group>` writes the current context into `manifest.yml`.
Use `ws context save --local <group>` when the saved group should live only in `manifest.local.yml`.

## Configuration

| Variable | Description |
|---|---|
| `WS_HOME` | Optional workspace root override |
| `WS_WORKERS` | Max parallel workers (default: CPU count) |
| `NO_COLOR` | Disable colored output |

---

See [DESIGN.md](DESIGN.md) for rationale behind CLI shape decisions (help
layout, aliases, passthrough model, why not Cobra).
