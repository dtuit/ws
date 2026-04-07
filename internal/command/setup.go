package command

import (
	"fmt"
	"os"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Setup clones missing repos for the selected filter.
func Setup(m *manifest.Manifest, wsHome, filter string) error {
	repos, err := resolveFilterRepos(m, wsHome, filter, false)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
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

	total := 0
	for _, repo := range m.AllRepos(wsHome) {
		if git.IsCheckout(repo.Path) {
			total++
		}
	}

	if cloned > 0 {
		fmt.Printf("Cloned %d repo(s).\n", cloned)
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

	return nil
}
