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

func TestShellInitScriptUsesCompletionSentinel(t *testing.T) {
	script := ShellInitScript()

	assert.Contains(t, script, CompletionCommandFallbackSentinel)
	assert.Contains(t, script, "_ws_complete_bash")
	assert.Contains(t, script, "_ws_complete_zsh")
	assert.Contains(t, script, "_ws_prompt_prefix")
	assert.Contains(t, script, "_ws_refresh_prompt")
	assert.Contains(t, script, "WS_CHANGEPS1")
}
