package common

import (
	"encoding/json"
	"testing"
)

func TestBaseOnEvent(t *testing.T) {
	var b Base
	in := json.RawMessage(`{"a":1}`)
	out, keep := b.OnEvent(in)
	if !keep || string(out) != string(in) {
		t.Fatalf("OnEvent passthrough failed: keep=%v out=%s", keep, out)
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("X_A", "")
	t.Setenv("X_B", "bee")
	if FirstNonEmptyEnv("X_A", "X_B") != "bee" {
		t.Fatal("FirstNonEmptyEnv")
	}
	if FirstNonEmptyEnv("X_MISSING") != "" {
		t.Fatal("empty")
	}
	env := []string{"A=1", "B=2"}
	if GetEnvValue(env, "B") != "2" {
		t.Fatal("GetEnvValue")
	}
	env = SetIfEmpty(env, "B", "9")
	if GetEnvValue(env, "B") != "2" {
		t.Fatal("SetIfEmpty must not overwrite")
	}
	env = SetIfEmpty(env, "C", "3")
	if GetEnvValue(env, "C") != "3" {
		t.Fatal("SetIfEmpty insert")
	}
	env = SetIfEmpty(env, "D", "")
	if GetEnvValue(env, "D") != "" {
		t.Fatal("empty val skipped")
	}
	env = UpsertEnv(env, "A", "10")
	if GetEnvValue(env, "A") != "10" {
		t.Fatal("UpsertEnv replace")
	}
}
