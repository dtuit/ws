package command

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

// BrowseOptions carries caller-controlled flags for Browse.
type BrowseOptions struct {
	Yes bool // skip the VS Code prompt; always launch
}

// Browse resolves a repo name (or ".") to a browsable URL, prints it, and
// hands it to the platform opener. Inside VS Code's integrated terminal,
// prompts before launching unless opts.Yes is set.
func Browse(m *manifest.Manifest, wsHome, arg string, opts BrowseOptions) error {
	name, err := resolveBrowseTarget(m, wsHome, arg)
	if err != nil {
		return err
	}
	url, err := m.BrowseURL(name)
	if err != nil {
		return err
	}
	fmt.Println(url)
	return openInBrowser(url, opts.Yes)
}

func resolveBrowseTarget(m *manifest.Manifest, wsHome, arg string) (string, error) {
	if arg != agentFilterCwd {
		return arg, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving current directory: %w", err)
	}
	index := buildPathIndex(m, wsHome, "", false)
	name, ok := matchSessionRepo(cwd, index)
	if !ok || name == agentRootRepoName {
		return "", fmt.Errorf("current directory is not inside a manifest repo")
	}
	return name, nil
}

func openInBrowser(url string, autoYes bool) error {
	inVSCode := os.Getenv("TERM_PROGRAM") == "vscode"

	if !autoYes && inVSCode && stdinIsTTY() {
		fmt.Print("Open in browser? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp != "y" && resp != "yes" {
			return nil
		}
	}

	// Inside a VS Code integrated terminal (including Remote-SSH), use the
	// editor CLI's --openExternal. Over Remote-SSH this forwards the open
	// request back to the user's local machine so the browser launches
	// there — xdg-open on the remote typically fails because there's no
	// display. WS_EDITOR lets forks like antigravity be used in place of
	// "code".
	if inVSCode {
		if bin, err := exec.LookPath(ResolveEditor("")); err == nil {
			return exec.Command(bin, "--openExternal", url).Start()
		}
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			fmt.Fprintln(os.Stderr, "xdg-open not found; copy the URL above")
			return nil
		}
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
