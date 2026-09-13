package runtime

import (
	"context"
	"testing"

	"github.com/cocofhu/grasp/internal/models"
)

// plan g2.2: empty ACP narration must not Done / finishReact / leave node_complete.
func TestReactEmptyNarrationKeepsDialogueOpen(t *testing.T) {
	p, _, _, _, req := reactSetup(t, func(int) chatFunc {
		return func(turn int) turnAction {
			if turn == 0 {
				return turnAction{
					narration: "need info",
					questions: []models.ReactQuestion{
						{ID: "q1", Prompt: "?", Options: []models.ReactOption{{ID: "a", Label: "A"}}},
					},
				}
			}
			return turnAction{narration: ""}
		}
	})
	open := p.ReactOpen(context.Background(), req)
	if open.Done {
		t.Fatal("opening question must pause")
	}
	hist := []models.ReactMessage{
		{Role: "agent", Text: "need info"},
		{Role: "human", Text: "answer"},
	}
	reply := p.ReactReply(context.Background(), req, hist, "answer", nil, false)
	if reply.Err != nil {
		t.Fatalf("empty reply err: %v", reply.Err)
	}
	if reply.Done {
		t.Fatalf("empty narration must not finish the node, got %+v", reply)
	}
	if !p.HasLiveSession(req.RunID, req.NodeID) {
		t.Fatal("session must stay open after empty-fail")
	}
}

func TestReactApproveEmptyNarrationKeepsDialogueOpen(t *testing.T) {
	p, host, _, _, req := approveSetup(t, func(int) chatFunc {
		return func(int) turnAction {
			return turnAction{narration: "", outcome: true}
		}
	})
	open := p.ReactOpen(context.Background(), req)
	if open.Done {
		t.Fatal("approve open must park")
	}
	hist := []models.ReactMessage{{Role: "human", Text: "做登录"}}
	reply := p.ReactReply(context.Background(), req, hist, "做登录", nil, false)
	if reply.Err != nil {
		t.Fatalf("approve empty reply err: %v", reply.Err)
	}
	if reply.Done {
		t.Fatalf("approve empty narration must not Done, got %+v", reply)
	}
	if host.HasOutcome(req.RunID, req.NodeID) {
		t.Fatal("empty approve turn must not leave node_complete / node failed")
	}
	if !p.HasLiveSession(req.RunID, req.NodeID) {
		t.Fatal("approve session must stay open")
	}
}
