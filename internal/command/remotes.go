package command

import (
	"fmt"
	"os"
	"sort"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// RemotesSync reconciles declared remotes against each repo on disk:
// adds missing remotes, warns on URL divergence, leaves unknown on-disk
// remotes alone. Never destructive.
func RemotesSync(m *manifest.Manifest, wsHome, filter string) error {
	repos, err := resolveCommandRepos(m, wsHome, filter, false)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	added, warned, skipped := SyncRepoRemotes(repos)

	switch {
	case added == 0 && warned == 0:
		fmt.Println("Remotes already in sync.")
	default:
		fmt.Printf("Synced %d remote(s)", added)
		if warned > 0 {
			fmt.Printf("; %d divergence warning(s)", warned)
		}
		fmt.Println(".")
	}
	if skipped > 0 {
		fmt.Printf("Skipped %d uncloned repo(s).\n", skipped)
	}
	return nil
}

// SyncRepoRemotes performs the per-repo reconciliation without printing a
// summary, returning (added, warned, skipped) counts. Useful when another
// command (e.g. Setup) wants to embed the reconcile pass without the
// standalone summary line.
func SyncRepoRemotes(repos []manifest.RepoInfo) (added, warned, skipped int) {
	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			skipped++
			continue
		}
		names := make([]string, 0, len(repo.Remotes))
		for name := range repo.Remotes {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			want := repo.Remotes[name]
			got, err := git.RemoteURL(repo.Path, name)
			if err != nil {
				if err := git.AddRemote(repo.Path, name, want); err != nil {
					fmt.Fprintf(os.Stderr, "  %s: add remote %s failed: %v\n", repo.Name, name, err)
					continue
				}
				fmt.Printf("  %s: added %s %s\n", repo.Name, name, want)
				added++
				continue
			}
			if got != want {
				fmt.Fprintf(os.Stderr, "  %s: remote %s differs — on disk: %s, manifest: %s (leaving unchanged)\n",
					repo.Name, name, got, want)
				warned++
			}
		}
	}
	return added, warned, skipped
}
