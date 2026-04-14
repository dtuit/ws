package command

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

// Mux is the backend-agnostic terminal multiplexer interface.
type Mux interface {
	// HasSession reports whether a named session exists.
	HasSession(name string) (bool, error)
	// CreateAndAttach creates a new session with the given windows and attaches to it.
	// This is a single operation because some backends (zellij) cannot create detached.
	CreateAndAttach(name string, wsHome string, windows []MuxWindowSpec) error
	// Attach attaches to (or switches to) an existing session.
	Attach(name string) error
	// Kill destroys a session.
	Kill(name string) error
	// List prints all sessions, optionally highlighting one.
	List(highlight string) error
	// IsInside reports whether the current process is inside this multiplexer.
	IsInside() bool
	// Name returns the backend name ("tmux" or "zellij").
	Name() string
}

// MuxWindowSpec is a resolved window ready for creation.
type MuxWindowSpec struct {
	Name   string
	Panes  []MuxPaneSpec
	Layout string // "tiled", "even-horizontal", "even-vertical"
}

// MuxPaneSpec is a single pane within a window.
type MuxPaneSpec struct {
	Dir string // absolute path
	Cmd string // optional command to run
}

// MuxAttachOrCreate attaches to an existing workspace session,
// or creates one with the configured layout and attaches.
func MuxAttachOrCreate(m *manifest.Manifest, wsHome string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	session := muxSessionName(m, wsHome)

	exists, err := mux.HasSession(session)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf("Attaching to %s session %q\n", mux.Name(), session)
		return mux.Attach(session)
	}

	windows, err := buildMuxLayout(m, wsHome)
	if err != nil {
		return err
	}

	fmt.Printf("Creating %s session %q (%d windows)\n", mux.Name(), session, len(windows))
	return mux.CreateAndAttach(session, wsHome, windows)
}

// MuxKill kills the workspace multiplexer session.
func MuxKill(m *manifest.Manifest, wsHome string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	session := muxSessionName(m, wsHome)

	exists, err := mux.HasSession(session)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s session %q does not exist", mux.Name(), session)
	}

	if err := mux.Kill(session); err != nil {
		return err
	}
	fmt.Printf("Killed %s session %q\n", mux.Name(), session)
	return nil
}

// MuxList lists multiplexer sessions.
func MuxList(m *manifest.Manifest, wsHome string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	session := muxSessionName(m, wsHome)
	return mux.List(session)
}

// muxSessionName returns the sanitized session name for the workspace.
func muxSessionName(m *manifest.Manifest, wsHome string) string {
	if m.Mux.Session != "" {
		return sanitizeMuxSessionName(m.Mux.Session)
	}
	return sanitizeMuxSessionName(filepath.Base(wsHome))
}

func sanitizeMuxSessionName(name string) string {
	// tmux disallows dots and colons in session names
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, ":", "-")
	return name
}

// resolveMuxBackend returns the configured or auto-detected multiplexer.
func resolveMuxBackend(m *manifest.Manifest) (Mux, error) {
	backend := m.Mux.Backend

	if backend != "" {
		switch backend {
		case "tmux":
			return newTmuxMux()
		case "zellij":
			return newZellijMux()
		default:
			return nil, fmt.Errorf("unknown mux backend %q (supported: tmux, zellij)", backend)
		}
	}

	// Auto-detect: prefer zellij, fall back to tmux
	if bin, err := exec.LookPath("zellij"); err == nil && bin != "" {
		return newZellijMux()
	}
	if bin, err := exec.LookPath("tmux"); err == nil && bin != "" {
		return newTmuxMux()
	}

	return nil, fmt.Errorf("no terminal multiplexer found; install tmux or zellij")
}

// buildMuxLayout resolves manifest config into concrete window/pane specs.
func buildMuxLayout(m *manifest.Manifest, wsHome string) ([]MuxWindowSpec, error) {
	windows := m.Mux.Windows

	// Default layout: single window at workspace root
	if len(windows) == 0 {
		return []MuxWindowSpec{
			{
				Name:  "workspace",
				Panes: []MuxPaneSpec{{Dir: wsHome}},
			},
		}, nil
	}

	var specs []MuxWindowSpec
	for _, w := range windows {
		spec, err := resolveWindow(m, wsHome, w)
		if err != nil {
			return nil, fmt.Errorf("window %q: %w", w.Name, err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func resolveWindow(m *manifest.Manifest, wsHome string, w manifest.MuxWindow) (MuxWindowSpec, error) {
	spec := MuxWindowSpec{
		Name:   w.Name,
		Layout: w.Layout,
	}
	if spec.Layout == "" {
		spec.Layout = "tiled"
	}

	if w.Filter != "" {
		repos, err := resolveCommandRepos(m, wsHome, w.Filter, false)
		if err != nil {
			return MuxWindowSpec{}, fmt.Errorf("resolving filter %q: %w", w.Filter, err)
		}

		if len(repos) == 0 {
			// Filter matched nothing — single pane at workspace root
			spec.Panes = []MuxPaneSpec{{Dir: wsHome, Cmd: w.Cmd}}
		} else if w.Split {
			for _, repo := range repos {
				spec.Panes = append(spec.Panes, MuxPaneSpec{Dir: repo.Path, Cmd: w.Cmd})
			}
		} else {
			// No split — use first repo
			spec.Panes = []MuxPaneSpec{{Dir: repos[0].Path, Cmd: w.Cmd}}
		}
		return spec, nil
	}

	dir := wsHome
	if w.Dir != "" {
		dir = resolveWindowDir(m, wsHome, w.Dir)
	}

	spec.Panes = []MuxPaneSpec{{Dir: dir, Cmd: w.Cmd}}
	return spec, nil
}

// resolveWindowDir resolves a dir value: if it matches a repo name, use that
// repo's path; otherwise treat as a path relative to wsHome.
func resolveWindowDir(m *manifest.Manifest, wsHome, dir string) string {
	active := m.ActiveRepos()
	if cfg, ok := active[dir]; ok {
		return m.ResolvePath(wsHome, dir, cfg)
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(wsHome, dir)
}
