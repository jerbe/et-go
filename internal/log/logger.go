package log

import (
	"log/slog"
	"os"
)

// Logger 日志封装
type Logger struct {
	*slog.Logger
}

// New 创建日志实例
func New(level string) *Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})
	return &Logger{Logger: slog.New(handler)}
}
