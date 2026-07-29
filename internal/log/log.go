package log

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Level controls which log records are emitted.
type Level int

const (
	LevelOff Level = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

var (
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	current = LevelInfo
)

func init() {
	slog.SetDefault(defaultLogger)
}

// Default returns the package logger configured via InitFromEnv or SetLevel.
func Default() *slog.Logger {
	return defaultLogger
}

// CurrentLevel returns the active log level.
func CurrentLevel() Level {
	return current
}

// InitFromEnv configures the default logger from KILHOG_LOG_LEVEL (default: info).
func InitFromEnv() (Level, error) {
	level, err := ParseLevel(os.Getenv("KILHOG_LOG_LEVEL"))
	if err != nil {
		return LevelInfo, err
	}
	SetLevel(level)
	return level, nil
}

// SetLevel configures the default logger level.
func SetLevel(level Level) {
	current = level
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: toSlogLevel(level),
	}))
	slog.SetDefault(defaultLogger)
}

// ParseLevel parses a log level name (debug, info, warn, error, off).
func ParseLevel(raw string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return LevelInfo, nil
	case "debug":
		return LevelDebug, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "off", "none":
		return LevelOff, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level %q (expected debug, info, warn, error, or off)", raw)
	}
}

// Enabled reports whether records at the given level would be emitted.
func Enabled(level Level) bool {
	return level <= current
}

func toSlogLevel(level Level) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelError + 1
	}
}
