package sandbox

import (
	"encoding/json"
	"testing"
)

func TestSupportsPreviewAndChanges(t *testing.T) {
	var nilC *Capabilities
	if nilC.SupportsPreview() || nilC.SupportsChanges() {
		t.Fatal("nil caps should be false")
	}
	c := &Capabilities{}
	if c.SupportsPreview() || c.SupportsChanges() {
		t.Fatal("empty caps should be false")
	}
	c.Preview.VNC = true
	c.Changes.Endpoint = "/api/changes"
	if !c.SupportsPreview() || !c.SupportsChanges() {
		t.Fatal("expected supports true")
	}
}

func TestEmbeddedRules(t *testing.T) {
	names, err := EmbeddedRuleBasenames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("expected embedded rule names")
	}
	b, err := ReadEmbeddedRule("rules/" + names[0])
	if err != nil || len(b) == 0 {
		t.Fatalf("ReadEmbeddedRule: %v len=%d", err, len(b))
	}
}

func TestAggregateFrames(t *testing.T) {
	frames := []json.RawMessage{
		json.RawMessage(`{"op":"event","data":{"type":"session_update","update":{"sessionUpdate":"agent_message_chunk","content":{"text":"hello"}}}}`),
		json.RawMessage(`{"type":"session_update","update":{"sessionUpdate":"agent_thought_chunk","content":{"text":"think"}}}`),
		json.RawMessage(`not-json`),
	}
	ev := AggregateFrames(frames)
	if len(ev) == 0 {
		t.Fatal("expected aggregated events")
	}
}
