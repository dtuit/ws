package main

import (
	"fmt"
	"strconv"

	"github.com/dtuit/ws/internal/command"
)

func runCompletion(args []string) {
	if len(args) == 0 {
		return
	}

	current, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}

	words := args[1:]
	m := loadManifestForCompletion(words)
	for _, value := range command.CompletionOutput(m, words, current) {
		fmt.Println(value)
	}
}
