package service

import (
	"encoding/json"
	"testing"
)

func TestMcpUpgradeNeeded(t *testing.T) {
	cases := []struct {
		name      string
		current   json.RawMessage
		incoming  json.RawMessage
		wantUpgrade bool
	}{
		{"empty to servers", json.RawMessage(`[]`), json.RawMessage(`[{"name":"artifact-store","type":"http","url":"http://x"}]`), true},
		{"nil to servers", nil, json.RawMessage(`[{"name":"a"}]`), true},
		{"null to servers", json.RawMessage(`null`), json.RawMessage(`[{"name":"a"}]`), true},
		{"servers to servers", json.RawMessage(`[{"name":"a"}]`), json.RawMessage(`[{"name":"b"}]`), false},
		{"empty to empty", json.RawMessage(`[]`), json.RawMessage(`[]`), false},
		{"servers to empty", json.RawMessage(`[{"name":"a"}]`), json.RawMessage(`[]`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpUpgradeNeeded(tc.current, tc.incoming); got != tc.wantUpgrade {
				t.Fatalf("mcpUpgradeNeeded(%s,%s)=%v want %v", tc.current, tc.incoming, got, tc.wantUpgrade)
			}
		})
	}
}
