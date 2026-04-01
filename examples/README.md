# ws example workspace

A sample workspace using small open-source Go repos from [Charm](https://github.com/charmbracelet).

## Quick start

```bash
cd examples
ws setup
ws ll
```

## Try it out

```bash
# Dashboard
ws ll

# Scope to TUI repos
ws context tui
ws ll

# Run a command across repos
ws git log --oneline -3

# Clear context
ws context none

# Generate VS Code workspace
ws code tui
```
