package command

import (
	"os"
	"path/filepath"
)

// wsStateDir is the workspace-scoped directory that holds ws-managed state
// files (context, agent pins, etc.). Lives under $WS_HOME.
const wsStateDir = ".ws"

// stateReadPath returns the path to read a state file from. Prefers the new
// nested path ($WS_HOME/.ws/<newName>) when it exists, otherwise falls back
// to the legacy flat path ($WS_HOME/<legacyName>) so existing workspaces
// keep working until the next write migrates them.
func stateReadPath(wsHome, newName, legacyName string) string {
	newPath := filepath.Join(wsHome, wsStateDir, newName)
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	if legacyName != "" {
		legacyPath := filepath.Join(wsHome, legacyName)
		if _, err := os.Stat(legacyPath); err == nil {
			return legacyPath
		}
	}
	return newPath
}

// stateWritePath returns the new nested path for a state file, creating the
// .ws/ directory if missing and removing any legacy flat file so the state
// migrates on first write.
func stateWritePath(wsHome, newName, legacyName string) (string, error) {
	dir := filepath.Join(wsHome, wsStateDir, filepath.Dir(newName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if legacyName != "" {
		_ = os.Remove(filepath.Join(wsHome, legacyName))
	}
	return filepath.Join(wsHome, wsStateDir, newName), nil
}
