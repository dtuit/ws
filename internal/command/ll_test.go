package command

import (
	"testing"

	"github.com/dtuit/ws/internal/term"
	"github.com/stretchr/testify/assert"
)

func TestFormatLLName_UsesSeparateWorktreeSuffixColor(t *testing.T) {
	term.SetEnabled(true)
	defer term.SetEnabled(false)

	got := formatLLName("repo@feature", 14, term.Green)

	assert.Equal(t, term.Colorize(term.Green, "repo")+term.Colorize(llWorktreeSuffixColor, "@feature")+"  ", got)
}

func TestFormatLLName_UsesDefaultColorForPrimaryRepo(t *testing.T) {
	term.SetEnabled(true)
	defer term.SetEnabled(false)

	got := formatLLName("repo", 6, term.Green)

	assert.Equal(t, term.Colorize(term.Green, "repo")+"  ", got)
}
