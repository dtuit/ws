package main

import (
	"fmt"
	"os"

	"github.com/dtuit/ws/internal/command"
)

func usage() {
	fmt.Print(command.UsageText())
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
