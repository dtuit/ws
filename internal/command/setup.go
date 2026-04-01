package command

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Setup clones missing repos. With installShell, writes shell config to bashrc/zshrc.
func Setup(m *manifest.Manifest, wsHome, filter string, installShell bool) error {
	repos := m.ResolveFilter(filter, wsHome)
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

	if installShell {
		if err := writeShellConfig(wsHome); err != nil {
			return fmt.Errorf("installing shell config: %w", err)
		}
	} else if os.Getenv("WS_HOME") == "" {
		fmt.Printf("\nAdd to your shell config (~/.bashrc or ~/.zshrc):\n\n")
		fmt.Printf("  # BEGIN ws\n")
		fmt.Printf("  export WS_HOME=%q\n", wsHome)
		fmt.Printf("  eval \"$(ws init)\"\n")
		fmt.Printf("  # END ws\n\n")
		fmt.Printf("Or run: ws setup --install-shell\n")
	}

	return nil
}

const shellMarkerBegin = "# BEGIN ws"
const shellMarkerEnd = "# END ws"

func shellBlock(wsHome string) string {
	return fmt.Sprintf("%s\nexport WS_HOME=%q\neval \"$(ws init)\"\n%s",
		shellMarkerBegin, wsHome, shellMarkerEnd)
}

func writeShellConfig(wsHome string) error {
	rcFile := shellRCPath()
	if rcFile == "" {
		return fmt.Errorf("could not determine shell config file")
	}

	block := shellBlock(wsHome)

	content := ""
	if data, err := os.ReadFile(rcFile); err == nil {
		content = string(data)
	}

	re := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(shellMarkerBegin) + `.*?` + regexp.QuoteMeta(shellMarkerEnd))
	if re.MatchString(content) {
		existing := re.FindString(content)
		if existing == block {
			fmt.Printf("Shell config in %s is up to date.\n", rcFile)
			return nil
		}
		content = re.ReplaceAllString(content, block)
		fmt.Printf("Updated shell config in %s\n", rcFile)
	} else {
		if !strings.HasSuffix(content, "\n") && content != "" {
			content += "\n"
		}
		content += "\n" + block + "\n"
		fmt.Printf("Added shell config to %s\n", rcFile)
	}

	return os.WriteFile(rcFile, []byte(content), 0644)
}

func shellRCPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}
