package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/term"
)

// RunOpts configures a parallel command run across repos.
type RunOpts struct {
	Verb      string // progress label: "fetching", "pulling", etc.
	Summary   string // summary label: "Fetched", "Pulled", etc.
	Suppress  string // suppress this exact output (e.g. "Already up to date.")
	GitPrompt bool   // if false, suppress git credential prompts (default: suppress)
}

// RunResult is the outcome of running a command in one repo.
type RunResult struct {
	Name   string
	Output string
	Err    error
}

// RunAll runs a command in each repo dir in parallel with progress display.
// Returns the number of repos that failed.
func RunAll(repos []manifest.RepoInfo, cmdArgs []string, maxWorkers int, opts RunOpts) int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)
	failCount := 0
	done := 0
	total := len(repos)

	maxName := 0
	for _, r := range repos {
		if len(r.Name) > maxName {
			maxName = len(r.Name)
		}
	}

	var env []string
	if !opts.GitPrompt {
		env = append(os.Environ(),
			"GIT_TERMINAL_PROMPT=0",
			"GIT_SSH_COMMAND=ssh -o BatchMode=yes",
		)
	}

	fi, _ := os.Stdout.Stat()
	isTTY := fi != nil && fi.Mode()&os.ModeCharDevice != 0

	for _, repo := range repos {
		wg.Add(1)
		go func(r manifest.RepoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			prefix := fmt.Sprintf("%-*s | ", maxName, r.Name)

			if !IsCheckout(r.Path) {
				mu.Lock()
				done++
				fmt.Fprintf(os.Stderr, "%sskipped (not cloned)\n", prefix)
				mu.Unlock()
				return
			}

			cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
			cmd.Dir = r.Path
			cmd.Stdin = nil
			if env != nil {
				cmd.Env = env
			}
			output, err := cmd.CombinedOutput()

			mu.Lock()
			defer mu.Unlock()

			done++
			text := strings.TrimSpace(string(output))

			if isTTY {
				fmt.Fprint(os.Stderr, "\r\033[K")
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "%s%s\n", prefix, term.Colorize(term.Red, "failed: "+err.Error()))
				if text != "" {
					for _, line := range strings.Split(text, "\n") {
						fmt.Fprintf(os.Stderr, "%s%s\n", prefix, line)
					}
				}
				failCount++
			} else if text != "" && text != opts.Suppress {
				for _, line := range strings.Split(text, "\n") {
					fmt.Println(prefix + line)
				}
			}

			if isTTY {
				fmt.Fprintf(os.Stderr, "\r\033[K%s... %d/%d", opts.Verb, done, total)
			}
		}(repo)
	}

	wg.Wait()
	if isTTY {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if opts.Summary != "" {
		fmt.Printf("%s %d repo(s).\n", opts.Summary, total-failCount)
	}
	return failCount
}
