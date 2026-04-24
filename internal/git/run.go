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
	Verb          string // progress label: "fetching", "pulling", etc.
	Summary       string // summary label: "Fetched", "Pulled", etc.
	Suppress      string // suppress this exact output (e.g. "Already up to date.")
	GitPrompt     bool   // if false, suppress git credential prompts (default: suppress)
	ColorPrefixes bool   // if true, color per-repo prefixes like docker compose logs
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

	prefixes := make([]string, len(repos))
	for i, r := range repos {
		prefixes[i] = formatRunPrefix(r.Name, maxName, i, opts.ColorPrefixes)
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

	// Force git color on; CombinedOutput() hides the TTY so git would otherwise strip ANSI codes.
	effectiveArgs := cmdArgs
	if term.Enabled() && len(cmdArgs) > 0 && cmdArgs[0] == "git" {
		effectiveArgs = append([]string{"git", "-c", "color.ui=always"}, cmdArgs[1:]...)
	}

	for i, repo := range repos {
		wg.Add(1)
		go func(r manifest.RepoInfo, prefix string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if !IsCheckout(r.Path) {
				mu.Lock()
				done++
				fmt.Fprintf(os.Stderr, "%sskipped (not cloned)\n", prefix)
				mu.Unlock()
				return
			}

			cmd := exec.Command(effectiveArgs[0], effectiveArgs[1:]...)
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
		}(repo, prefixes[i])
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

var runPrefixPalette = []string{
	term.Blue,
	term.Yellow,
	term.Green,
	term.Magenta,
	term.Cyan,
}

func formatRunPrefix(name string, width, colorIndex int, colorize bool) string {
	namePart := fmt.Sprintf("%-*s", width, name)
	if !colorize || len(runPrefixPalette) == 0 {
		return namePart + " | "
	}
	return term.Colorize(runPrefixPalette[colorIndex%len(runPrefixPalette)], namePart) + term.Colorize(term.Dim, " | ")
}
