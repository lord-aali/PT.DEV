package logx

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// Level controls fetch and general log verbosity.
type Level int

const (
	LevelDefault Level = iota
	LevelVerbose
	LevelDebug
)

var (
	mu       sync.Mutex
	level    = LevelDefault
	out      io.Writer = os.Stderr
	noColor  bool
	cyan     = color.New(color.FgCyan)
	yellow   = color.New(color.FgYellow)
	green    = color.New(color.FgGreen)
	red      = color.New(color.FgRed)
	magenta  = color.New(color.FgMagenta)
	dim      = color.New(color.FgHiBlack)
	bold     = color.New(color.Bold)
)

// SetLevel parses "verbose" or "debug" (empty = default).
func SetLevel(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default", "info":
		level = LevelDefault
	case "verbose":
		level = LevelVerbose
	case "debug":
		level = LevelDebug
	default:
		return fmt.Errorf("invalid log level %q (use verbose or debug)", s)
	}
	return nil
}

// LevelName returns the active level as a string.
func LevelName() string {
	switch level {
	case LevelVerbose:
		return "verbose"
	case LevelDebug:
		return "debug"
	default:
		return "default"
	}
}

// SetNoColor disables ANSI colors.
func SetNoColor(v bool) {
	noColor = v
	color.NoColor = v || shouldDisableColor()
}

func shouldDisableColor() bool {
	if noColor {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	f, ok := out.(*os.File)
	if !ok {
		return true
	}
	return !isatty.IsTerminal(f.Fd()) && !isatty.IsCygwinTerminal(f.Fd())
}

func initColors() {
	color.NoColor = shouldDisableColor()
}

func write(c *color.Color, prefix, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	initColors()
	msg := fmt.Sprintf(format, args...)
	if prefix != "" {
		_, _ = c.Fprintf(out, "%s %s\n", prefix, msg)
		return
	}
	_, _ = c.Fprintln(out, msg)
}

// Info prints a normal progress line.
func Info(format string, args ...any) {
	write(bold, "", format, args...)
}

// Success prints a green success line.
func Success(format string, args ...any) {
	write(green, "✓", format, args...)
}

// Warn prints a yellow warning.
func Warn(format string, args ...any) {
	write(yellow, "!", format, args...)
}

// Error prints a red error.
func Error(format string, args ...any) {
	write(red, "✗", format, args...)
}

// Verbose prints only when level >= verbose (cyan).
func Verbose(format string, args ...any) {
	if level < LevelVerbose {
		return
	}
	write(cyan, "→", format, args...)
}

// Debug prints only when level >= debug (dim).
func Debug(format string, args ...any) {
	if level < LevelDebug {
		return
	}
	write(dim, "…", format, args...)
}

// DebugStatus prints a magenta status line in debug mode.
func DebugStatus(format string, args ...any) {
	if level < LevelDebug {
		return
	}
	write(magenta, "←", format, args...)
}

// FetchMethodURL logs method + URL at verbose+ (module downloads).
func FetchMethodURL(method, url string) {
	if level < LevelVerbose {
		return
	}
	write(cyan, "→", "%s %s", method, url)
}

// HTTP logs an Apache Common access-log hit: → [http] METHOD path status
func HTTP(method, uri, status string) {
	if level < LevelVerbose {
		return
	}
	if status != "" {
		write(cyan, "→", "[http] %s %s %s", method, uri, status)
		return
	}
	write(cyan, "→", "[http] %s %s", method, uri)
}

// IsDebug reports whether debug logging is on.
func IsDebug() bool { return level >= LevelDebug }

// IsVerbose reports whether verbose or debug is on.
func IsVerbose() bool { return level >= LevelVerbose }

// Writer returns the log output writer.
func Writer() io.Writer { return out }
