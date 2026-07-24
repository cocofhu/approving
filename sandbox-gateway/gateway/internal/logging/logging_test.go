package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestInitJSONFields(t *testing.T) {
	t.Setenv("SBGW_LOG_LEVEL", "debug")
	t.Setenv("SBGW_LOG_PRETTY", "0")
	Init("test-svc")

	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf).With().Timestamp().Str("svc", "test-svc").Logger()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	log.Info().Str("sandbox_id", "abc").Msg("sandbox ready")
	line := buf.String()
	for _, want := range []string{`"level":"info"`, `"msg":"sandbox ready"`, `"sandbox_id":"abc"`, `"svc":"test-svc"`, `"ts":`} {
		if !strings.Contains(line, want) {
			t.Fatalf("missing %s in %s", want, line)
		}
	}
}

func TestSaveErrAndWarnErr(t *testing.T) {
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf).With().Timestamp().Logger()

	SaveErr(nil, "noop", nil)
	if buf.Len() != 0 {
		t.Fatalf("nil err should not log: %s", buf.String())
	}
	SaveErr(os.ErrNotExist, "persist sandbox", map[string]any{"sandbox_id": "x"})
	if !strings.Contains(buf.String(), `"level":"error"`) || !strings.Contains(buf.String(), `"msg":"persist sandbox"`) {
		t.Fatalf("SaveErr output: %s", buf.String())
	}
	buf.Reset()
	WarnErr(os.ErrPermission, "cleanup", map[string]any{"name": "c"})
	if !strings.Contains(buf.String(), `"level":"warn"`) {
		t.Fatalf("WarnErr output: %s", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	if parseLevel("WARN") != zerolog.WarnLevel {
		t.Fatal("warn")
	}
	if parseLevel("nope") != zerolog.InfoLevel {
		t.Fatal("default info")
	}
}
