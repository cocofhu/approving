package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]zerolog.Level{
		"debug":   zerolog.DebugLevel,
		"warn":    zerolog.WarnLevel,
		"error":   zerolog.ErrorLevel,
		"info":    zerolog.InfoLevel,
		"":        zerolog.InfoLevel,
		" DEBUG ": zerolog.DebugLevel,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSetupJSON(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	lg := Setup()
	lg.Info().Msg("test") // should not panic
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("global level = %v, want debug", zerolog.GlobalLevel())
	}
}

func TestSetupPretty(t *testing.T) {
	t.Setenv("LOG_PRETTY", "1")
	lg := Setup()
	lg.Info().Msg("pretty") // console writer path
}
