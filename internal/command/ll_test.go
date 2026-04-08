package command

import (
	"strings"
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

func TestFormatLLDetail_TruncatesMessageWhenWidthUnknown(t *testing.T) {
	term.SetEnabled(false)
	defer term.SetEnabled(false)

	msg := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-extra"
	plain, colored := formatLLDetail(msg, " (1 minute ago)", "", 0)

	assert.Contains(t, plain, "...")
	assert.True(t, strings.HasSuffix(plain, " (1 minute ago)"))
	assert.Equal(t, plain, colored)
}

func TestFormatLLDetail_DropsMetaWhenSpaceIsTooTight(t *testing.T) {
	term.SetEnabled(false)
	defer term.SetEnabled(false)

	plain, colored := formatLLDetail("add infer extraction run protobufs", " (1 minute ago)", " [+2 wt]", 12)

	assert.NotContains(t, plain, "(1 minute ago)")
	assert.NotContains(t, plain, "[+2 wt]")
	assert.Equal(t, plain, colored)
	assert.LessOrEqual(t, len([]rune(plain)), 12)
}

func TestLLDetailIndentWidth_LeavesRoomForWrappedMessage(t *testing.T) {
	assert.Equal(t, 20, llDetailIndentWidth(30, 120))
	assert.Equal(t, 10, llDetailIndentWidth(30, 20))
	assert.Equal(t, 2, llDetailIndentWidth(30, 8))
}
