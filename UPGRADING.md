# Upgrading `ws`

## Multi-remote manifests (breaking)

The manifest schema changed to support multiple git remotes per repo (e.g. a
forked OSS repo with both `origin` and `upstream`). The changes are breaking
but mechanical.

### What changed

1. Top-level `remotes.default` → `remotes.origin`. `origin` is now the only
   magic key; `default` is no longer recognised.
2. Per-repo `url:` is removed. Replacement: `repos.<name>.remotes.origin:
   <full-url>`.
3. Per-repo `remote:` (alias selector) is removed. If a repo needs a non-prefix
   origin, set `repos.<name>.remotes.origin` directly.
4. New per-repo field: `remotes` — a map of `name → full URL` for extra git
   remotes. Merged with the top-level remotes map; per-repo wins on conflict.
5. New per-repo field: `default_compare` — the name of the remote `ws ll`
   should compare against in addition to the branch's native upstream. Renders
   as `<remote>:<symbol>` (e.g. `upstream:↑3`) next to the existing sync
   symbol. Shows `~` until the remote has been fetched.

### Before / after

```yaml
# Before
remotes:
  default: git@github.com:acme-corp
  upstream: https://github.com/open-source-org
repos:
  upstream-lib: { remote: upstream, branch: stable }
  custom-tool:  { url: git@custom.host:org/repo.git }

# After
remotes:
  origin: git@github.com:acme-corp
repos:
  upstream-lib:
    branch: stable
    remotes:
      upstream: git@github.com:open-source-org/upstream-lib.git
    default_compare: upstream
  custom-tool:
    remotes:
      origin: git@custom.host:org/repo.git
```

### What `ws` does with it

- `ws setup` clones from the effective `origin`, then runs
  `git remote add <name> <url>` for every other declared remote.
- `ws remotes sync [filter]` is a new command that reconciles declared remotes
  against existing checkouts: adds missing remotes, warns on URL drift, never
  removes on-disk remotes.
- `ws fetch` now defaults to `git fetch --all --prune` so every configured
  remote gets fetched. Pass `--remote <name>` (repeatable) to narrow.
- `ws pull` is unchanged — still pulls from origin via the branch's git
  upstream config.

### Validation

The manifest parser now rejects:

- a repo whose effective remotes map has no `origin` entry,
- per-repo `url:` or `remote:` keys (with a migration message),
- `default_compare` that doesn't match any declared remote,
- any unknown key inside a repo entry.

These are parse-time errors, so `ws list`, `ws ll`, etc. surface them early.
