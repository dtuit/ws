package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

const (
	manifestFile      = "manifest.yml"
	localManifestFile = "manifest.local.yml"
)

// SaveContextGroup snapshots the current effective context into a named group.
// By default the group is written to manifest.yml; with local=true it is written
// to manifest.local.yml instead.
func SaveContextGroup(m *manifest.Manifest, wsHome, group string, local bool) error {
	group = strings.TrimSpace(group)
	if err := validateContextGroupName(m, group); err != nil {
		return err
	}

	members, err := currentContextGroupMembers(m, wsHome)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(wsHome, manifestFile)
	targetLabel := manifestFile
	if local {
		targetPath = filepath.Join(wsHome, localManifestFile)
		targetLabel = localManifestFile
	} else if err := validateSharedManifestGroupMembers(wsHome, members); err != nil {
		return fmt.Errorf("cannot save group %q to %s: %w", group, manifestFile, err)
	}

	if err := upsertManifestGroup(targetPath, group, members, local); err != nil {
		return err
	}

	fmt.Printf("Saved current context as group %q in %s (%d repos)\n", group, targetLabel, len(members))
	return nil
}

func validateContextGroupName(m *manifest.Manifest, group string) error {
	if err := manifest.ValidateName(group); err != nil {
		return fmt.Errorf("group %q: %w", group, err)
	}

	switch group {
	case "all", "none", "reset":
		return fmt.Errorf("group %q uses a reserved filter name", group)
	}

	if _, ok := m.ActiveRepos()[group]; ok {
		return fmt.Errorf("group %q conflicts with an existing repo name", group)
	}

	return nil
}

func currentContextGroupMembers(m *manifest.Manifest, wsHome string) ([]string, error) {
	state, ok, err := loadStoredContextState(wsHome)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no context set")
	}

	members, invalid := collapseContextMembers(state.Resolved, m.ActiveRepos())
	if len(invalid) > 0 {
		return nil, fmt.Errorf("current context includes repos not in the manifest: %s", strings.Join(invalid, ", "))
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("current context matched no repos")
	}

	return members, nil
}

func collapseContextMembers(resolved []string, active map[string]manifest.RepoConfig) ([]string, []string) {
	if len(resolved) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool, len(resolved))
	members := make([]string, 0, len(resolved))
	var invalid []string

	for _, entry := range resolved {
		member := strings.TrimSpace(entry)
		if member == "" {
			continue
		}

		if repoName, selector, ok := splitWorktreeToken(member, active); ok && repoName != "" && selector != "" {
			member = repoName
		}

		if _, ok := active[member]; !ok {
			invalid = append(invalid, entry)
			continue
		}
		if seen[member] {
			continue
		}

		seen[member] = true
		members = append(members, member)
	}

	return members, invalid
}

func validateSharedManifestGroupMembers(wsHome string, members []string) error {
	shared, err := manifest.Load(filepath.Join(wsHome, manifestFile))
	if err != nil {
		return err
	}

	active := shared.ActiveRepos()
	var missing []string
	for _, member := range members {
		if _, ok := active[member]; !ok {
			missing = append(missing, member)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("current context includes repos not defined there: %s; use --local to save this group in %s instead", strings.Join(missing, ", "), localManifestFile)
	}
	return nil
}

func upsertManifestGroup(path, group string, members []string, createIfMissing bool) error {
	content, err := readManifestText(path, createIfMissing)
	if err != nil {
		return err
	}

	updated, err := upsertManifestGroupText(content, group, members)
	if err != nil {
		return fmt.Errorf("updating %s: %w", filepath.Base(path), err)
	}

	return os.WriteFile(path, []byte(updated), 0644)
}

func readManifestText(path string, createIfMissing bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && createIfMissing {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func upsertManifestGroupText(content, group string, members []string) (string, error) {
	lineEnding := detectLineEnding(content)
	lines, hasTrailingNewline := splitManifestLines(content)
	groupBlock := renderGroupBlockLines("  ", group, members)

	if len(lines) == 0 {
		return joinManifestLines(append([]string{"groups:"}, groupBlock...), lineEnding, true), nil
	}

	groupsIndex, ok := findTopLevelKeyLine(lines, "groups")
	if !ok {
		lines = appendTopLevelSection(lines, "groups:", groupBlock)
		return joinManifestLines(lines, lineEnding, hasTrailingNewline), nil
	}

	nextTopLevel := len(lines)
	for i := groupsIndex + 1; i < len(lines); i++ {
		if isTopLevelContentLine(lines[i]) {
			nextTopLevel = i
			break
		}
	}

	groupsEnd := trimGroupsSectionEnd(lines, groupsIndex+1, nextTopLevel)
	childIndent := detectGroupChildIndent(lines, groupsIndex+1, groupsEnd)
	if childIndent == "" {
		childIndent = "  "
	}
	groupBlock = renderGroupBlockLines(childIndent, group, members)

	start, end, found := findGroupEntryRange(lines, groupsIndex+1, groupsEnd, childIndent, group)
	if found {
		lines = spliceLines(lines, start, end, groupBlock)
	} else {
		lines = spliceLines(lines, groupsEnd, groupsEnd, groupBlock)
	}

	return joinManifestLines(lines, lineEnding, hasTrailingNewline), nil
}

func detectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func splitManifestLines(content string) ([]string, bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	hasTrailingNewline := strings.HasSuffix(normalized, "\n")
	if hasTrailingNewline {
		normalized = strings.TrimSuffix(normalized, "\n")
	}
	if normalized == "" {
		return nil, hasTrailingNewline
	}
	return strings.Split(normalized, "\n"), hasTrailingNewline
}

func joinManifestLines(lines []string, lineEnding string, hasTrailingNewline bool) string {
	if len(lines) == 0 {
		if hasTrailingNewline {
			return lineEnding
		}
		return ""
	}
	out := strings.Join(lines, lineEnding)
	if hasTrailingNewline {
		out += lineEnding
	}
	return out
}

func renderGroupBlockLines(childIndent, group string, members []string) []string {
	lines := []string{childIndent + group + ":"}
	itemIndent := childIndent + "  "
	for _, member := range members {
		lines = append(lines, itemIndent+"- "+member)
	}
	return lines
}

func appendTopLevelSection(lines []string, header string, block []string) []string {
	out := append([]string{}, lines...)
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, header)
	out = append(out, block...)
	return out
}

func findTopLevelKeyLine(lines []string, key string) (int, bool) {
	for i, line := range lines {
		if leadingIndent(line) != "" {
			continue
		}
		if matchesMappingKey(line, key) {
			return i, true
		}
	}
	return -1, false
}

func isTopLevelContentLine(line string) bool {
	if leadingIndent(line) != "" {
		return false
	}
	trimmed := strings.TrimSpace(line)
	return trimmed != "" && !strings.HasPrefix(trimmed, "#")
}

func trimGroupsSectionEnd(lines []string, start, end int) int {
	for end > start {
		line := lines[end-1]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isTopLevelComment(line) {
			end--
			continue
		}
		break
	}
	return end
}

func detectGroupChildIndent(lines []string, start, end int) string {
	indent := ""
	for i := start; i < end; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lineIndent := leadingIndent(line)
		if lineIndent == "" {
			continue
		}

		if indent == "" || len(lineIndent) < len(indent) {
			indent = lineIndent
		}
	}
	return indent
}

func findGroupEntryRange(lines []string, start, end int, childIndent, group string) (int, int, bool) {
	for i := start; i < end; i++ {
		if !isDirectGroupEntryHeader(lines[i], childIndent, group) {
			continue
		}

		blockEnd := end
		for j := i + 1; j < end; j++ {
			if isAnyDirectGroupEntryHeader(lines[j], childIndent) {
				blockEnd = j
				break
			}
		}
		for blockEnd > i && strings.TrimSpace(lines[blockEnd-1]) == "" {
			blockEnd--
		}

		return i, blockEnd, true
	}
	return 0, 0, false
}

func isDirectGroupEntryHeader(line, childIndent, group string) bool {
	if leadingIndent(line) != childIndent {
		return false
	}
	return matchesMappingKey(strings.TrimSpace(line), group)
}

func isAnyDirectGroupEntryHeader(line, childIndent string) bool {
	if leadingIndent(line) != childIndent {
		return false
	}

	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
		return false
	}

	colon := strings.IndexByte(trimmed, ':')
	if colon <= 0 {
		return false
	}
	rest := trimmed[colon+1:]
	return rest == "" || strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t") || strings.HasPrefix(rest, "#")
}

func matchesMappingKey(line, key string) bool {
	trimmed := strings.TrimSpace(line)
	prefix := key + ":"
	if !strings.HasPrefix(trimmed, prefix) {
		return false
	}
	if len(trimmed) == len(prefix) {
		return true
	}
	next := trimmed[len(prefix)]
	return next == ' ' || next == '\t' || next == '#'
}

func leadingIndent(line string) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i]
}

func isTopLevelComment(line string) bool {
	return leadingIndent(line) == "" && strings.HasPrefix(strings.TrimSpace(line), "#")
}

func spliceLines(lines []string, start, end int, replacement []string) []string {
	out := make([]string, 0, len(lines)-(end-start)+len(replacement))
	out = append(out, lines[:start]...)
	out = append(out, replacement...)
	out = append(out, lines[end:]...)
	return out
}
