package command

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const shellMarkerBegin = "# BEGIN ws"
const shellMarkerEnd = "# END ws"

func shellBlock(wsHome string) string {
	return fmt.Sprintf("%s\nexport WS_HOME=%q\neval \"$(ws shell init)\"\n%s",
		shellMarkerBegin, wsHome, shellMarkerEnd)
}

// InstallShellConfig writes or updates the shell config block for ws integration.
func InstallShellConfig(wsHome string) error {
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
