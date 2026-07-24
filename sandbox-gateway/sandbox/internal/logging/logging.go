// Package logging provides stdout JSON logging
// (level / msg / ts fields suitable for Loki-style aggregators).
package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Init configures the global zerolog logger and bridges the stdlib log package
// onto the same JSON stdout stream so legacy log.Printf calls stay collectible.
//
// Env:
//   - LOG_LEVEL: debug|info|warn|error (default info)
//   - LOG_PRETTY: "1"/"true" → human-readable console (dev only)
func Init(svc string) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"

	level := parseLevel(firstNonEmpty(os.Getenv("LOG_LEVEL"), "info"))
	zerolog.SetGlobalLevel(level)

	var w io.Writer = os.Stdout
	if truthy(os.Getenv("LOG_PRETTY")) {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}

	log.Logger = zerolog.New(w).With().Timestamp().Str("svc", svc).Logger()
}

// StdBridge returns an io.Writer for log.SetOutput so stdlib log lines become
// single-line JSON {"level":"info","msg":"...","ts":"..."}.
func StdBridge() io.Writer {
	return stdBridge{l: log.Logger}
}

type stdBridge struct{ l zerolog.Logger }

func (b stdBridge) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg == "" {
		return len(p), nil
	}
	level := zerolog.InfoLevel
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "panic"), strings.Contains(lower, "fatal"):
		level = zerolog.ErrorLevel
	case strings.Contains(lower, "失败"), strings.Contains(lower, "error"), strings.Contains(lower, "err:"):
		level = zerolog.ErrorLevel
	case strings.Contains(lower, "warn"), strings.Contains(lower, "超时"):
		level = zerolog.WarnLevel
	}
	b.l.WithLevel(level).Str("via", "stdlib").Msg(msg)
	return len(p), nil
}

func WarnErr(err error, msg string, fields map[string]any) {
	if err == nil {
		return
	}
	ev := log.Warn().Err(err)
	for k, v := range fields {
		ev = ev.Interface(k, v)
	}
	ev.Msg(msg)
}

func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "trace":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
