package command

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type tmuxMux struct {
	bin string
}

func newTmuxMux() (*tmuxMux, error) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("tmux not found in PATH")
	}
	return &tmuxMux{bin: bin}, nil
}

func (t *tmuxMux) Name() string { return "tmux" }

func (t *tmuxMux) IsInside() bool {
	return os.Getenv("TMUX") != ""
}

func (t *tmuxMux) HasSession(name string) (bool, error) {
	cmd := exec.Command(t.bin, "has-session", "-t", name)
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means session does not exist (not a real error).
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (t *tmuxMux) CreateAndAttach(name string, _ string, windows []MuxWindowSpec) error {
	if len(windows) == 0 {
		return fmt.Errorf("no windows to create")
	}

	// First window: create the session
	first := windows[0]
	firstDir := ""
	if len(first.Panes) > 0 {
		firstDir = first.Panes[0].Dir
	}

	// Use a generous initial size so split-window has room for many panes.
	// The actual size adjusts when a client attaches.
	args := []string{"new-session", "-d", "-s", name, "-x", "200", "-y", "50", "-n", first.Name}
	if firstDir != "" {
		args = append(args, "-c", firstDir)
	}
	if out, err := exec.Command(t.bin, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("new-session: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Additional panes in first window
	if err := t.addPanes(name, first); err != nil {
		return err
	}

	// Remaining windows
	for _, win := range windows[1:] {
		winDir := ""
		if len(win.Panes) > 0 {
			winDir = win.Panes[0].Dir
		}

		args := []string{"new-window", "-t", name, "-n", win.Name}
		if winDir != "" {
			args = append(args, "-c", winDir)
		}
		if out, err := exec.Command(t.bin, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("new-window %q: %s: %w", win.Name, strings.TrimSpace(string(out)), err)
		}

		if err := t.addPanes(name, win); err != nil {
			return err
		}
	}

	// Send commands to panes that have them
	for _, win := range windows {
		for i, pane := range win.Panes {
			if pane.Cmd == "" {
				continue
			}
			target := fmt.Sprintf("%s:%s.%d", name, win.Name, i)
			cmd := exec.Command(t.bin, "send-keys", "-t", target, pane.Cmd, "Enter")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("send-keys %q: %s: %w", target, strings.TrimSpace(string(out)), err)
			}
		}
	}

	// Focus first window
	if out, err := exec.Command(t.bin, "select-window", "-t", name+":0").CombinedOutput(); err != nil {
		return fmt.Errorf("select-window: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return t.Attach(name)
}

// addPanes splits the window for panes beyond the first (which is created
// with the window itself) and applies the layout.
func (t *tmuxMux) addPanes(session string, win MuxWindowSpec) error {
	if len(win.Panes) <= 1 {
		return nil
	}

	layout := win.Layout
	if layout == "" {
		layout = "tiled"
	}
	target := session + ":" + win.Name

	for _, pane := range win.Panes[1:] {
		args := []string{"split-window", "-t", target}
		if pane.Dir != "" {
			args = append(args, "-c", pane.Dir)
		}
		if out, err := exec.Command(t.bin, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("split-window in %q: %s: %w", win.Name, strings.TrimSpace(string(out)), err)
		}

		// Rebalance after each split so the next split has room.
		if out, err := exec.Command(t.bin, "select-layout", "-t", target, layout).CombinedOutput(); err != nil {
			return fmt.Errorf("select-layout %q: %s: %w", layout, strings.TrimSpace(string(out)), err)
		}
	}

	return nil
}

func (t *tmuxMux) Attach(name string) error {
	if t.IsInside() {
		cmd := exec.Command(t.bin, "switch-client", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	cmd := exec.Command(t.bin, "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (t *tmuxMux) Kill(name string) error {
	out, err := exec.Command(t.bin, "kill-session", "-t", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("kill-session: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (t *tmuxMux) List(highlight string) error {
	out, err := exec.Command(t.bin, "list-sessions").CombinedOutput()
	if err != nil {
		// No server running means no sessions.
		if strings.Contains(string(out), "no server running") ||
			strings.Contains(string(out), "error connecting") {
			fmt.Println("No tmux sessions.")
			return nil
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	fmt.Print(string(out))
	return nil
}
