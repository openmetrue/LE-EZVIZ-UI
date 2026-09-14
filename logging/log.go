package logging

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

func init() {
	var err error
	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = []string{
		"stdout",
		"./lez.log",
	}
	if Log, err = cfg.Build(); err != nil {
		panic(err)
	}
}

// ParseLevel maps "debug" / "info" / "warn" / "error" to zap levels.
// Empty or unknown values are Info — Debug logs every stream packet (~45 MB/day).
func ParseLevel(s string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return zap.DebugLevel
	case "warn", "warning":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	default:
		return zap.InfoLevel
	}
}

func CreateLogger(logFile, stdout bool, level string) {
	var err error
	cfg := zap.NewDevelopmentConfig()
	cfg.OutputPaths = make([]string, 0, 2)
	if stdout {
		cfg.OutputPaths = append(cfg.OutputPaths, "stdout")
	}
	if logFile {
		cfg.OutputPaths = append(cfg.OutputPaths, "./lez.log")
	}
	cfg.Level.SetLevel(ParseLevel(level))
	Log, err = cfg.Build()
	if err != nil {
		panic(err)
	}
}
