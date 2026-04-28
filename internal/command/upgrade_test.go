package command

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.13.0", "v0.14.0", -1},
		{"v0.14.0", "v0.13.0", 1},
		{"v0.14.0", "v0.14.0", 0},
		{"0.14.0", "v0.14.0", 0},
		{"v0.14.1", "v0.14.0", 1},
		{"v1.0.0", "v0.99.0", 1},
		{"v0.14.0-rc1", "v0.14.0", 0}, // pre-release suffix dropped, equal main
		{"dev", "v0.14.0", -1},        // unparsable current => out of date
		{"v0.14.0", "dev", 1},
	}
	for _, tc := range cases {
		got := compareSemver(tc.a, tc.b)
		assert.Equal(t, tc.want, got, "compareSemver(%q, %q)", tc.a, tc.b)
	}
}

// withReleaseServer swaps upgradeReleaseURL for a test server that returns
// the given response body, restoring the original URL on test teardown.
func withReleaseServer(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	original := upgradeReleaseURL
	upgradeReleaseURL = srv.URL
	t.Cleanup(func() { upgradeReleaseURL = original })
}

// withCurrentVersion swaps the version source for a fixed value.
func withCurrentVersion(t *testing.T, v string) {
	t.Helper()
	original := upgradeCurrentVersion
	upgradeCurrentVersion = func() string { return v }
	t.Cleanup(func() { upgradeCurrentVersion = original })
}

func TestUpgradeCheck_OutOfDate(t *testing.T) {
	withCurrentVersion(t, "v0.13.0")
	withReleaseServer(t, http.StatusOK, `{"tag_name":"v0.14.0","html_url":"https://example.com/r"}`)
	output := captureCommandStdout(t, func() {
		assert.NoError(t, UpgradeCheck())
	})
	assert.Contains(t, output, "out of date")
	assert.Contains(t, output, "v0.14.0")
	assert.Contains(t, output, "https://example.com/r")
}

func TestUpgradeCheck_UpToDate(t *testing.T) {
	withCurrentVersion(t, "v0.14.0")
	withReleaseServer(t, http.StatusOK, `{"tag_name":"v0.14.0"}`)
	output := captureCommandStdout(t, func() {
		assert.NoError(t, UpgradeCheck())
	})
	assert.Contains(t, output, "is up to date")
}

func TestUpgradeCheck_Ahead(t *testing.T) {
	withCurrentVersion(t, "v0.15.0")
	withReleaseServer(t, http.StatusOK, `{"tag_name":"v0.14.0"}`)
	output := captureCommandStdout(t, func() {
		assert.NoError(t, UpgradeCheck())
	})
	assert.Contains(t, output, "ahead of the latest release")
}

func TestUpgradeCheck_PropagatesHTTPError(t *testing.T) {
	withReleaseServer(t, http.StatusInternalServerError, "boom")
	err := UpgradeCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestUpgradeCheck_RejectsEmptyTag(t *testing.T) {
	withReleaseServer(t, http.StatusOK, `{"tag_name":""}`)
	err := UpgradeCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tag_name")
}
