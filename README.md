# ws

A fast, single-binary CLI for managing multi-repo workspaces. Reads a declarative `manifest.yml` and provides dashboard, parallel git operations, cloning, and VS Code workspace management.

## Install

```bash
go install github.com/dtuit/ws/cmd/ws@latest
```

Or build from source:

```bash
git clone git@github.com:dtuit/ws.git && cd ws && make install
```

## Setup

Point `ws` at your workspace (the directory containing `manifest.yml`):

```bash
export WS_HOME=/path/to/my-workspace
```

Or just run `ws` from within the workspace directory - it walks up to find `manifest.yml`.

## Commands

```
ws ll [filter]            Dashboard: branch, dirty status, last commit
ws super [filter] <cmd>   Run a command across repos
ws fetch [filter]         Fetch all repos
ws pull [filter]          Pull all repos
ws setup [filter]         Clone missing repos
ws focus [filter]         Filter VS Code workspace folders
ws list [--all]           Show manifest repos with status
```

**Filters:** `all` (default), group name (`backend`), comma-separated (`backend,infra`), or a repo name (`api-server`).

## Manifest

`manifest.yml` is the single source of truth:

```yaml
remotes:
  default: git@github.com:acme-corp
  upstream: git@github.com:open-source-org

branch: main

groups:
  backend: [api-server, auth-service, worker]
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
repos:
  legacy-api:                                # un-exclude a repo
  my-fork: { remote: my-remote, branch: dev }

exclude:
  - repo-i-dont-need

groups:
  my-project: [api-server, my-fork]
```

Merge rules: local repos/remotes/groups are added (same name = local wins). A repo in local `repos:` is active even if excluded in the main manifest.
