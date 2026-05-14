package command

import (
	"fmt"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

func WorkspaceList(m *manifest.Manifest, _ string) {
	active := activeWorkspaceName()
	if active == "" {
		fmt.Println("Active workspace: default")
	} else {
		fmt.Printf("Active workspace: %s\n", active)
	}

	names := workspacePresetNames(m)
	if len(names) == 0 {
		fmt.Println("No named workspaces configured.")
		return
	}

	fmt.Println("Named workspaces:")
	for _, name := range names {
		marker := " "
		if name == active {
			marker = "*"
		}
		fmt.Printf("%s %-16s %s\n", marker, name, m.Workspaces[name])
	}

	if active != "" {
		if _, ok := m.Workspaces[active]; !ok {
			fmt.Printf("* %-16s (active in shell, not defined in manifest)\n", active)
		}
	}
}

func WorkspaceUse(m *manifest.Manifest, wsHome, name string, includeWorktrees bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("workspace name is required")
	}
	if _, ok := m.Workspaces[name]; !ok {
		return fmt.Errorf("unknown workspace %q", name)
	}

	if err := ensureWorkspaceReady(m, wsHome, name, includeWorktrees); err != nil {
		return err
	}

	fmt.Printf("Workspace %q is ready. The shell wrapper will set WS_WORKSPACE for this terminal.\n", name)
	return nil
}

func WorkspaceClear() {
	fmt.Println("Shell workspace cleared. The shell wrapper will unset WS_WORKSPACE for this terminal.")
}

func OpenWorkspace(m *manifest.Manifest, wsHome, workspace, editor string, includeWorktrees bool) error {
	if workspace == "" {
		workspace = activeWorkspaceName()
	}
	if workspace != "" {
		if err := ensureWorkspaceReady(m, wsHome, workspace, includeWorktrees); err != nil {
			return err
		}
	}
	return Open(m, wsHome, workspace, editor)
}

func ensureWorkspaceReady(m *manifest.Manifest, wsHome, workspace string, includeWorktrees bool) error {
	state, ok, err := loadWorkspaceContextState(wsHome, workspace)
	if err != nil {
		return err
	}
	if ok {
		return applyContext(m, wsHome, workspace, state.Raw, includeWorktrees, applyModeRefresh)
	}

	filter, ok := workspacePresetFilter(m, workspace)
	if !ok {
		return fmt.Errorf("unknown workspace %q", workspace)
	}
	return applyContext(m, wsHome, workspace, filter, includeWorktrees, applyModeSet)
}
