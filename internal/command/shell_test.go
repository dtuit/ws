package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellBlockUsesShellInitCommand(t *testing.T) {
	block := shellBlock("/workspace")

	assert.Contains(t, block, `export WS_HOME="/workspace"`)
	assert.Contains(t, block, `eval "$(ws shell init)"`)
	assert.NotContains(t, block, `eval "$(ws init)"`)
}
