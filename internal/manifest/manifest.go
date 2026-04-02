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
	Root          string              // directory where repos live, relative to manifest dir (default "..")
	Workspace     string              // VS Code workspace filename (default "ws.code-workspace")
	Worktrees     bool                // default worktree expansion behavior for supported commands
	Remotes       map[string]string   // name → URL prefix ("default" is the fallback)
	DefaultBranch string              // default branch for all repos
	Groups        map[string][]string // group name → ordered repo names
	Repos         map[string]RepoConfig
	Exclude       []string
	worktreesSet  bool
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
	Name   string
	URL    string
	Branch string
	Groups []string
	Path   string // absolute path to repo on disk
}

// rawManifest is the YAML deserialization target.
type rawManifest struct {
	Root      string                       `yaml:"root"`      // where repos live (default "..")
	Workspace string                       `yaml:"workspace"` // VS Code workspace filename
	Worktrees *bool                        `yaml:"worktrees"` // default worktree behavior
	Remotes   map[string]string            `yaml:"remotes"`   // named remotes
	Branch    string                       `yaml:"branch"`
	Groups    map[string][]string          `yaml:"groups"`
	Repos     map[string]map[string]string `yaml:"repos"`
	Exclude   []string                     `yaml:"exclude"`
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
func Parse(data []byte) (*Manifest, error) {
	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	m := &Manifest{
		Root:          raw.Root,
		Workspace:     raw.Workspace,
		Remotes:       make(map[string]string),
		DefaultBranch: raw.Branch,
		Groups:        raw.Groups,
		Repos:         make(map[string]RepoConfig),
		Exclude:       raw.Exclude,
	}

	if m.Root == "" {
		m.Root = ".."
	}
	if m.Workspace == "" {
		m.Workspace = "ws.code-workspace"
	}
	if raw.Worktrees != nil {
		m.Worktrees = *raw.Worktrees
		m.worktreesSet = true
	}
	if m.DefaultBranch == "" {
		m.DefaultBranch = "master"
	}
	if m.Groups == nil {
		m.Groups = make(map[string][]string)
	}
	for name := range m.Groups {
		if err := validateName(name); err != nil {
			return nil, fmt.Errorf("group %q: %w", name, err)
		}
	}

	for name, url := range raw.Remotes {
		m.Remotes[name] = url
	}

	// Repos: handle nil values (bare YAML entries like "my-repo:")
	for name, cfg := range raw.Repos {
		if err := validateName(name); err != nil {
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

// validateName ensures a name is safe to use as a directory component.
func validateName(name string) error {
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

	local, err := Parse(data)
	if err != nil {
		return err
	}

	// Root: local overrides if explicitly set (not the default "..")
	if local.Root != ".." {
		m.Root = local.Root
	}

	// Workspace: local overrides if explicitly set
	if local.Workspace != "ws.code-workspace" {
		m.Workspace = local.Workspace
	}
	if local.worktreesSet {
		m.Worktrees = local.Worktrees
		m.worktreesSet = true
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
