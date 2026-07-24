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

// Init configures the global zerolog logger.
//
// Env:
//   - SBGW_LOG_LEVEL / LOG_LEVEL: debug|info|warn|error (default info)
//   - SBGW_LOG_PRETTY / LOG_PRETTY: "1"/"true" → human-readable console (dev only)
func Init(svc string) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "ts"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "msg"

	level := parseLevel(firstNonEmpty(os.Getenv("SBGW_LOG_LEVEL"), os.Getenv("LOG_LEVEL"), "info"))
	zerolog.SetGlobalLevel(level)

	var w io.Writer = os.Stdout
	if truthy(firstNonEmpty(os.Getenv("SBGW_LOG_PRETTY"), os.Getenv("LOG_PRETTY"))) {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}

	log.Logger = zerolog.New(w).With().Timestamp().Str("svc", svc).Logger()
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

// SaveErr logs a non-nil persist/side-effect error without aborting the caller.
// Use instead of `_ = store.Save(...)` when the primary result already succeeded
// or failed independently — so nothing is silently dropped.
func SaveErr(err error, msg string, fields map[string]any) {
	if err == nil {
		return
	}
	ev := log.Error().Err(err)
	for k, v := range fields {
		ev = ev.Interface(k, v)
	}
	ev.Msg(msg)
}

// WarnErr logs a recoverable/best-effort failure.
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
