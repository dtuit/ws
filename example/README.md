# ws example workspace

A sample workspace using small open-source Go repos from [Charm](https://github.com/charmbracelet).

For the full manifest schema, see [../manifest.reference.yml](../manifest.reference.yml).

## Quick start

```bash
cd example
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

# Launch an agent from the narrowed view
cd .scope

# Run a command across repos
ws git log --oneline -3

# Clear context
cd ..
ws context none

# Generate VS Code workspace
ws code tui
```
