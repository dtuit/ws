# ws

`ws` turns a repo containing `manifest.yml` into the control plane for a multi-repo workspace.

The intended model is simple:

- create a dedicated repo for your workspace manifest
- use that repo as the root workspace
- clone and operate on the managed repos from there
- use filters and context to scope editor state, operations, and agent-visible paths

`ws` reads `manifest.yml`, clones missing repos, fans commands out across repos, generates a VS Code workspace, and maintains a scoped `.scope/` tree for filesystem-based agents.

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

## Workspace Repo Model

Create a repo whose job is to describe and manage your working set.

```text
acme-workspace/
├── manifest.yml
├── manifest.local.yml      # optional, ignored
├── .ws-context             # generated, ignored
├── .scope/                 # generated symlinks, ignored
├── ws.code-workspace       # generated, ignored
└── repos/
    ├── api-server/
    ├── auth-service/
    └── web-app/
```

For a self-contained workspace repo, `root: repos` is the simplest default.
If tighter agent scoping matters more than a single-tree layout, keep the managed repos outside the workspace repo and use `.scope/` as the agent-facing tree.

A typical `.gitignore` for the workspace repo:

```gitignore
.scope/
.ws-context
*.code-workspace
manifest.local.yml
repos/
```

Commit the control-plane files: `manifest.yml`, shared docs, and any shared scripts.
Keep local overrides, generated workspace files, and cloned repos out of git.

## Bootstrap A Workspace Repo

```bash
mkdir acme-workspace
cd acme-workspace
git init

cat > .gitignore <<'EOF'
.scope/
.ws-context
*.code-workspace
manifest.local.yml
repos/
EOF

cat > manifest.yml <<'EOF'
remotes:
  default: git@github.com:acme-corp

branch: main
root: repos

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

See [example/README.md](example/README.md) for a runnable sample workspace.

## Quick Start

1. Create a repo for the workspace manifest.
2. Add `manifest.yml`.
3. Run `ws` from inside that repo.

Example `manifest.yml`:

```yaml
remotes:
  default: git@github.com:acme-corp

branch: main
root: repos
workspace: ws.code-workspace

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
ws code backend
```

If you want shell integration for `ws cd` and tab completion, either add this to your shell config:

```bash
export WS_HOME=/path/to/acme-workspace
eval "$(ws init)"
```

or let `ws` write it for you:

```bash
ws setup --install-shell
```

With that loaded, `ws` completes built-in commands, filters, repo names, and falls back to your shell's command completion for fan-out commands like `ws backend git ...`.

## Daily Workflow

Operate from the workspace repo. `ws` finds the workspace by:

1. `-w` / `--workspace`
2. `WS_HOME`
3. walking up from the current directory until it finds `manifest.yml`

Core commands:

```text
ws ll [filter] [--worktrees]
                          Dashboard: branch, dirty state, last commit
ws list [--all] [--worktrees]
                          Show repos in the manifest
ws setup [filter]         Clone missing repos
ws fetch [filter]         Fetch all repos in scope
ws pull [filter] [--worktrees]
                          Pull manifest checkouts or all discovered worktrees
ws context [filter]       Persist the default filter
ws code [-t|--worktrees] [filter]
                          Generate the VS Code workspace file
ws context add <filter>   Extend the current context
ws cd [repo] [--worktree <selector>]
                          Print repo path (or workspace root)
ws init                   Emit shell integration and completion
```

`ws code` is git-worktree aware. If a selected repo has sibling worktrees, they are added as separate folders so both checkouts appear in the VS Code Explorer. `-t` is accepted as a short explicit alias for `ws code --worktrees`.

Any unrecognized command is executed across repos automatically:

```bash
ws git status
ws --worktrees git status
ws backend git log --oneline -3
ws ops make plan
ws -- fetch data.json
```

`ws cd` changes your shell directory only when the `ws init` shell function is installed. The same shell hook also enables completion for `bash` and `zsh`. Without it, `ws cd` just prints the path.

## Filters

Filters apply to `ll`, `setup`, `fetch`, `pull`, `code`, `context`, and fan-out commands.

- `all`: every repo that belongs to at least one group
- `backend`: a group name
- `backend,ops`: multiple groups
- `api-server`: an individual repo

If you want a repo included in default operations, put it in at least one group. The default `all` filter only includes grouped repos.

## Worktrees

`ws` discovers linked git worktrees at runtime from the manifest checkout. They are not stored in `manifest.yml`.

- Default commands still target the manifest checkout for each repo.
- `ws list` shows a `WT` count for each repo; `ws list --worktrees` expands to one row per checkout.
- `ws ll --worktrees`, `ws pull --worktrees`, and `ws --worktrees <command...>` expand operations to all discovered worktrees.
- `ws cd api-server --worktree feature/auth` resolves a linked worktree by unique branch, path basename, or exact path.
- `ws fetch` remains repo-scoped and runs once per manifest repo.
- `ws code` always includes sibling worktrees in the generated workspace, and `ws code -t backend` is accepted as an explicit form.

## Context And Agents

`ws context <filter>` does three things:

1. stores the default filter in `.ws-context`
2. regenerates the VS Code workspace file for that filter
3. rebuilds `.scope/` with symlinks to only the repos in scope

That makes the workspace repo useful as an agent entry point:

- run `ws context backend` before starting focused work
- run `ws context add repo-x` when you want to widen that scope without replacing it
- use the workspace repo as the control plane and `.scope/` as the narrowed filesystem view for agents
- keep the shared manifest committed while local context stays in ignored files

Recommended operator and agent loop:

1. From the workspace root, run `ws context <filter>`.
   To widen an existing scope, use `ws context add <filter>`.
2. Verify the scope with `ws ll` or `ws list`.
3. Start the agent from `.scope/` when you want filesystem visibility to match that context.
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
remotes:
  default: git@github.com:acme-corp
  upstream: https://github.com/open-source-org

branch: main
root: repos
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
  upstream-lib: { remote: upstream, branch: stable }
  custom-tool: { url: git@custom.host:org/repo.git }

exclude:
  - legacy-api
```

Field summary:

- `remotes`: named clone URL prefixes; `default` is the fallback
- `branch`: default branch for repos that do not override it
- `root`: where repos live; relative to the manifest directory or absolute
- `workspace`: filename for the generated VS Code workspace
- `groups`: named repo sets used for filters
- `repos`: active repos and per-repo overrides
- `exclude`: catalog entries you do not want in normal operations

Start from the full reference file at [manifest.reference.yml](manifest.reference.yml).

## Local Overrides

Use `manifest.local.yml` for personal changes you do not want to commit.

```yaml
remotes:
  my-fork: git@github.com:myuser

repos:
  my-experiment: { remote: my-fork, branch: dev }

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
- `root` and `workspace`: local overrides when set

## Configuration

| Variable | Description |
|---|---|
| `WS_HOME` | Optional workspace root override |
| `WS_WORKERS` | Max parallel workers (default: CPU count) |
| `NO_COLOR` | Disable colored output |
