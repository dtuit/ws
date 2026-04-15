package command

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/dtuit/ws/internal/manifest"
)

// Mux is the backend-agnostic terminal multiplexer interface.
type Mux interface {
	// HasSession reports whether a named session exists.
	HasSession(name string) (bool, error)
	// CreateAndAttach creates a new session with the given layout and attaches to it.
	// This is a single operation because some backends (zellij) cannot create detached.
	CreateAndAttach(name string, wsHome string, opts MuxCreateOpts) error
	// Attach attaches to (or switches to) an existing session.
	Attach(name string) error
	// Kill destroys a session.
	Kill(name string) error
	// List prints all sessions, optionally highlighting one.
	List(highlight string) error
	// ListWindows returns the current window/pane layout of an existing session.
	ListWindows(session string) ([]MuxWindowSpec, error)
	// AddWindow creates a new window in an existing session.
	AddWindow(session string, win MuxWindowSpec) error
	// ActiveSession returns the session name the current process is inside.
	ActiveSession() (string, error)
	// ActiveWindow returns the name of the focused window/tab in the given session.
	ActiveWindow(session string) (string, error)
	// IsInside reports whether the current process is inside this multiplexer.
	IsInside() bool
	// Name returns the backend name ("tmux" or "zellij").
	Name() string
}

// MuxCreateOpts holds options for creating a new multiplexer session.
type MuxCreateOpts struct {
	Windows []MuxWindowSpec
	Bars    bool // show status/help bars
}

// MuxWindowSpec is a resolved window ready for creation.
type MuxWindowSpec struct {
	Name   string
	Panes  []MuxPaneSpec
	Layout string // "tiled", "even-horizontal", "even-vertical"
}

// MuxPaneSpec is a single pane within a window.
type MuxPaneSpec struct {
	Dir  string // absolute path
	Cmd  string // optional command to run
	Size int    // percentage of the split dimension (0 = equal distribution)
}

// MuxAttachOrCreate attaches to an existing workspace session,
// or creates one with the configured layout and attaches.
func MuxAttachOrCreate(m *manifest.Manifest, wsHome, sessionName string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	sc, session, err := m.Mux.ResolveSession(sessionName, wsHome)
	if err != nil {
		return err
	}

	exists, err := mux.HasSession(session)
	if err != nil {
		return err
	}

	if exists {
		fmt.Printf("Attaching to %s session %q\n", mux.Name(), session)
		return mux.Attach(session)
	}

	windows, err := buildMuxLayout(m, wsHome, sc.Windows)
	if err != nil {
		return err
	}

	opts := MuxCreateOpts{
		Windows: windows,
		Bars:    m.Mux.Bars,
	}

	fmt.Printf("Creating %s session %q (%d windows)\n", mux.Name(), session, len(windows))
	return mux.CreateAndAttach(session, wsHome, opts)
}

// MuxKill kills the workspace multiplexer session.
func MuxKill(m *manifest.Manifest, wsHome, sessionName string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	_, session, err := m.Mux.ResolveSession(sessionName, wsHome)
	if err != nil {
		return err
	}

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

// MuxDup duplicates a window/tab in the current active session.
// If windowName is empty, the currently focused window is duplicated.
func MuxDup(m *manifest.Manifest, wsHome, windowName string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	session, err := mux.ActiveSession()
	if err != nil {
		return err
	}

	if windowName == "" {
		windowName, err = mux.ActiveWindow(session)
		if err != nil {
			return err
		}
	}

	windows, err := mux.ListWindows(session)
	if err != nil {
		return err
	}

	var target *MuxWindowSpec
	for i := range windows {
		if windows[i].Name == windowName {
			target = &windows[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("window %q not found in session %q", windowName, session)
	}

	newName := nextWindowName(windowName, windows)
	dup := MuxWindowSpec{
		Name:   newName,
		Panes:  make([]MuxPaneSpec, len(target.Panes)),
		Layout: target.Layout,
	}
	copy(dup.Panes, target.Panes)

	if err := mux.AddWindow(session, dup); err != nil {
		return err
	}

	fmt.Printf("Duplicated %s window %q as %q\n", mux.Name(), windowName, newName)
	return nil
}

// nextWindowName returns a unique name by appending a numeric suffix.
func nextWindowName(base string, existing []MuxWindowSpec) string {
	names := make(map[string]bool, len(existing))
	for _, w := range existing {
		names[w.Name] = true
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !names[candidate] {
			return candidate
		}
	}
}

// MuxList lists multiplexer sessions.
func MuxList(m *manifest.Manifest, wsHome string) error {
	mux, err := resolveMuxBackend(m)
	if err != nil {
		return err
	}

	// For list, show all sessions — use empty highlight
	return mux.List("")
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
func buildMuxLayout(m *manifest.Manifest, wsHome string, windows []manifest.MuxWindow) ([]MuxWindowSpec, error) {

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
			spec.Panes = []MuxPaneSpec{{Dir: wsHome, Cmd: paneCmd(w.Cmd, 0)}}
		} else if w.Split {
			for i, repo := range repos {
				spec.Panes = append(spec.Panes, MuxPaneSpec{Dir: repo.Path, Cmd: paneCmd(w.Cmd, i)})
			}
		} else {
			// No split — use first repo
			spec.Panes = []MuxPaneSpec{{Dir: repos[0].Path, Cmd: paneCmd(w.Cmd, 0)}}
		}
		applyPaneSizes(spec.Panes, w.Sizes)
		return spec, nil
	}

	dir := wsHome
	if w.Dir != "" {
		dir = resolveWindowDir(m, wsHome, w.Dir)
	}

	n := w.Panes
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		spec.Panes = append(spec.Panes, MuxPaneSpec{Dir: dir, Cmd: paneCmd(w.Cmd, i)})
	}
	applyPaneSizes(spec.Panes, w.Sizes)
	return spec, nil
}

// paneCmd returns the command for pane at index i.
// If cmd has one entry it applies to all panes (broadcast).
// If cmd has multiple entries, the i-th entry is used; out-of-range returns "".
func paneCmd(cmd []string, i int) string {
	if len(cmd) == 0 {
		return ""
	}
	if len(cmd) == 1 {
		return cmd[0]
	}
	if i < len(cmd) {
		return cmd[i]
	}
	return ""
}

// applyPaneSizes distributes manifest sizes across resolved pane specs.
func applyPaneSizes(panes []MuxPaneSpec, sizes []int) {
	if len(sizes) != len(panes) {
		return
	}
	for i := range panes {
		panes[i].Size = sizes[i]
	}
}

// computePaneSizePercents sets Size on each pane as a rounded percentage.
// If all panes are equal (within 1 cell), sizes are left at 0.
func computePaneSizePercents(panes []MuxPaneSpec, dims []int) {
	if len(dims) != len(panes) || len(panes) <= 1 {
		return
	}
	total := 0
	for _, d := range dims {
		total += d
	}
	if total == 0 {
		return
	}
	allEqual := true
	for _, d := range dims[1:] {
		if diff := d - dims[0]; diff < -1 || diff > 1 {
			allEqual = false
			break
		}
	}
	if allEqual {
		return
	}
	remaining := 100
	for i, d := range dims {
		if i == len(dims)-1 {
			panes[i].Size = remaining
		} else {
			pct := (d * 100) / total
			if pct < 1 {
				pct = 1
			}
			panes[i].Size = pct
			remaining -= pct
		}
	}
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
