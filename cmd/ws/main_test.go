package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCodeArgs_Default(t *testing.T) {
	filter, err := parseCodeArgs([]string{"backend"}, "")
	require.NoError(t, err)
	assert.Equal(t, "backend", filter)
}

func TestParseCodeArgs_WorktreesFlags(t *testing.T) {
	tests := [][]string{
		{"--worktrees", "backend"},
		{"-t", "backend"},
		{"backend", "--worktrees"},
		{"-t"},
	}

	for _, args := range tests {
		filter, err := parseCodeArgs(args, "ctx")
		require.NoError(t, err)
		if len(args) == 1 && args[0] == "-t" {
			assert.Equal(t, "ctx", filter)
			continue
		}
		assert.Equal(t, "backend", filter)
	}
}

func TestParseCodeArgs_RejectsUnknownFlag(t *testing.T) {
	_, err := parseCodeArgs([]string{"--bogus"}, "")
	require.Error(t, err)
}

func TestParseCodeArgs_RejectsMultipleFilters(t *testing.T) {
	_, err := parseCodeArgs([]string{"backend", "frontend"}, "")
	require.Error(t, err)
}

func TestParseContextArgs_Show(t *testing.T) {
	action, filter, err := parseContextArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, "show", action)
	assert.Equal(t, "", filter)
}

func TestParseContextArgs_Set(t *testing.T) {
	action, filter, err := parseContextArgs([]string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", action)
	assert.Equal(t, "backend", filter)
}

func TestParseContextArgs_Add(t *testing.T) {
	action, filter, err := parseContextArgs([]string{"add", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "add", action)
	assert.Equal(t, "backend,repo-a", filter)
}

func TestParseContextArgs_AddRequiresFilter(t *testing.T) {
	_, _, err := parseContextArgs([]string{"add"})
	require.Error(t, err)
}
