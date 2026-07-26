package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// RunEvents streams a run's live updates over WebSocket. On connect it sends a
// "snapshot" frame, then relays broker messages: "trace" (state-trace entries),
// "status" (run status changes), "react" (react-dialogue turns), "acp"
// (a running node's in-progress agent events, pushed via publishAcp), and
// "review" (queue_state / turn_begin / turn_done / error for the platform
// review session controller).
//
// Client → server control frames (SandboxChat-aligned):
//
//	{"type":"review_chat","nodeId":"<producer>","content":"…","images":[],"annotations":[],"gateNodeId":""}
//	{"type":"review_cancel","nodeId":"<producer>"}
//
// review_chat with gateNodeId set enqueues via GateReactRevise semantics;
// otherwise via node-inline EnqueueReviewTurn.
func (h *Handlers) RunEvents(c *gin.Context) {
	if h.Auth != nil {
		if _, ok := h.Auth.RequireSession(c); !ok {
			return
		}
	}
	runID := c.Param("id")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade writes its own HTTP error response; log at debug since a
		// failed handshake is usually a client/proxy issue, not a server fault.
		log.Debug().Str("run_id", runID).Err(err).Msg("run events websocket upgrade failed")
		return
	}
	defer conn.Close()

	ch, unsub := h.Eng.Broker().Subscribe(runID)
	defer unsub()

	// Send a snapshot of the current run on connect.
	if run, ok := h.Runs.Get(runID); ok {
		_ = conn.WriteJSON(gin.H{"type": "snapshot", "run": h.runDetailDTO(run)})
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	// Reader pump: detect disconnect + handle review control frames.
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				_ = conn.Close()
				return
			}
			h.handleRunWSControl(runID, data)
		}
	}()

	for {
		select {
		case msg, open := <-ch:
			if !open {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func (h *Handlers) handleRunWSControl(runID string, data []byte) {
	var m struct {
		Type        string                   `json:"type"`
		NodeID      string                   `json:"nodeId"`
		Content     string                   `json:"content"`
		Images      []models.PromptImage     `json:"images"`
		Annotations []models.ReactAnnotation `json:"annotations"`
		GateNodeID  string                   `json:"gateNodeId"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	switch m.Type {
	case "review_cancel":
		nodeID := strings.TrimSpace(m.NodeID)
		if nodeID == "" {
			return
		}
		_ = h.Eng.CancelReviewSession(runID, nodeID)
	case "review_chat", "chat":
		nodeID := strings.TrimSpace(m.NodeID)
		if nodeID == "" {
			return
		}
		gateID := strings.TrimSpace(m.GateNodeID)
		if gateID != "" {
			_ = h.Eng.GateReactRevise(runID, gateID, m.Content, m.Images, m.Annotations)
			return
		}
		_, _ = h.Eng.EnqueueReviewTurn(runID, nodeID, m.Content, m.Images, m.Annotations, "node", "")
	}
}
