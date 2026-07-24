package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// RunEvents streams a run's live updates over WebSocket. On connect it sends a
// "snapshot" frame, then relays broker messages: "trace" (state-trace entries),
// "status" (run status changes), "react" (react-dialogue turns), and "acp"
// (a running node's in-progress agent events, pushed via publishAcp).
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

	// Reader pump to detect client disconnects.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				_ = conn.Close()
				return
			}
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
