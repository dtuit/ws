package command

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Setup clones missing repos for the selected filter. Returns the number
// of repos cloned so callers can decide whether to run follow-up actions
// (e.g. context refresh).
func Setup(m *manifest.Manifest, wsHome, filter string) (int, error) {
	repos, err := resolveFilterRepos(m, wsHome, filter, false)
	if err != nil {
		return 0, err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return 0, nil
	}

	cloned := 0
	for _, repo := range repos {
		if _, err := os.Stat(repo.Path); err == nil {
			continue
		}
		if err := manifest.ValidateURL(repo.URL); err != nil {
			fmt.Fprintf(os.Stderr, "  Skipping %s: %v\n", repo.Name, err)
			continue
		}
		fmt.Printf("  Cloning %s (%s)...\n", repo.Name, repo.Branch)
		if err := git.Clone(repo); err != nil {
			fmt.Fprintf(os.Stderr, "  FAILED: %v\n", err)
			continue
		}
		cloned++
	}

	// Reconcile remotes on already-cloned repos (newly-cloned ones are already
	// configured by Clone). Non-destructive: adds missing remotes, warns on
	// URL drift. The standalone `ws remotes sync` reuses the same helper.
	addedRemotes, _, _ := SyncRepoRemotes(repos)

	total := 0
	for _, repo := range m.AllRepos(wsHome) {
		if git.IsCheckout(repo.Path) {
			total++
		}
	}

	if cloned > 0 {
		fmt.Printf("Cloned %d repo(s).\n", cloned)
	}
	if addedRemotes > 0 {
		fmt.Printf("Added %d remote(s) to existing repos.\n", addedRemotes)
	}
	fmt.Printf("Setup complete: %d repo(s) on disk.\n", total)

	if os.Getenv("WS_HOME") == "" {
		fmt.Printf("\nAdd to your shell config (~/.bashrc or ~/.zshrc) for ws cd and completion:\n\n")
		fmt.Printf("  # BEGIN ws\n")
		fmt.Printf("  export WS_HOME=%q\n", wsHome)
		fmt.Printf("  eval \"$(ws shell init)\"\n")
		fmt.Printf("  # END ws\n\n")
		fmt.Printf("Or run: ws shell install\n")
	}

	return cloned, nil
}

// SetupGuide prints a summary of active repos and groups, with suggested
// commands for cloning a useful subset. Used when `ws setup` is run with no
// filter and no context — better than cloning 100+ repos by default.
func SetupGuide(m *manifest.Manifest, wsHome string) error {
	active := m.ActiveRepos()
	activeCount := len(active)

	// Collect groups (skip empty ones), sort by name for stable output.
	type groupInfo struct {
		name    string
		members []string
	}
	var groups []groupInfo
	for name, members := range m.Groups {
		if len(members) == 0 {
			continue
		}
		groups = append(groups, groupInfo{name: name, members: members})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].name < groups[j].name
	})

	if activeCount == 0 {
		fmt.Println("No active repos in manifest. Add some to `repos:` in manifest.yml.")
		return nil
	}

	fmt.Printf("This workspace has %d active repos", activeCount)
	if len(groups) > 0 {
		fmt.Printf(" across %d groups. Pick a subset to clone:\n\n", len(groups))

		// Pad group names so members align.
		maxName := 0
		for _, g := range groups {
			if n := len(g.name); n > maxName {
				maxName = n
			}
		}
		for _, g := range groups {
			sample := g.members
			const maxSample = 3
			overflow := 0
			if len(sample) > maxSample {
				overflow = len(sample) - maxSample
				sample = sample[:maxSample]
			}
			line := strings.Join(sample, ", ")
			if overflow > 0 {
				line += fmt.Sprintf(", +%d", overflow)
			}
			fmt.Printf("  %-*s  (%d)  %s\n", maxName, g.name, len(g.members), line)
		}
		fmt.Println()
		fmt.Println("Recommended flow:")
		fmt.Println("  ws context <group>       # pick one from above")
		fmt.Println("  ws setup                 # clone just those")
		fmt.Println()
	} else {
		fmt.Printf(". No groups defined.\n\n")
	}

	fmt.Println("Other options:")
	fmt.Println("  ws setup <repo>          # a single repo")
	fmt.Println("  ws setup all             # clone everything (slow for large workspaces)")
	fmt.Println()

	fmt.Println("Current context: (not set)")
	return nil
}
