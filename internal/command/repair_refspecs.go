package command

import (
	"fmt"
	"os"
	"regexp"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// singleBranchRefspec matches a refspec of the form
// "+refs/heads/<branch>:refs/remotes/origin/<branch>" where the branch is a
// concrete name (not a wildcard). This is what `git clone --single-branch`
// writes into .git/config.
var singleBranchRefspec = regexp.MustCompile(`^\+refs/heads/([^*:]+):refs/remotes/origin/([^*:]+)$`)

// RepairRefspecs restores the default fetch refspec on repos whose origin
// was cloned with --single-branch (the historical ws Clone behavior). Older
// ws versions baked a restricted "+refs/heads/<branch>:refs/remotes/origin/<branch>"
// into .git/config, so `git fetch` only ever pulled that one branch.
//
// Legacy repair command — can be removed once no one has pre-fix checkouts.
func RepairRefspecs(m *manifest.Manifest, wsHome, filter string) error {
	repos, err := resolveCommandRepos(m, wsHome, filter, false)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	var repaired, healthy, skipped, issues int
	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			skipped++
			continue
		}
		refspecs, err := git.FetchRefspecs(repo.Path, "origin")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: read refspec failed: %v\n", repo.Name, err)
			issues++
			continue
		}
		if hasDefaultOriginRefspec(refspecs) {
			healthy++
			continue
		}
		if isSingleBranchRefspecSet(refspecs) {
			if err := git.SetDefaultFetchRefspec(repo.Path, "origin"); err != nil {
				fmt.Fprintf(os.Stderr, "  %s: repair failed: %v\n", repo.Name, err)
				issues++
				continue
			}
			fmt.Printf("  %s: restored default fetch refspec\n", repo.Name)
			repaired++
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s: non-standard origin refspecs, leaving unchanged:\n", repo.Name)
		for _, r := range refspecs {
			fmt.Fprintf(os.Stderr, "      %s\n", r)
		}
		issues++
	}

	fmt.Printf("Repaired %d; already OK %d", repaired, healthy)
	if issues > 0 {
		fmt.Printf("; %d need manual attention", issues)
	}
	if skipped > 0 {
		fmt.Printf("; %d uncloned", skipped)
	}
	fmt.Println(".")
	if issues > 0 {
		return fmt.Errorf("%d repo(s) need manual attention", issues)
	}
	return nil
}

func hasDefaultOriginRefspec(refspecs []string) bool {
	const want = "+refs/heads/*:refs/remotes/origin/*"
	for _, r := range refspecs {
		if r == want {
			return true
		}
	}
	return false
}

func isSingleBranchRefspecSet(refspecs []string) bool {
	if len(refspecs) == 0 {
		return true
	}
	if len(refspecs) != 1 {
		return false
	}
	return singleBranchRefspec.MatchString(refspecs[0])
}
