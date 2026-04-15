package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextWindowName_Basic(t *testing.T) {
	windows := []MuxWindowSpec{
		{Name: "editor"},
		{Name: "shell"},
	}
	assert.Equal(t, "editor-2", nextWindowName("editor", windows))
}

func TestNextWindowName_SkipsExisting(t *testing.T) {
	windows := []MuxWindowSpec{
		{Name: "editor"},
		{Name: "editor-2"},
		{Name: "shell"},
	}
	assert.Equal(t, "editor-3", nextWindowName("editor", windows))
}

func TestNextWindowName_SkipsMultipleExisting(t *testing.T) {
	windows := []MuxWindowSpec{
		{Name: "editor"},
		{Name: "editor-2"},
		{Name: "editor-3"},
	}
	assert.Equal(t, "editor-4", nextWindowName("editor", windows))
}

func TestNextWindowName_NoPriorDups(t *testing.T) {
	windows := []MuxWindowSpec{
		{Name: "code"},
	}
	assert.Equal(t, "code-2", nextWindowName("code", windows))
}
