package qq

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// dispatchHandler receives decoded dispatch events (event type + raw d payload).
type dispatchHandler func(evtType string, data []byte)

// gateway maintains the QQ WebSocket connection with heartbeat, identify, and
// resume/reconnect. All writes go through a mutex; token refresh is handled by
// the client under its own lock.
type gateway struct {
	client  *client
	intents int
	onEvent dispatchHandler

	writeMu   sync.Mutex
	conn      *websocket.Conn
	sessionID string
	lastSeq   atomic.Int64 // written by read loop, read by heartbeat goroutine
}

func newGateway(c *client, intents int, onEvent dispatchHandler) *gateway {
	if intents == 0 {
		intents = defaultIntents
	}
	return &gateway{client: c, intents: intents, onEvent: onEvent}
}

// run connects and maintains the session until ctx is cancelled, reconnecting
// with backoff and resuming when a session id is known.
func (g *gateway) run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		resume := g.sessionID != "" && g.lastSeq.Load() > 0
		if err := g.connectOnce(ctx, resume); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn().Err(err).Dur("backoff", backoff).Msg("qq gateway: connection ended; retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			continue
		}
		backoff = time.Second
	}
}

func (g *gateway) connectOnce(ctx context.Context, resume bool) error {
	wsURL, err := g.client.gatewayURL(ctx)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	g.writeMu.Lock()
	g.conn = conn
	g.writeMu.Unlock()
	defer func() {
		_ = conn.Close()
		g.writeMu.Lock()
		g.conn = nil
		g.writeMu.Unlock()
	}()

	// Expect Hello first.
	var hello gatewayFrame
	if err := conn.ReadJSON(&hello); err != nil {
		return err
	}
	interval := 30000
	if hello.Op == opHello && len(hello.D) > 0 {
		var hd helloData
		if json.Unmarshal(hello.D, &hd) == nil && hd.HeartbeatInterval > 0 {
			interval = hd.HeartbeatInterval
		}
	}

	tok, err := g.client.authHeader(ctx)
	if err != nil {
		return err
	}
	if resume {
		if err := g.writeJSON(gatewayFrame{Op: opResume, D: mustJSON(resumeData{
			Token: tok, SessionID: g.sessionID, Seq: g.lastSeq.Load(),
		})}); err != nil {
			return err
		}
	} else {
		if err := g.writeJSON(gatewayFrame{Op: opIdentify, D: mustJSON(identifyData{
			Token: tok, Intents: g.intents, Shard: []int{0, 1},
			Properties: map[string]string{},
		})}); err != nil {
			return err
		}
	}

	// Heartbeat loop bound to this connection.
	hbCtx, stopHB := context.WithCancel(ctx)
	defer stopHB()
	go g.heartbeatLoop(hbCtx, time.Duration(interval)*time.Millisecond)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var frame gatewayFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return err
		}
		switch frame.Op {
		case opDispatch:
			if frame.S > 0 {
				g.lastSeq.Store(frame.S)
			}
			g.handleDispatch(frame)
		case opHeartbeatACK:
			// ok
		case opReconnect:
			return nil // reconnect (resume)
		case opInvalidSess:
			g.sessionID = ""
			g.lastSeq.Store(0)
			return nil // full re-identify
		}
	}
}

func (g *gateway) handleDispatch(frame gatewayFrame) {
	switch frame.T {
	case evtReady:
		var rd readyData
		if json.Unmarshal(frame.D, &rd) == nil {
			g.sessionID = rd.SessionID
			log.Info().Str("session", rd.SessionID).Str("bot", rd.User.Username).Msg("qq gateway ready")
		}
	case evtResumed:
		log.Info().Msg("qq gateway resumed")
	default:
		if g.onEvent != nil {
			g.onEvent(frame.T, frame.D)
		}
	}
}

func (g *gateway) heartbeatLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var d []byte
			if seq := g.lastSeq.Load(); seq > 0 {
				d = mustJSON(seq)
			} else {
				d = []byte("null")
			}
			if err := g.writeJSON(gatewayFrame{Op: opHeartbeat, D: d}); err != nil {
				return
			}
		}
	}
}

func (g *gateway) writeJSON(v gatewayFrame) error {
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	if g.conn == nil {
		return websocket.ErrCloseSent
	}
	return g.conn.WriteJSON(v)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
