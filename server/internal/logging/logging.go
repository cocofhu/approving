// Package logging configures the process-wide structured logger.
//
// Logging shape:
//   - single-line JSON to stdout
//   - fixed fields: level (lowercase), msg, ts (RFC3339Nano)
//   - high-cardinality values (run_id, node_id, trace_id, ...) are JSON
//     fields, never stream labels
//   - LOG_PRETTY=1 switches to a human console writer for local dev only
//
// Secrets (tokens, credentials, Authorization) must never be passed as
// fields; callers are responsible for redaction at the call site.
package logging

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Setup builds the global logger and returns it. Production emits JSON
// (the only shape log-center / Promtail can parse); LOG_PRETTY=1 flips
// to a console writer for readability during local development.
func Setup() zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"

	level := parseLevel(os.Getenv("LOG_LEVEL"))
	zerolog.SetGlobalLevel(level)

	var lg zerolog.Logger
	if strings.TrimSpace(os.Getenv("LOG_PRETTY")) == "1" {
		lg = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
			With().Timestamp().Logger()
	} else {
		lg = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	lg = lg.With().Str("svc", "approving").Logger()
	zerolog.DefaultContextLogger = &lg
	return lg
}

func parseLevel(raw string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
