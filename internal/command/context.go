package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

const contextFile = ".ws-context"

// SetContext sets the default filter for all commands.
func SetContext(m *manifest.Manifest, wsHome, filter string) error {
	path := filepath.Join(wsHome, contextFile)

	if filter == "" || filter == "none" || filter == "reset" {
		os.Remove(path)
		fmt.Println("Context cleared.")
		return nil
	}

	// Validate that the filter resolves to something
	repos := m.ResolveFilter(filter)
	if len(repos) == 0 {
		return fmt.Errorf("filter %q matched no repos", filter)
	}

	if err := os.WriteFile(path, []byte(filter+"\n"), 0644); err != nil {
		return err
	}
	fmt.Printf("Context set to %q (%d repos)\n", filter, len(repos))
	return nil
}

// GetContext reads the current context filter, or "" if none is set.
func GetContext(wsHome string) string {
	data, err := os.ReadFile(filepath.Join(wsHome, contextFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ShowContext displays the current context.
func ShowContext(m *manifest.Manifest, wsHome string) {
	ctx := GetContext(wsHome)
	if ctx == "" {
		fmt.Println("No context set (using all grouped repos)")
		return
	}
	repos := m.ResolveFilter(ctx)
	fmt.Printf("Context: %s (%d repos)\n", ctx, len(repos))
}
