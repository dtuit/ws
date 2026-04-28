package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dtuit/ws/internal/version"
)

// upgradeReleaseURL is the GitHub Releases endpoint queried by UpgradeCheck.
// Overridable in tests.
var upgradeReleaseURL = "https://api.github.com/repos/dtuit/ws/releases/latest"

// upgradeHTTPClient is the HTTP client used by UpgradeCheck. Overridable in
// tests; gets a short timeout because this runs in front of the user.
var upgradeHTTPClient = &http.Client{Timeout: 8 * time.Second}

// upgradeCurrentVersion returns the running binary's version string. Defined
// as a func variable so tests can substitute a fixed value.
var upgradeCurrentVersion = func() string { return version.Current().Version }

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// UpgradeCheck queries GitHub for the latest release and compares it to the
// running binary's version. Prints a summary and returns an error only on
// network/parse failures.
func UpgradeCheck() error {
	current := upgradeCurrentVersion()

	req, err := http.NewRequest("GET", upgradeReleaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ws-upgrade-check")

	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GitHub releases API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("parsing release response: %w", err)
	}

	latest := strings.TrimSpace(rel.TagName)
	if latest == "" {
		return fmt.Errorf("GitHub did not return a tag_name")
	}

	cmp := compareSemver(current, latest)
	switch {
	case cmp < 0:
		fmt.Printf("ws %s is out of date — latest is %s\n", current, latest)
		if rel.HTMLURL != "" {
			fmt.Printf("  release notes: %s\n", rel.HTMLURL)
		}
		fmt.Println("  upgrade: curl -LsSf https://raw.githubusercontent.com/dtuit/ws/main/install.sh | sh")
	case cmp == 0:
		fmt.Printf("ws %s is up to date.\n", current)
	default:
		fmt.Printf("ws %s is ahead of the latest release (%s).\n", current, latest)
	}
	return nil
}

// compareSemver returns -1 if a < b, 0 if equal, 1 if a > b. Leading "v",
// trailing pre-release suffixes, and missing components are tolerated; any
// unparsable side sorts as -1 so an unknown current ("dev") flags as "older
// than the release" and surfaces the upgrade hint.
func compareSemver(a, b string) int {
	an, aOK := splitSemver(a)
	bn, bOK := splitSemver(b)
	switch {
	case !aOK && !bOK:
		return 0
	case !aOK:
		return -1
	case !bOK:
		return 1
	}
	for i := 0; i < 3; i++ {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// splitSemver parses "v1.2.3" or "1.2.3-rc1" into [1,2,3]. Returns ok=false
// when no leading numeric component is present.
func splitSemver(s string) ([3]int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return [3]int{}, false
	}
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
