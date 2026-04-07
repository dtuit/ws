package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

func findWorkspaceHome(override string) (string, error) {
	// 0. -w flag takes priority
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "manifest.yml")); err == nil {
			return abs, nil
		}
		return "", fmt.Errorf("-w %s: no manifest.yml found there", override)
	}

	// 1. Check WS_HOME env var
	if home := os.Getenv("WS_HOME"); home != "" {
		abs, err := filepath.Abs(home)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "manifest.yml")); err == nil {
			return abs, nil
		}
		return "", fmt.Errorf("WS_HOME=%s but no manifest.yml found there", home)
	}

	// 2. Walk up from cwd (max 10 levels to avoid picking up stray manifests)
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "manifest.yml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("manifest.yml not found; set WS_HOME or run from within the workspace")
}

func loadManifestForCompletion(words []string) *manifest.Manifest {
	override := completionWorkspaceOverride(words)
	wsHome, err := findWorkspaceHome(override)
	if err != nil {
		return nil
	}

	m, err := manifest.LoadWithLocal(wsHome)
	if err != nil {
		return nil
	}
	return m
}

func completionWorkspaceOverride(words []string) string {
	for i := 0; i < len(words); i++ {
		switch words[i] {
		case "-w", "--workspace":
			if i+1 >= len(words) {
				return ""
			}
			return strings.TrimSpace(words[i+1])
		case "-t", "-W", "--worktrees", "--no-worktrees":
			continue
		default:
			if !strings.HasPrefix(words[i], "-") {
				return ""
			}
		}
	}
	return ""
}
