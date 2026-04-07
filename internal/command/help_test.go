package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsageTextIncludesShellCommands(t *testing.T) {
	text := UsageText()

	assert.Contains(t, text, "shell init")
	assert.Contains(t, text, "shell install")
	assert.NotContains(t, text, "\n  help ")
	assert.NotContains(t, text, "\n  version ")
}
