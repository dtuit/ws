package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type zellijMux struct {
	bin string
}

func newZellijMux() (*zellijMux, error) {
	bin, err := exec.LookPath("zellij")
	if err != nil {
		return nil, fmt.Errorf("zellij not found in PATH")
	}
	return &zellijMux{bin: bin}, nil
}

func (z *zellijMux) Name() string { return "zellij" }

func (z *zellijMux) IsInside() bool {
	return os.Getenv("ZELLIJ") != ""
}

func (z *zellijMux) HasSession(name string) (bool, error) {
	out, err := exec.Command(z.bin, "list-sessions", "--short", "--no-formatting").Output()
	if err != nil {
		// If zellij server isn't running, there are no sessions.
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// zellij list-sessions --short may include extra info after the name;
		// the session name is the first field.
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true, nil
		}
	}
	return false, nil
}

func (z *zellijMux) CreateAndAttach(name string, wsHome string, windows []MuxWindowSpec) error {
	if z.IsInside() {
		return fmt.Errorf("already inside a zellij session; detach first or use a separate terminal")
	}

	layoutPath := filepath.Join(wsHome, ".ws-mux-layout.kdl")
	kdl := generateKDLLayout(windows)
	if err := os.WriteFile(layoutPath, []byte(kdl), 0644); err != nil {
		return fmt.Errorf("writing layout file: %w", err)
	}

	// Create and attach in one step using the generated layout.
	cmd := exec.Command(z.bin, "--session", name, "--new-session-with-layout", layoutPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (z *zellijMux) Attach(name string) error {
	if z.IsInside() {
		return fmt.Errorf("already inside a zellij session; detach first or use a separate terminal")
	}

	cmd := exec.Command(z.bin, "attach", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (z *zellijMux) Kill(name string) error {
	out, err := exec.Command(z.bin, "kill-session", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("kill-session: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (z *zellijMux) List(highlight string) error {
	out, err := exec.Command(z.bin, "list-sessions").CombinedOutput()
	if err != nil {
		// No server running means no sessions.
		if _, ok := err.(*exec.ExitError); ok {
			fmt.Println("No zellij sessions.")
			return nil
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		fmt.Println("No zellij sessions.")
		return nil
	}
	fmt.Println(output)
	return nil
}

// generateKDLLayout builds a zellij KDL layout string from window specs.
func generateKDLLayout(windows []MuxWindowSpec) string {
	var b strings.Builder
	b.WriteString("layout {\n")
	for _, win := range windows {
		fmt.Fprintf(&b, "    tab name=%q", win.Name)
		if len(win.Panes) > 1 {
			b.WriteString(" split_direction=\"vertical\"")
		}
		b.WriteString(" {\n")
		for _, pane := range win.Panes {
			if pane.Cmd != "" {
				cmdParts := strings.Fields(pane.Cmd)
				fmt.Fprintf(&b, "        pane cwd=%q command=%q", pane.Dir, cmdParts[0])
				if len(cmdParts) > 1 {
					b.WriteString(" {\n")
					b.WriteString("            args")
					for _, arg := range cmdParts[1:] {
						fmt.Fprintf(&b, " %q", arg)
					}
					b.WriteString("\n        }\n")
				} else {
					b.WriteString("\n")
				}
			} else {
				fmt.Fprintf(&b, "        pane cwd=%q\n", pane.Dir)
			}
		}
		b.WriteString("    }\n")
	}
	b.WriteString("}\n")
	return b.String()
}
