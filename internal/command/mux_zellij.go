package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func (z *zellijMux) CreateAndAttach(name string, wsHome string, opts MuxCreateOpts) error {
	if z.IsInside() {
		return fmt.Errorf("already inside a zellij session; detach first or use a separate terminal")
	}

	layoutPath := filepath.Join(wsHome, ".ws-mux-layout.kdl")
	kdl := generateKDLLayout(opts.Windows, opts.Bars)
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

func (z *zellijMux) ActiveSession() (string, error) {
	name := os.Getenv("ZELLIJ_SESSION_NAME")
	if name == "" {
		return "", fmt.Errorf("not inside a zellij session")
	}
	return name, nil
}

func (z *zellijMux) ActiveWindow(session string) (string, error) {
	cmd := exec.Command(z.bin, "action", "list-panes", "--json", "--tab", "--geometry")
	cmd.Env = append(os.Environ(), "ZELLIJ_SESSION_NAME="+session)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("list-panes: %w", err)
	}

	var panes []zellijPane
	if err := json.Unmarshal(out, &panes); err != nil {
		return "", fmt.Errorf("parsing list-panes: %w", err)
	}

	for _, p := range panes {
		if p.IsFocused {
			return p.TabName, nil
		}
	}
	return "", fmt.Errorf("could not determine active tab; specify the window name explicitly")
}

func (z *zellijMux) AddWindow(session string, win MuxWindowSpec) error {
	env := append(os.Environ(), "ZELLIJ_SESSION_NAME="+session)

	// Create the new tab with the first pane's directory.
	newTabArgs := []string{"action", "new-tab", "--name", win.Name}
	if len(win.Panes) > 0 && win.Panes[0].Dir != "" {
		newTabArgs = append(newTabArgs, "--cwd", win.Panes[0].Dir)
	}
	cmd := exec.Command(z.bin, newTabArgs...)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("new-tab: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Split for additional panes.
	if len(win.Panes) > 1 {
		direction := "down"
		if win.Layout == "even-horizontal" || win.Layout == "tiled" {
			direction = "right"
		}
		for _, pane := range win.Panes[1:] {
			splitArgs := []string{"action", "new-pane", "--direction", direction}
			if pane.Dir != "" {
				splitArgs = append(splitArgs, "--cwd", pane.Dir)
			}
			cmd := exec.Command(z.bin, splitArgs...)
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("new-pane: %s: %w", strings.TrimSpace(string(out)), err)
			}
		}
	}

	return nil
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
	// Use delete-session --force so it works for both running and exited sessions.
	out, err := exec.Command(z.bin, "delete-session", "--force", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete-session: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (z *zellijMux) ListWindows(session string) ([]MuxWindowSpec, error) {
	// Use ZELLIJ_SESSION_NAME to target a specific session from outside.
	cmd := exec.Command(z.bin, "action", "list-panes", "--json", "--tab", "--geometry")
	cmd.Env = append(os.Environ(), "ZELLIJ_SESSION_NAME="+session)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list-panes: %w", err)
	}

	var panes []zellijPane
	if err := json.Unmarshal(out, &panes); err != nil {
		return nil, fmt.Errorf("parsing list-panes output: %w", err)
	}

	// Group non-plugin panes by tab, preserving tab order.
	type tabInfo struct {
		name     string
		position int
		panes    []zellijPanePos
	}
	tabs := make(map[int]*tabInfo)
	for _, p := range panes {
		if p.IsPlugin {
			continue
		}
		tab, ok := tabs[p.TabID]
		if !ok {
			tab = &tabInfo{name: p.TabName, position: p.TabPosition}
			tabs[p.TabID] = tab
		}
		tab.panes = append(tab.panes, zellijPanePos{
			spec: MuxPaneSpec{Dir: p.PaneCwd},
			x:    p.PaneX,
			y:    p.PaneY,
			cols: p.PaneColumns,
			rows: p.PaneRows,
		})
	}

	sorted := make([]*tabInfo, 0, len(tabs))
	for _, tab := range tabs {
		sorted = append(sorted, tab)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].position < sorted[j].position })

	specs := make([]MuxWindowSpec, 0, len(sorted))
	for _, tab := range sorted {
		layout := inferLayoutFromPositions(tab.panes)
		paneSpecs := make([]MuxPaneSpec, len(tab.panes))
		var dims []int
		for i, p := range tab.panes {
			paneSpecs[i] = p.spec
			if layout == "even-vertical" {
				dims = append(dims, p.rows)
			} else {
				dims = append(dims, p.cols)
			}
		}
		computePaneSizePercents(paneSpecs, dims)
		specs = append(specs, MuxWindowSpec{
			Name:   tab.name,
			Panes:  paneSpecs,
			Layout: layout,
		})
	}
	return specs, nil
}

// zellijPane is the JSON structure returned by zellij action list-panes --json --tab --geometry.
type zellijPane struct {
	IsFocused   bool   `json:"is_focused"`
	IsPlugin    bool   `json:"is_plugin"`
	TabID       int    `json:"tab_id"`
	TabPosition int    `json:"tab_position"`
	TabName     string `json:"tab_name"`
	PaneCwd     string `json:"pane_cwd"`
	PaneX       int    `json:"pane_x"`
	PaneY       int    `json:"pane_y"`
	PaneColumns int    `json:"pane_columns"`
	PaneRows    int    `json:"pane_rows"`
}

// zellijPanePos pairs a resolved pane spec with its on-screen coordinates and size.
type zellijPanePos struct {
	spec       MuxPaneSpec
	x, y       int
	cols, rows int
}

// layoutToZellijSplit converts a ws layout name to a zellij split_direction value.
func layoutToZellijSplit(layout string) string {
	switch layout {
	case "even-vertical":
		return "horizontal"
	default:
		// "even-horizontal" and "tiled" both use vertical (side-by-side)
		return "vertical"
	}
}

// inferLayoutFromPositions determines the pane layout from pane coordinates.
// Same Y → side by side (even-horizontal), same X → stacked (even-vertical), mixed → tiled.
func inferLayoutFromPositions(panes []zellijPanePos) string {
	if len(panes) <= 1 {
		return ""
	}
	allSameY := true
	allSameX := true
	for _, p := range panes[1:] {
		if p.y != panes[0].y {
			allSameY = false
		}
		if p.x != panes[0].x {
			allSameX = false
		}
	}
	if allSameY {
		return "even-horizontal"
	}
	if allSameX {
		return "even-vertical"
	}
	return "tiled"
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

// userShell returns the user's preferred shell from $SHELL, falling back to "bash".
func userShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "bash"
}

// generateKDLLayout builds a zellij KDL layout string from window specs.
func generateKDLLayout(windows []MuxWindowSpec, bars bool) string {
	var b strings.Builder
	b.WriteString("layout {\n")
	if bars {
		// Use default_tab_template so every tab gets the bars automatically.
		b.WriteString("    default_tab_template {\n")
		b.WriteString("        pane size=1 borderless=true {\n")
		b.WriteString("            plugin location=\"tab-bar\"\n")
		b.WriteString("        }\n")
		b.WriteString("        children\n")
		b.WriteString("        pane size=2 borderless=true {\n")
		b.WriteString("            plugin location=\"status-bar\"\n")
		b.WriteString("        }\n")
		b.WriteString("    }\n")
	}
	for _, win := range windows {
		fmt.Fprintf(&b, "    tab name=%q", win.Name)
		if len(win.Panes) > 1 {
			fmt.Fprintf(&b, " split_direction=%q", layoutToZellijSplit(win.Layout))
		}
		b.WriteString(" {\n")
		for _, pane := range win.Panes {
			sizeAttr := ""
			if pane.Size > 0 {
				sizeAttr = fmt.Sprintf(" size=\"%d%%\"", pane.Size)
			}
			if pane.Cmd != "" {
				// Wrap through the user's shell so the command gets full
				// initialization (rc files, aliases, env vars).
				// "exec $shell" drops back to an interactive shell when done.
				shell := userShell()
				shellCmd := pane.Cmd + "; exec " + shell
				fmt.Fprintf(&b, "        pane%s cwd=%q command=%q {\n", sizeAttr, pane.Dir, shell)
				fmt.Fprintf(&b, "            args \"-ic\" %q\n", shellCmd)
				b.WriteString("        }\n")
			} else {
				fmt.Fprintf(&b, "        pane%s cwd=%q\n", sizeAttr, pane.Dir)
			}
		}
		b.WriteString("    }\n")
	}
	b.WriteString("}\n")
	return b.String()
}
