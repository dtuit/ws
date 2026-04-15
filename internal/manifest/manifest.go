package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is the parsed workspace manifest.
type Manifest struct {
	Root          string              // directory where repos live, relative to manifest dir
	Workspace     string              // VS Code workspace filename (default "ws.code-workspace")
	Scopes        []ScopeDirConfig    // generated symlink directories for scoped repo views
	Worktrees     bool                // default worktree expansion behavior for supported commands
	WorktreeRoot  string              // directory for created worktrees (default ".worktrees")
	Mux           MuxConfig           // terminal multiplexer configuration
	Remotes       map[string]string   // name → URL prefix ("default" is the fallback)
	DefaultBranch string              // default branch for all repos
	Groups        map[string][]string // group name → ordered repo names
	Repos         map[string]RepoConfig
	Exclude       []string
	worktreesSet  bool
	scopesSet     bool
	muxSet        bool
}

const (
	DefaultScopeDir    = ".scope"
	ScopeSourceContext = "context"
	ScopeSourceAll     = "all"
)

// ScopeDirConfig describes one generated symlink directory in the workspace.
type ScopeDirConfig struct {
	Dir    string // workspace-relative path for the generated symlink directory
	Source string // one of ScopeSourceContext or ScopeSourceAll
}

// RepoConfig holds per-repo overrides.
type RepoConfig struct {
	Branch string // empty = use DefaultBranch
	Remote string // empty = use "default" remote
	URL    string // non-empty = full clone URL (overrides Remote)
	Root   string // empty = use manifest Root; relative resolved against wsHome
}

// RepoInfo is a fully resolved repo entry.
type RepoInfo struct {
	Name     string
	URL      string
	Branch   string
	Groups   []string
	Path     string // absolute path to repo on disk
	Worktree string // non-empty when this RepoInfo targets a specific linked worktree
}

// MuxConfig holds terminal multiplexer configuration.
type MuxConfig struct {
	Backend  string                // "tmux", "zellij", or "" (auto-detect)
	Session  string                // session name override (legacy single-session mode)
	Bars     bool                  // show status/help bars (zellij tab-bar + status-bar; tmux status)
	Windows  []MuxWindow           // window/tab layout (legacy single-session mode)
	Sessions map[string]MuxSession // named session configurations
	barsSet  bool
}

// MuxSession is one named session configuration within the mux block.
type MuxSession struct {
	Session string      // multiplexer session name override (default: config key)
	Windows []MuxWindow // window/tab layout
}

// ResolveSession returns the session config and multiplexer session name for a
// given config name. An empty name selects the first (or only) session.
// When using the legacy format (windows at the top level), the config name is empty.
func (c *MuxConfig) ResolveSession(name, wsHome string) (MuxSession, string, error) {
	if len(c.Sessions) > 0 {
		if name == "" {
			// A single-session map has only one key, so iteration is deterministic.
			if len(c.Sessions) == 1 {
				for k, s := range c.Sessions {
					sessionName := k
					if s.Session != "" {
						sessionName = s.Session
					}
					return s, sanitizeMuxName(sessionName), nil
				}
			}
			return MuxSession{}, "", fmt.Errorf("multiple mux sessions configured; specify one by name")
		}
		s, ok := c.Sessions[name]
		if !ok {
			var names []string
			for k := range c.Sessions {
				names = append(names, k)
			}
			return MuxSession{}, "", fmt.Errorf("unknown mux session %q (available: %s)", name, strings.Join(names, ", "))
		}
		sessionName := name
		if s.Session != "" {
			sessionName = s.Session
		}
		return s, sanitizeMuxName(sessionName), nil
	}

	// Legacy format: windows at top level, single session
	if name != "" {
		return MuxSession{}, "", fmt.Errorf("unknown mux session %q (no named sessions configured)", name)
	}
	sessionName := c.Session
	if sessionName == "" {
		sessionName = filepath.Base(wsHome)
	}
	return MuxSession{
		Windows: c.Windows,
	}, sanitizeMuxName(sessionName), nil
}

func sanitizeMuxName(name string) string {
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, ":", "-")
	return name
}

// SessionNames returns the configured session config names, or nil for legacy format.
func (c *MuxConfig) SessionNames() []string {
	if len(c.Sessions) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Sessions))
	for k := range c.Sessions {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// MuxWindow describes one window/tab in a multiplexer session.
type MuxWindow struct {
	Name   string   // tab/window name (required)
	Dir    string   // repo name or relative path
	Filter string   // ws filter string to resolve repos
	Split  bool     // when filter matches multiple repos, create one pane per repo
	Panes  int      // number of panes (for dir-based windows; default 1)
	Cmd    []string // per-pane commands; single value = all panes, list = positional
	Layout string   // pane layout: "tiled", "even-horizontal", "even-vertical"
	Sizes  []int    // pane sizes as percentages (omit for equal distribution)
}

type rawMuxConfig struct {
	Backend  string                    `yaml:"backend"`
	Session  string                    `yaml:"session"`
	Bars     *bool                     `yaml:"bars"`
	Windows  []rawMuxWindow            `yaml:"windows"`
	Sessions map[string]rawMuxSession  `yaml:"sessions"`
}

type rawMuxSession struct {
	Session string         `yaml:"session"`
	Windows []rawMuxWindow `yaml:"windows"`
}

type rawMuxWindow struct {
	Name   string         `yaml:"name"`
	Dir    string         `yaml:"dir"`
	Filter string         `yaml:"filter"`
	Split  bool           `yaml:"split"`
	Panes  int            `yaml:"panes"`
	Cmd    stringOrList   `yaml:"cmd"`
	Layout string         `yaml:"layout"`
	Sizes  []int          `yaml:"sizes"`
}

// stringOrList is a YAML type that accepts either a scalar string or a list of strings.
type stringOrList []string

func (s *stringOrList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var single string
		if err := node.Decode(&single); err != nil {
			return err
		}
		*s = stringOrList{single}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		*s = stringOrList(list)
		return nil
	default:
		return fmt.Errorf("cmd must be a string or list of strings")
	}
}

// rawManifest is the YAML deserialization target.
type rawManifest struct {
	Root      string                       `yaml:"root"`      // where repos live
	Workspace string                       `yaml:"workspace"` // VS Code workspace filename
	Scopes    *[]rawScopeDir               `yaml:"scopes"`    // generated scope symlink directories
	Worktrees    *bool                     `yaml:"worktrees"`      // default worktree behavior
	WorktreeRoot string                    `yaml:"worktree_root"` // directory for created worktrees
	Mux          *rawMuxConfig             `yaml:"mux"`           // terminal multiplexer config
	Remotes   map[string]string            `yaml:"remotes"`   // named remotes
	Branch    string                       `yaml:"branch"`
	Groups    map[string][]string          `yaml:"groups"`
	Repos     map[string]map[string]string `yaml:"repos"`
	Exclude   []string                     `yaml:"exclude"`
}

type rawScopeDir struct {
	Dir    string `yaml:"dir"`
	Source string `yaml:"source"`
}

const maxManifestSize = 1 << 20 // 1MB
const EmptyFilter = "__ws_empty__"

// Load reads and parses a manifest YAML file.
func Load(path string) (*Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if info.Size() > maxManifestSize {
		return nil, fmt.Errorf("manifest too large: %d bytes (max %d)", info.Size(), maxManifestSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	return Parse(data)
}

// Parse parses manifest YAML bytes into a Manifest.
// The primary manifest must set root explicitly.
func Parse(data []byte) (*Manifest, error) {
	return parse(data, true)
}

func parse(data []byte, requireRoot bool) (*Manifest, error) {
	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	m := &Manifest{
		Root:          raw.Root,
		Workspace:     raw.Workspace,
		Scopes:        defaultScopeDirs(),
		Remotes:       make(map[string]string),
		DefaultBranch: raw.Branch,
		Groups:        raw.Groups,
		Repos:         make(map[string]RepoConfig),
		Exclude:       raw.Exclude,
	}

	if requireRoot && strings.TrimSpace(m.Root) == "" {
		return nil, fmt.Errorf("parsing manifest: root is required")
	}
	if m.Workspace == "" {
		m.Workspace = "ws.code-workspace"
	}
	if raw.Scopes != nil {
		scopes, err := parseScopeDirs(*raw.Scopes)
		if err != nil {
			return nil, err
		}
		m.Scopes = scopes
		m.scopesSet = true
	}
	if raw.Worktrees != nil {
		m.Worktrees = *raw.Worktrees
		m.worktreesSet = true
	}
	if raw.WorktreeRoot != "" {
		m.WorktreeRoot = raw.WorktreeRoot
	}
	if raw.Mux != nil {
		m.Mux = parseMuxConfig(*raw.Mux)
		m.muxSet = true
	}
	if m.DefaultBranch == "" {
		m.DefaultBranch = "master"
	}
	if m.Groups == nil {
		m.Groups = make(map[string][]string)
	}
	for name := range m.Groups {
		if err := ValidateName(name); err != nil {
			return nil, fmt.Errorf("group %q: %w", name, err)
		}
	}

	for name, url := range raw.Remotes {
		m.Remotes[name] = url
	}

	// Repos: handle nil values (bare YAML entries like "my-repo:")
	for name, cfg := range raw.Repos {
		if err := ValidateName(name); err != nil {
			return nil, fmt.Errorf("repo %q: %w", name, err)
		}
		rc := RepoConfig{}
		if cfg != nil {
			rc.Branch = cfg["branch"]
			rc.Remote = cfg["remote"]
			rc.URL = cfg["url"]
			rc.Root = cfg["root"]
		}
		m.Repos[name] = rc
	}

	return m, nil
}

// ValidateName ensures a repo or group name is safe to use as a manifest key
// and directory component.
func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name")
	}
	if strings.Contains(name, ",") {
		return fmt.Errorf("must not contain commas")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("must not contain path separators")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("must not contain '..'")
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("must be a simple directory name")
	}
	return nil
}

// LoadWithLocal loads the main manifest and merges the local override if it exists.
func LoadWithLocal(dir string) (*Manifest, error) {
	m, err := Load(filepath.Join(dir, "manifest.yml"))
	if err != nil {
		return nil, err
	}

	localPath := filepath.Join(dir, "manifest.local.yml")
	if _, err := os.Stat(localPath); err == nil {
		if err := m.MergeLocal(localPath); err != nil {
			return nil, fmt.Errorf("loading local overrides: %w", err)
		}
	}

	return m, nil
}

// MergeLocal applies a local override file on top of the manifest.
func (m *Manifest) MergeLocal(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	local, err := parse(data, false)
	if err != nil {
		return err
	}

	// Root: local overrides if explicitly set.
	if local.Root != "" {
		m.Root = local.Root
	}

	// Workspace: local overrides if explicitly set
	if local.Workspace != "ws.code-workspace" {
		m.Workspace = local.Workspace
	}
	if local.scopesSet {
		m.Scopes = append([]ScopeDirConfig(nil), local.Scopes...)
		m.scopesSet = true
	}
	if local.worktreesSet {
		m.Worktrees = local.Worktrees
		m.worktreesSet = true
	}
	if local.WorktreeRoot != "" {
		m.WorktreeRoot = local.WorktreeRoot
	}
	if local.muxSet {
		m.Mux = local.Mux
		m.muxSet = true
	}

	// Remotes: union, local wins on conflict
	for name, url := range local.Remotes {
		m.Remotes[name] = url
	}

	// Repos: union, local wins on conflict
	for name, cfg := range local.Repos {
		m.Repos[name] = cfg
	}

	// Exclude: union
	existing := make(map[string]bool)
	for _, name := range m.Exclude {
		existing[name] = true
	}
	for _, name := range local.Exclude {
		if !existing[name] {
			m.Exclude = append(m.Exclude, name)
		}
	}

	// Groups: local replaces same-name groups, new groups are added
	for name, members := range local.Groups {
		m.Groups[name] = members
	}

	return nil
}

func parseMuxConfig(raw rawMuxConfig) MuxConfig {
	cfg := MuxConfig{
		Backend: raw.Backend,
		Session: raw.Session,
	}
	if raw.Bars != nil {
		cfg.Bars = *raw.Bars
		cfg.barsSet = true
	}
	cfg.Windows = parseMuxWindows(raw.Windows)
	if len(raw.Sessions) > 0 {
		cfg.Sessions = make(map[string]MuxSession, len(raw.Sessions))
		for name, rs := range raw.Sessions {
			cfg.Sessions[name] = MuxSession{
				Session: rs.Session,
				Windows: parseMuxWindows(rs.Windows),
			}
		}
	}
	return cfg
}

func parseMuxWindows(raw []rawMuxWindow) []MuxWindow {
	var windows []MuxWindow
	for _, w := range raw {
		windows = append(windows, MuxWindow{
			Name:   w.Name,
			Dir:    w.Dir,
			Filter: w.Filter,
			Split:  w.Split,
			Panes:  w.Panes,
			Cmd:    []string(w.Cmd),
			Layout: w.Layout,
			Sizes:  w.Sizes,
		})
	}
	return windows
}

func defaultScopeDirs() []ScopeDirConfig {
	return []ScopeDirConfig{
		{Dir: DefaultScopeDir, Source: ScopeSourceContext},
	}
}

func parseScopeDirs(raw []rawScopeDir) ([]ScopeDirConfig, error) {
	scopes := make([]ScopeDirConfig, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for i, cfg := range raw {
		scope, err := normalizeScopeDirConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("scopes[%d]: %w", i, err)
		}
		if seen[scope.Dir] {
			return nil, fmt.Errorf("scopes[%d]: duplicate dir %q", i, scope.Dir)
		}
		seen[scope.Dir] = true
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func normalizeScopeDirConfig(raw rawScopeDir) (ScopeDirConfig, error) {
	dir := filepath.Clean(strings.TrimSpace(raw.Dir))
	if dir == "" || dir == "." {
		return ScopeDirConfig{}, fmt.Errorf("dir is required")
	}
	if filepath.IsAbs(dir) {
		return ScopeDirConfig{}, fmt.Errorf("dir must be relative to the workspace")
	}
	if dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator)) {
		return ScopeDirConfig{}, fmt.Errorf("dir must stay within the workspace")
	}

	source := strings.TrimSpace(raw.Source)
	if source == "" {
		source = ScopeSourceContext
	}
	switch source {
	case ScopeSourceContext, ScopeSourceAll:
	default:
		return ScopeDirConfig{}, fmt.Errorf("unknown source %q", source)
	}

	return ScopeDirConfig{
		Dir:    dir,
		Source: source,
	}, nil
}

// ResolveURL constructs the clone URL for a repo.
func (m *Manifest) ResolveURL(name string, cfg RepoConfig) string {
	if cfg.URL != "" {
		return cfg.URL
	}
	remoteName := cfg.Remote
	if remoteName == "" {
		remoteName = "default"
	}
	prefix := m.Remotes[remoteName]
	if prefix == "" {
		prefix = m.Remotes["default"]
	}
	return prefix + "/" + name + ".git"
}

// ResolveRoot returns the absolute path where repos live.
// If Root is relative, it's resolved against wsHome (the manifest directory).
func (m *Manifest) ResolveRoot(wsHome string) string {
	if filepath.IsAbs(m.Root) {
		return m.Root
	}
	return filepath.Join(wsHome, m.Root)
}

// ResolveWorktreeRoot returns the absolute path where worktrees are stored.
// Defaults to ".worktrees" relative to wsHome.
func (m *Manifest) ResolveWorktreeRoot(wsHome string) string {
	root := m.WorktreeRoot
	if root == "" {
		root = ".worktrees"
	}
	if filepath.IsAbs(root) {
		return root
	}
	return filepath.Join(wsHome, root)
}

// ResolvePath returns the absolute path to a repo on disk.
// Per-repo Root overrides the manifest-level Root.
func (m *Manifest) ResolvePath(wsHome, name string, cfg RepoConfig) string {
	root := m.Root
	if cfg.Root != "" {
		root = cfg.Root
	}
	if filepath.IsAbs(root) {
		return filepath.Join(root, name)
	}
	return filepath.Clean(filepath.Join(wsHome, root, name))
}

// ValidateURL checks that a URL uses a safe git transport scheme.
func ValidateURL(url string) error {
	// Allow SSH shorthand (git@host:org/repo.git)
	if strings.Contains(url, "@") && strings.Contains(url, ":") && !strings.Contains(url, "://") {
		return nil
	}
	allowed := []string{"https://", "ssh://", "git://", "http://"}
	for _, scheme := range allowed {
		if strings.HasPrefix(url, scheme) {
			return nil
		}
	}
	return fmt.Errorf("disallowed URL scheme: %q (allowed: git@..., https://, ssh://, git://)", url)
}

// ResolveBranch returns the effective branch for a repo.
func (m *Manifest) ResolveBranch(cfg RepoConfig) string {
	if cfg.Branch != "" {
		return cfg.Branch
	}
	return m.DefaultBranch
}

// RepoGroups returns a reverse lookup: repo name → list of groups it belongs to.
func (m *Manifest) RepoGroups() map[string][]string {
	result := make(map[string][]string)
	for group, members := range m.Groups {
		for _, name := range members {
			result[name] = append(result[name], group)
		}
	}
	return result
}

// ActiveRepos returns all repos from the Repos map that are not excluded.
// A repo in Repos is never excluded (repos > exclude).
func (m *Manifest) ActiveRepos() map[string]RepoConfig {
	excluded := make(map[string]bool)
	for _, name := range m.Exclude {
		excluded[name] = true
	}

	result := make(map[string]RepoConfig)
	for name, cfg := range m.Repos {
		// Repos in the repos: section are always active (they trump exclude)
		_ = excluded[name]
		result[name] = cfg
	}
	return result
}

// AllRepos returns all active repos as sorted RepoInfo slice.
// wsHome is the directory containing the manifest, used to resolve repo paths.
func (m *Manifest) AllRepos(wsHome string) []RepoInfo {
	active := m.ActiveRepos()
	repoGroups := m.RepoGroups()

	var result []RepoInfo
	for name, cfg := range active {
		result = append(result, RepoInfo{
			Name:   name,
			URL:    m.ResolveURL(name, cfg),
			Branch: m.ResolveBranch(cfg),
			Groups: repoGroups[name],
			Path:   m.ResolvePath(wsHome, name, cfg),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// ResolveFilter resolves a filter string into an ordered list of RepoInfo.
// "" or "all" → all active repos from the merged manifest. Group name → group members. Comma-separated → union.
// wsHome is the directory containing the manifest, used to resolve repo paths.
func (m *Manifest) ResolveFilter(filter, wsHome string) []RepoInfo {
	active := m.ActiveRepos()
	repoGroups := m.RepoGroups()
	if filter == EmptyFilter {
		return nil
	}
	if filter == "" || filter == "all" {
		return m.AllRepos(wsHome)
	}

	seen := make(map[string]bool)
	var result []RepoInfo
	for _, token := range strings.Split(filter, ",") {
		token = strings.TrimSpace(token)
		if members, ok := m.Groups[token]; ok {
			for _, name := range members {
				if _, ok := active[name]; ok && !seen[name] {
					cfg := active[name]
					result = append(result, RepoInfo{
						Name:   name,
						URL:    m.ResolveURL(name, cfg),
						Branch: m.ResolveBranch(cfg),
						Groups: repoGroups[name],
						Path:   m.ResolvePath(wsHome, name, cfg),
					})
					seen[name] = true
				}
			}
		} else if _, ok := active[token]; ok && !seen[token] {
			cfg := active[token]
			result = append(result, RepoInfo{
				Name:   token,
				URL:    m.ResolveURL(token, cfg),
				Branch: m.ResolveBranch(cfg),
				Groups: repoGroups[token],
				Path:   m.ResolvePath(wsHome, token, cfg),
			})
			seen[token] = true
		} else if !seen[token] {
			fmt.Fprintf(os.Stderr, "Warning: '%s' is not a known group or repo\n", token)
		}
	}
	return result
}

// IsGroupOrRepo returns true if the name matches a known group or active repo.
func (m *Manifest) IsGroupOrRepo(name string) bool {
	for _, token := range strings.Split(name, ",") {
		token = strings.TrimSpace(token)
		if _, ok := m.Groups[token]; ok {
			return true
		}
		if _, ok := m.Repos[token]; ok {
			return true
		}
	}
	return false
}
