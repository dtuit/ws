package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Setup clones missing repos and prints shell setup instructions.
func Setup(m *manifest.Manifest, parentDir, wsHome, filter string) error {
	repos := m.ResolveFilter(filter)
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	cloned := 0
	for _, repo := range repos {
		repoDir := filepath.Join(parentDir, repo.Name)
		if _, err := os.Stat(repoDir); err == nil {
			continue
		}
		if err := manifest.ValidateURL(repo.URL); err != nil {
			fmt.Fprintf(os.Stderr, "  Skipping %s: %v\n", repo.Name, err)
			continue
		}
		fmt.Printf("  Cloning %s (%s)...\n", repo.Name, repo.Branch)
		if err := git.Clone(parentDir, repo); err != nil {
			fmt.Fprintf(os.Stderr, "  FAILED: %v\n", err)
			continue
		}
		cloned++
	}

	// Count total cloned
	total := 0
	for _, repo := range m.AllRepos() {
		gitDir := filepath.Join(parentDir, repo.Name, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			total++
		}
	}

	if cloned > 0 {
		fmt.Printf("Cloned %d repo(s).\n", cloned)
	}
	fmt.Printf("Setup complete: %d repo(s) on disk.\n", total)

	// Print shell setup hint if not already configured
	if os.Getenv("WS_HOME") == "" {
		fmt.Printf("\nAdd to your shell config (~/.bashrc or ~/.zshrc):\n\n")
		fmt.Printf("  # BEGIN ws\n")
		fmt.Printf("  export WS_HOME=%q\n", wsHome)
		fmt.Printf("  eval \"$(ws init)\"\n")
		fmt.Printf("  # END ws\n\n")
	}

	return nil
}
