package services

import (
	"encoding/json"
	"testing"
)

func TestExtractPmAgentTextNestedOpEvent(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"op": "event",
		"data": map[string]any{
			"type": "session_update",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": "Hello"},
			},
		},
	})
	if got := extractPmAgentText(raw); got != "Hello" {
		t.Fatalf("got %q want Hello", got)
	}
}

func TestExtractPmAgentTextIgnoresThought(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"type": "session_update",
		"update": map[string]any{
			"sessionUpdate": "agent_thought_chunk",
			"content":       map[string]any{"text": "think"},
		},
	})
	if got := extractPmAgentText(raw); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractPmAgentTextParts(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"type": "session_update",
		"update": map[string]any{
			"sessionUpdate": "agentMessageChunk",
			"content": []any{
				map[string]any{"text": "A"},
				map[string]any{"parts": []any{map[string]any{"text": "B"}}},
			},
		},
	})
	if got := extractPmAgentText(raw); got != "AB" {
		t.Fatalf("got %q want AB", got)
	}
}

func TestNormalizePmKind(t *testing.T) {
	if got := normalizePmKind("agentMessageChunk"); got != "agent_message_chunk" {
		t.Fatalf("got %q", got)
	}
}

func TestSubscribeCatchupDoesNotDropBufferedEvents(t *testing.T) {
	r := NewPmTurnRunner(nil, nil)
	turn := &pmActiveTurn{
		threadID: "thr-1",
		subs:     make(map[*turnSub]struct{}),
	}
	for i := 0; i < 100; i++ {
		turn.events = append(turn.events, PmTurnEvent{Seq: i, Type: "acp"})
		turn.nextSeq = i + 1
	}
	r.turns["thr-1"] = turn

	ch, unsub, ok := r.Subscribe("thr-1", -1)
	if !ok {
		t.Fatal("expected subscribe ok")
	}
	defer unsub()

	got := 0
	for ev := range ch {
		if ev.Seq != got {
			t.Fatalf("seq=%d want %d", ev.Seq, got)
		}
		got++
		if got == 100 {
			break
		}
	}
	if got != 100 {
		t.Fatalf("got %d events, want 100 (silent drop regress)", got)
	}
}

func TestPmTurnRunnerActiveIgnoresDoneTurn(t *testing.T) {
	r := NewPmTurnRunner(nil, nil)
	if r.Active("thr-1") {
		t.Fatal("nil turn must not be Active")
	}
	r.turns["thr-1"] = &pmActiveTurn{threadID: "thr-1"}
	if !r.Active("thr-1") {
		t.Fatal("running turn must be Active")
	}
	r.turns["thr-1"].done = true
	if r.Active("thr-1") {
		t.Fatal("done-but-not-GC turn must not be Active (avoids false live resume)")
	}
}

func TestSubscribeAfterSeqSkipsAlreadySeen(t *testing.T) {
	r := NewPmTurnRunner(nil, nil)
	turn := &pmActiveTurn{
		threadID: "thr-1",
		subs:     make(map[*turnSub]struct{}),
		done:     true,
	}
	for i := 0; i < 5; i++ {
		turn.events = append(turn.events, PmTurnEvent{Seq: i, Type: "acp"})
	}
	turn.nextSeq = 5
	r.turns["thr-1"] = turn

	ch, _, ok := r.Subscribe("thr-1", 2)
	if !ok {
		t.Fatal("expected subscribe ok")
	}
	var seqs []int
	for ev := range ch {
		seqs = append(seqs, ev.Seq)
	}
	if len(seqs) != 2 || seqs[0] != 3 || seqs[1] != 4 {
		t.Fatalf("got %v want [3 4]", seqs)
	}
}
