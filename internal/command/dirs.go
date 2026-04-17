package command

import (
	"fmt"
	"unicode/utf8"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Dirs prints repo name and absolute path pairs, one per line.
// Names are padded so paths align for readability, while remaining
// easy to parse (columns are whitespace-separated).
func Dirs(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool, root bool) error {
	if root {
		fmt.Println(wsHome)
		return nil
	}

	repos, err := resolveCommandRepos(m, wsHome, filter, includeWorktrees)
	if err != nil {
		return err
	}

	// Collect cloned repos and measure name widths
	type entry struct {
		name, path string
	}
	var entries []entry
	maxName := 0
	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			continue
		}
		entries = append(entries, entry{repo.Name, repo.Path})
		if n := utf8.RuneCountInString(repo.Name); n > maxName {
			maxName = n
		}
	}

	for _, e := range entries {
		fmt.Printf("%-*s  %s\n", maxName, e.name, e.path)
	}
	return nil
}
