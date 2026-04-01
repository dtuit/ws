package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

const contextFile = ".ws-context"
const scopeDir = ".scope"

// SetContext sets the default filter, regenerates the VS Code workspace,
// and updates the repos/ symlink directory to match.
func SetContext(m *manifest.Manifest, wsHome, filter string) error {
	path := filepath.Join(wsHome, contextFile)
	filter, err := normalizeContextFilter(m, filter)
	if err != nil {
		return err
	}

	if filter == "" || filter == "none" || filter == "reset" {
		os.Remove(path)
		fmt.Println("Context cleared.")
		if err := syncReposDir(m, wsHome, ""); err != nil {
			return err
		}
		return Code(m, wsHome, "")
	}

	repos := m.ResolveFilter(filter, wsHome)
	if len(repos) == 0 {
		return fmt.Errorf("filter %q matched no repos", filter)
	}

	if err := os.WriteFile(path, []byte(filter+"\n"), 0644); err != nil {
		return err
	}
	fmt.Printf("Context set to %q (%d repos)\n", filter, len(repos))

	if err := syncReposDir(m, wsHome, filter); err != nil {
		return err
	}
	return Code(m, wsHome, filter)
}

// AddContext extends the current context with more groups or repos.
// If no context is set, it behaves like SetContext.
func AddContext(m *manifest.Manifest, wsHome, filter string) error {
	addition, err := normalizeContextFilter(m, filter)
	if err != nil {
		return err
	}
	if addition == "" {
		return fmt.Errorf("usage: ws context add <filter>")
	}

	currentRaw := GetContext(wsHome)
	current, err := normalizeContextFilter(m, currentRaw)
	if err != nil {
		return fmt.Errorf("existing context %q is invalid: %w", currentRaw, err)
	}

	merged := mergeContextFilters(current, addition)
	return SetContext(m, wsHome, merged)
}

// GetContext reads the current context filter, or "" if none is set.
func GetContext(wsHome string) string {
	data, err := os.ReadFile(filepath.Join(wsHome, contextFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ShowContext displays the current context.
func ShowContext(m *manifest.Manifest, wsHome string) {
	ctx := GetContext(wsHome)
	if ctx == "" {
		fmt.Println("No context set (using all grouped repos)")
		return
	}
	normalized, err := normalizeContextFilter(m, ctx)
	if err != nil {
		fmt.Printf("Context: %s (invalid: %v)\n", ctx, err)
		return
	}
	if normalized == "" {
		fmt.Println("No context set (using all grouped repos)")
		return
	}
	repos := m.ResolveFilter(normalized, wsHome)
	fmt.Printf("Context: %s (%d repos)\n", normalized, len(repos))
}

// syncReposDir creates/updates a repos/ directory with symlinks to the scoped repos.
// This constrains what filesystem-based agents (CLI tools, Claude Code) can see.
func syncReposDir(m *manifest.Manifest, wsHome, filter string) error {
	repos := m.ResolveFilter(filter, wsHome)
	dir := filepath.Join(wsHome, scopeDir)

	// Remove existing symlinks (but not non-symlink entries, for safety)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.Type()&os.ModeSymlink != 0 {
				os.Remove(p)
			}
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating repos dir: %w", err)
	}

	// Create symlinks for scoped repos
	for _, repo := range repos {
		target := repo.Path
		link := filepath.Join(dir, repo.Name)

		// Use relative path for the symlink
		relTarget, err := filepath.Rel(dir, target)
		if err != nil {
			relTarget = target
		}

		if err := os.Symlink(relTarget, link); err != nil && !os.IsExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: could not symlink %s: %v\n", repo.Name, err)
		}
	}

	fmt.Printf(".scope/ updated (%d symlinks)\n", len(repos))
	return nil
}

func normalizeContextFilter(m *manifest.Manifest, filter string) (string, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return "", nil
	}

	active := m.ActiveRepos()
	tokens := strings.Split(filter, ",")
	seen := make(map[string]bool)
	var normalized []string
	var invalid []string
	hasAll := false

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		switch token {
		case "none", "reset":
			if len(tokens) > 1 {
				return "", fmt.Errorf("%q cannot be combined with other filters", token)
			}
			return "", nil
		case "all":
			hasAll = true
			continue
		}

		if _, ok := m.Groups[token]; !ok {
			if _, ok := active[token]; !ok {
				invalid = append(invalid, token)
				continue
			}
		}

		if !seen[token] {
			seen[token] = true
			normalized = append(normalized, token)
		}
	}

	if len(invalid) > 0 {
		return "", fmt.Errorf("unknown context filter(s): %s", strings.Join(invalid, ", "))
	}
	if hasAll {
		return "all", nil
	}
	return strings.Join(normalized, ","), nil
}

func mergeContextFilters(current, addition string) string {
	if current == "" {
		return addition
	}
	if current == "all" || addition == "all" {
		return "all"
	}

	seen := make(map[string]bool)
	var merged []string
	for _, filter := range []string{current, addition} {
		for _, token := range strings.Split(filter, ",") {
			token = strings.TrimSpace(token)
			if token == "" || seen[token] {
				continue
			}
			seen[token] = true
			merged = append(merged, token)
		}
	}
	return strings.Join(merged, ",")
}
