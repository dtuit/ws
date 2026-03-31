# ws

A fast, single-binary CLI for managing multi-repo workspaces. Reads a declarative `manifest.yml` and provides a dashboard, parallel command execution, cloning, and VS Code workspace management.

## Install

```bash
go install github.com/dtuit/ws/cmd/ws@latest
```

Or build from source:

```bash
git clone git@github.com:dtuit/ws.git && cd ws && make install
```

## Quick Start

```bash
# Point ws at your workspace
export WS_HOME=/path/to/my-workspace

# Clone all repos defined in manifest.yml
ws setup

# See what's going on
ws ll
```

## Commands

```
ws ll [filter]         Dashboard: branch, sync, dirty status, last commit
ws setup [filter]      Clone missing repos
ws focus [filter]      Filter VS Code workspace folders
ws list [--all]        Show manifest repos with status
ws fetch [filter]      Fetch all repos
ws pull [filter]       Pull all repos
ws context [filter]    Set default filter (no arg = show, "none" = clear)
```

Any other command is run across repos automatically:

```bash
ws git status                 # in all repos
ws backend git log --oneline -3   # in a group
ws ls -la                     # any command, not just git
```

### Dashboard

`ws ll` shows branch, remote sync status, and working tree state at a glance:

```
api-server      main    =   [    ]  fix: resolve compatibility issue (3 weeks ago)
auth-service    develop ↑2  [+*  ]  add OAuth2 support (2 hours ago)
worker          master  ↓1  [   $]  batch processing update (5 days ago)
web-app         main    ~   [ ?  ]  update dependencies (1 day ago)
```

**Sync indicators:** `=` in-sync, `↑N` ahead, `↓N` behind, `N⇕M` diverged, `~` no remote

**Dirty indicators:** `+` staged, `*` unstaged, `?` untracked, `$` stashed

### Context

Set a default filter so all commands scope to it:

```bash
ws context backend     # all commands now target backend repos
ws ll                  # shows only backend
ws git fetch           # fetches only backend
ws context none        # clear
```

### Filters

All commands that accept a filter support:

- `all` - all repos in any group (default)
- Group name: `backend`, `frontend`, `infra`
- Comma-separated: `backend,infra`
- Individual repo: `api-server`

## Manifest

`manifest.yml` is the single source of truth:

```yaml
remotes:
  default: git@github.com:acme-corp
  upstream: git@github.com:open-source-org

branch: main
root: ..          # where repos live, relative to this file (default: ..)

groups:
  backend:  [api-server, auth-service, worker]
  frontend: [web-app, admin-dashboard]

repos:
  api-server:                               # default remote + branch
  auth-service: { branch: develop }         # override branch
  web-app: { remote: upstream }             # use named remote
  custom: { url: git@host:org/repo.git }    # full URL

exclude:
  - legacy-api
```

## Local Overrides

Create `manifest.local.yml` (gitignored) for personal customization:

```yaml
remotes:
  my-fork: git@github.com:myuser

repos:
  legacy-api:                                # un-exclude a repo
  my-experiment: { remote: my-fork, branch: dev }

exclude:
  - repo-i-dont-need

groups:
  my-project: [api-server, my-experiment]
```

Merge rules: local repos/remotes/groups are added (same name = local wins). A repo in local `repos:` is active even if excluded in the main manifest.

## Configuration

| Variable | Description |
|---|---|
| `WS_HOME` | Path to workspace directory containing `manifest.yml` |
| `WS_WORKERS` | Max parallel workers (default: CPU count) |
| `NO_COLOR` | Disable colored output |
