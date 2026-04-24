package term

import "os"

var enabled bool

func init() {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return
	}
	enabled = (fi.Mode()&os.ModeCharDevice != 0) && os.Getenv("NO_COLOR") == ""
}

const (
	Reset   = "\033[0m"
	Red     = "\033[31m"
	Blue    = "\033[34m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
)

// Colorize wraps s in the given ANSI code if color output is enabled.
func Colorize(code, s string) string {
	if !enabled {
		return s
	}
	return code + s + Reset
}

// SetEnabled overrides automatic terminal detection.
func SetEnabled(on bool) {
	enabled = on
}

// Enabled reports whether ANSI color output is currently enabled.
func Enabled() bool {
	return enabled
}
