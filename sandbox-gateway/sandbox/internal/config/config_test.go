package config

import "testing"

func TestConfigZero(t *testing.T) {
	var c Config
	if c.ListenAddr != "" || c.GinMode != "" || c.Model != "" {
		t.Fatal("zero values")
	}
	c = Config{ListenAddr: ":8765", GinMode: "release", Model: "auto"}
	if c.ListenAddr != ":8765" {
		t.Fatal(c)
	}
}
