package command

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/term"
	xterm "golang.org/x/term"
)

const (
	llWorktreeSuffixColor = term.Bold + term.Cyan
	llDefaultMessageWidth = 60
)

// LLMode controls optional ll output extensions.
type LLMode struct {
	ShowBranches bool
}

// LL displays a dashboard of repo status: branch, dirty state, last commit.
func LL(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool, mode LLMode) error {
	repos, err := resolveCommandRepos(m, wsHome, filter, includeWorktrees)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	worktreeExtras := make(map[string]int)
	if !includeWorktrees {
		for _, set := range git.DiscoverWorktreesAll(repos, git.Workers(len(repos))) {
			if set.Err == nil && len(set.Worktrees) > 1 {
				worktreeExtras[set.Repo.Name] = len(set.Worktrees) - 1
			}
		}
	}

	workers := git.Workers(len(repos))
	statuses := git.StatusAll(repos, workers)
	var branchLists []git.BranchList
	if mode.ShowBranches {
		branchLists = git.LocalBranchesAll(repos, workers)
	}
	termWidth := llTerminalWidth()

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
	for _, list := range branchLists {
		for _, branch := range list.Branches {
			if len(branch.Name) > maxBranch {
				maxBranch = len(branch.Name)
			}
		}
	}

	for i, s := range statuses {
		printLLStatusLine(s, maxName, maxBranch, termWidth, worktreeExtras, true)
		if !mode.ShowBranches || s.Err != nil {
			continue
		}
		if branchLists[i].Err != nil {
			printLLStatusLine(git.RepoStatus{Name: "", Err: branchLists[i].Err}, maxName, maxBranch, termWidth, nil, true)
			continue
		}
		for _, branch := range branchLists[i].Branches {
			if branch.Current {
				continue
			}
			printLLStatusLine(repoStatusFromLocalBranch(branch), maxName, maxBranch, termWidth, nil, false)
		}
	}
	return nil
}

func repoStatusFromLocalBranch(branch git.LocalBranchInfo) git.RepoStatus {
	return git.RepoStatus{
		Name:      "",
		Branch:    branch.Name,
		Ahead:     branch.Ahead,
		Behind:    branch.Behind,
		NoRemote:  branch.NoRemote,
		CommitMsg: branch.CommitMsg,
		CommitAge: branch.CommitAge,
	}
}

func printLLStatusLine(s git.RepoStatus, maxName, maxBranch, termWidth int, worktreeExtras map[string]int, showEmptySymbols bool) {
	nameStr := formatLLName(s.Name, maxName, term.Red)
	if s.Err != nil {
		fmt.Printf("%s  %s\n", nameStr, term.Colorize(term.Red, s.Err.Error()))
		return
	}

	// Status symbols: +staged *unstaged ?untracked $stashed
	symbols := s.Symbols()
	sync := s.SyncSymbol()

	// Color based on state
	color := llStatusColor(s)

	syncStr := fmt.Sprintf("%-4s", sync)
	statusText := fmt.Sprintf("  %-*s %s", maxBranch, s.Branch, syncStr)
	if showEmptySymbols || symbols != "" {
		statusText += fmt.Sprintf("[%s]", fmt.Sprintf("%-4s", symbols))
	}
	if cmp := s.CompareSymbol(); cmp != "" {
		statusText += fmt.Sprintf(" %s:%s", s.CompareRemote, cmp)
	}

	ageSuffix := ""
	if s.CommitAge != "" {
		ageSuffix = " (" + s.CommitAge + ")"
	}
	extrasSuffix := ""
	if extra := worktreeExtras[s.Name]; extra > 0 {
		extrasSuffix = fmt.Sprintf(" [+%d wt]", extra)
	}

	nameStr = formatLLName(s.Name, maxName, color)
	statusStr := term.Colorize(color, statusText)
	prefix := nameStr + statusStr
	prefixWidth := maxName + utf8.RuneCountInString(statusText)

	detailPlain, detailColored := formatLLDetail(s.CommitMsg, ageSuffix, extrasSuffix, 0)
	if termWidth > 0 && prefixWidth+2+utf8.RuneCountInString(detailPlain) > termWidth {
		indentWidth := llDetailIndentWidth(maxName, termWidth)
		indent := strings.Repeat(" ", indentWidth)
		_, wrappedDetail := formatLLDetail(s.CommitMsg, ageSuffix, extrasSuffix, termWidth-indentWidth)
		fmt.Printf("%s\n%s%s\n", prefix, indent, wrappedDetail)
		return
	}

	fmt.Printf("%s  %s\n", prefix, detailColored)
}

func llStatusColor(s git.RepoStatus) string {
	cmpAhead, cmpBehind := s.CompareAhead, s.CompareBehind
	if s.CompareRemote == "" || s.CompareNoRef {
		cmpAhead, cmpBehind = 0, 0
	}
	switch {
	case s.Branch == "(detached)":
		return term.Red
	case (s.Ahead > 0 && s.Behind > 0) || (cmpAhead > 0 && cmpBehind > 0):
		return term.Red
	case s.Ahead > 0 || cmpAhead > 0:
		return term.Magenta
	case s.Behind > 0 || cmpBehind > 0:
		return term.Yellow
	case s.IsDirty():
		return term.Yellow
	case s.NoRemote:
		return term.Cyan
	default:
		return term.Green
	}
}

func formatLLName(name string, width int, color string) string {
	if name == "" {
		return strings.Repeat(" ", width)
	}
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

func llTerminalWidth() int {
	width, _, err := xterm.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}

func llDetailIndentWidth(maxName, termWidth int) int {
	width := maxName + 2
	if width < 2 {
		width = 2
	}
	if width > 20 {
		width = 20
	}
	if termWidth > 0 && width > termWidth-10 {
		width = termWidth - 10
	}
	if width < 2 {
		width = 2
	}
	return width
}

func formatLLDetail(msg, ageSuffix, extrasSuffix string, available int) (string, string) {
	fullPlain := msg + ageSuffix + extrasSuffix
	if available <= 0 {
		available = llDefaultMessageWidth + utf8.RuneCountInString(ageSuffix+extrasSuffix)
	}
	if utf8.RuneCountInString(fullPlain) <= available {
		return fullPlain, msg + term.Colorize(term.Dim, ageSuffix) + term.Colorize(term.Dim, extrasSuffix)
	}

	metaWidth := utf8.RuneCountInString(ageSuffix + extrasSuffix)
	if metaWidth+4 > available {
		ageSuffix = ""
		extrasSuffix = ""
		metaWidth = 0
	}

	msgWidth := available - metaWidth
	if msgWidth < 1 {
		msgWidth = available
		ageSuffix = ""
		extrasSuffix = ""
	}
	msg = ellipsizeLL(msg, msgWidth)
	return msg + ageSuffix + extrasSuffix, msg + term.Colorize(term.Dim, ageSuffix) + term.Colorize(term.Dim, extrasSuffix)
}

func ellipsizeLL(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}
