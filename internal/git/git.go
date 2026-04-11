package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/dtuit/ws/internal/manifest"
)

// RepoStatus holds the result of querying a single repo's git state.
type RepoStatus struct {
	Name      string
	Branch    string
	Staged    bool // indexed changes ready to commit
	Unstaged  bool // working tree changes
	Untracked bool // untracked files
	Stashed   bool // stash entries exist
	Ahead     int  // commits ahead of upstream
	Behind    int  // commits behind upstream
	NoRemote  bool // no upstream tracking branch
	CommitMsg string
	CommitAge string
	Err       error
}

// LocalBranchInfo holds metadata for one local branch in a checkout.
type LocalBranchInfo struct {
	Name      string
	Current   bool
	Ahead     int
	Behind    int
	NoRemote  bool
	CommitMsg string
	CommitAge string
}

// BranchList holds the local branch listing for a single checkout.
type BranchList struct {
	Name     string
	Branches []LocalBranchInfo
	Err      error
}

// Symbols returns a compact status string like gita: +*?$ for dirty indicators.
func (s RepoStatus) Symbols() string {
	var b strings.Builder
	if s.Staged {
		b.WriteByte('+')
	}
	if s.Unstaged {
		b.WriteByte('*')
	}
	if s.Untracked {
		b.WriteByte('?')
	}
	if s.Stashed {
		b.WriteByte('$')
	}
	return b.String()
}

// SyncSymbol returns the remote sync indicator.
func (s RepoStatus) SyncSymbol() string {
	switch {
	case s.NoRemote:
		return "~"
	case s.Ahead > 0 && s.Behind > 0:
		return fmt.Sprintf("%d⇕%d", s.Ahead, s.Behind)
	case s.Ahead > 0:
		return fmt.Sprintf("↑%d", s.Ahead)
	case s.Behind > 0:
		return fmt.Sprintf("↓%d", s.Behind)
	default:
		return "="
	}
}

// IsDirty returns true if the working tree has any local changes.
func (s RepoStatus) IsDirty() bool {
	return s.Staged || s.Unstaged || s.Untracked
}

// gitCmd runs a git command in the given directory and returns trimmed stdout.
func gitCmd(dir string, args ...string) (string, error) {
	out, err := gitCmdRaw(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// gitCmdRaw runs a git command in the given directory and returns raw stdout.
func gitCmdRaw(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

// Status queries the git status of a single repo.
func Status(repoDir, name string) RepoStatus {
	s := RepoStatus{Name: name}

	if _, err := GitDir(repoDir); err != nil {
		s.Err = ErrNotCloned
		return s
	}

	// Single command: branch, ahead/behind, staged, unstaged, untracked
	statusOut, err := gitCmd(repoDir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		s.Err = err
		return s
	}
	parseStatusV2(&s, statusOut)

	stashed, err := HasStash(repoDir)
	if err != nil {
		s.Err = err
		return s
	}
	s.Stashed = stashed

	// Last commit message + age
	logOut, err := gitCmd(repoDir, "log", "-1", "--format=%s\x1f%ar")
	if err != nil {
		s.CommitMsg = "(no commits)"
		return s
	}
	parts := strings.SplitN(logOut, "\x1f", 2)
	if len(parts) == 2 {
		s.CommitMsg = parts[0]
		s.CommitAge = parts[1]
	}

	return s
}

// parseStatusV2 parses `git status --porcelain=v2 --branch` output into RepoStatus.
// Format: https://git-scm.com/docs/git-status#_porcelain_format_version_2
func parseStatusV2(s *RepoStatus, output string) {
	s.NoRemote = true // assume no remote until we see branch.upstream

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head == "(detached)" {
				s.Branch = "(detached)"
			} else {
				s.Branch = head
			}

		case strings.HasPrefix(line, "# branch.upstream "):
			s.NoRemote = false

		case strings.HasPrefix(line, "# branch.ab "):
			// "# branch.ab +3 -1" means ahead 3, behind 1
			ab := strings.TrimPrefix(line, "# branch.ab ")
			parts := strings.Fields(ab)
			if len(parts) == 2 {
				s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
				s.Behind, _ = strconv.Atoi(strings.TrimPrefix(parts[1], "-"))
			}

		case strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 "):
			// Changed entry: "1 XY ..." or "2 XY ..." (renamed)
			if len(line) >= 4 {
				x, y := line[2], line[3]
				if x != '.' {
					s.Staged = true
				}
				if y != '.' {
					s.Unstaged = true
				}
			}

		case strings.HasPrefix(line, "? "):
			s.Untracked = true

		case strings.HasPrefix(line, "u "):
			// Unmerged entry - count as both staged and unstaged
			s.Staged = true
			s.Unstaged = true
		}
	}
}

// Workers returns the effective worker count: WS_WORKERS env, or min(cpus, repoCount).
// Always returns at least 1.
func Workers(repoCount int) int {
	if env := os.Getenv("WS_WORKERS"); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			return n
		}
	}
	if repoCount <= 0 {
		return 1
	}
	cpus := runtime.NumCPU()
	if repoCount < cpus {
		return repoCount
	}
	return cpus
}

// StatusAll queries git status for multiple repos in parallel.
func StatusAll(repos []manifest.RepoInfo, maxWorkers int) []RepoStatus {
	results := make([]RepoStatus, len(repos))
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r manifest.RepoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = Status(r.Path, r.Name)
		}(i, repo)
	}

	wg.Wait()
	return results
}

const localBranchFormat = "%(if)%(HEAD)%(then)*%(else) %(end)\t%(refname:short)\t%(upstream:short)\t%(upstream:track)\t%(subject)\t%(committerdate:relative)"

// LocalBranches queries the local branch list for a single repo/worktree.
func LocalBranches(repoDir, name string) BranchList {
	info := BranchList{Name: name}

	if _, err := GitDir(repoDir); err != nil {
		info.Err = ErrNotCloned
		return info
	}

	branchOut, err := gitCmdRaw(repoDir, "for-each-ref", "--format="+localBranchFormat, "refs/heads")
	if err != nil {
		info.Err = err
		return info
	}

	branches, err := parseLocalBranchList(branchOut)
	if err != nil {
		info.Err = err
		return info
	}
	info.Branches = branches
	return info
}

// LocalBranchesAll queries the local branch list for multiple repos in parallel.
func LocalBranchesAll(repos []manifest.RepoInfo, maxWorkers int) []BranchList {
	results := make([]BranchList, len(repos))
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r manifest.RepoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = LocalBranches(r.Path, r.Name)
		}(i, repo)
	}

	wg.Wait()
	return results
}

func parseLocalBranchList(output string) ([]LocalBranchInfo, error) {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	branches := make([]LocalBranchInfo, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(line, "\t", 6)
		if len(fields) != 6 {
			return nil, fmt.Errorf("unexpected branch metadata output")
		}

		ahead, behind, noRemote := parseLocalBranchTrack(fields[2], fields[3])
		branches = append(branches, LocalBranchInfo{
			Name:      fields[1],
			Current:   strings.TrimSpace(fields[0]) == "*",
			Ahead:     ahead,
			Behind:    behind,
			NoRemote:  noRemote,
			CommitMsg: fields[4],
			CommitAge: fields[5],
		})
	}

	return branches, nil
}

func parseLocalBranchTrack(upstream, track string) (ahead, behind int, noRemote bool) {
	upstream = strings.TrimSpace(upstream)
	track = strings.TrimSpace(track)

	switch {
	case upstream == "":
		return 0, 0, true
	case track == "" || track == "[]":
		return 0, 0, false
	case track == "[gone]":
		return 0, 0, true
	}

	track = strings.TrimPrefix(track, "[")
	track = strings.TrimSuffix(track, "]")
	for _, part := range strings.Split(track, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ahead "):
			ahead, _ = strconv.Atoi(strings.TrimPrefix(part, "ahead "))
		case strings.HasPrefix(part, "behind "):
			behind, _ = strconv.Atoi(strings.TrimPrefix(part, "behind "))
		}
	}

	return ahead, behind, false
}

// Exec runs a command in each repo dir in parallel, printing prefixed output.
// Only suppresses git credential prompts when the command is git.
// Returns the number of repos that failed.
func Exec(repos []manifest.RepoInfo, cmdArgs []string, maxWorkers int) int {
	isGit := len(cmdArgs) > 0 && cmdArgs[0] == "git"
	return RunAll(repos, cmdArgs, maxWorkers, RunOpts{
		Verb:          "running",
		GitPrompt:     !isGit, // only suppress prompts for git commands
		ColorPrefixes: true,
	})
}

// Clone clones a single repo.
func Clone(repo manifest.RepoInfo) error {
	cmd := exec.Command("git", "clone", "-b", repo.Branch, "--single-branch", "--", repo.URL, repo.Path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
