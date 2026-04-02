package command

import (
	"fmt"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/term"
)

const llWorktreeSuffixColor = term.Bold + term.Cyan

// LL displays a dashboard of repo status: branch, dirty state, last commit.
func LL(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) error {
	repos := m.ResolveFilter(filter, wsHome)
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	worktreeExtras := make(map[string]int)
	if includeWorktrees {
		repos = expandReposToWorktrees(repos)
	} else {
		for _, set := range git.DiscoverWorktreesAll(repos, git.Workers(len(repos))) {
			if set.Err == nil && len(set.Worktrees) > 1 {
				worktreeExtras[set.Repo.Name] = len(set.Worktrees) - 1
			}
		}
	}

	workers := git.Workers(len(repos))
	statuses := git.StatusAll(repos, workers)

	// Calculate column widths
	maxName, maxBranch := 0, 0
	for _, s := range statuses {
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
		if len(s.Branch) > maxBranch {
			maxBranch = len(s.Branch)
		}
	}

	for _, s := range statuses {
		nameStr := formatLLName(s.Name, maxName, term.Red)
		if s.Err != nil {
			fmt.Printf("%s  %s\n", nameStr, term.Colorize(term.Red, s.Err.Error()))
			continue
		}

		// Status symbols: +staged *unstaged ?untracked $stashed
		symbols := s.Symbols()
		sync := s.SyncSymbol()

		// Color based on state
		var color string
		switch {
		case s.Branch == "(detached)":
			color = term.Red
		case s.Ahead > 0 && s.Behind > 0:
			color = term.Red
		case s.Ahead > 0:
			color = term.Magenta
		case s.Behind > 0:
			color = term.Yellow
		case s.IsDirty():
			color = term.Yellow
		case s.NoRemote:
			color = term.Cyan
		default:
			color = term.Green
		}

		// Pad symbols to fixed width for alignment
		symbolStr := fmt.Sprintf("%-4s", symbols)
		syncStr := fmt.Sprintf("%-4s", sync)

		msg := s.CommitMsg
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}

		age := ""
		if s.CommitAge != "" {
			age = " " + term.Colorize(term.Dim, "("+s.CommitAge+")")
		}
		extras := ""
		if extra := worktreeExtras[s.Name]; extra > 0 {
			extras = " " + term.Colorize(term.Dim, fmt.Sprintf("[+%d wt]", extra))
		}

		nameStr = formatLLName(s.Name, maxName, color)
		statusStr := term.Colorize(color, fmt.Sprintf("  %-*s %s[%s]", maxBranch, s.Branch, syncStr, symbolStr))
		fmt.Printf("%s%s  %s%s\n", nameStr, statusStr, msg, age+extras)
	}
	return nil
}

func formatLLName(name string, width int, color string) string {
	padding := width - len(name)
	if padding < 0 {
		padding = 0
	}
	base, suffix, ok := strings.Cut(name, "@")
	if !ok {
		return term.Colorize(color, name) + strings.Repeat(" ", padding)
	}
	return term.Colorize(color, base) + term.Colorize(llWorktreeSuffixColor, "@"+suffix) + strings.Repeat(" ", padding)
}
