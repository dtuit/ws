package command

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

func workspaceFileName(base, workspace string) string {
	if workspace == "" {
		return base
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if ext == "" {
		return fmt.Sprintf("%s-%s", stem, workspace)
	}
	return fmt.Sprintf("%s-%s%s", stem, workspace, ext)
}

func workspaceFilePath(m *manifest.Manifest, wsHome, workspace string) string {
	return filepath.Join(wsHome, workspaceFileName(m.Workspace, workspace))
}

func workspaceScopeHintPath(wsHome, workspace string) string {
	return filepath.Join(wsHome, wsStateDir, workspaceScopeHintStateFile(workspace))
}

func workspaceLabel(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return "default"
	}
	return workspace
}

func workspacePresetFilter(m *manifest.Manifest, workspace string) (string, bool) {
	if workspace == "" || m == nil {
		return "", false
	}
	filter, ok := m.Workspaces[workspace]
	return strings.TrimSpace(filter), ok
}

func workspacePresetNames(m *manifest.Manifest) []string {
	if m == nil || len(m.Workspaces) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.Workspaces))
	for name := range m.Workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
