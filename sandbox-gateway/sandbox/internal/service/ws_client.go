package service

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsClient 包装 gorilla WebSocket：每连接独立写锁，禁止并发 Write。
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsClient) writeMessage(messageType int, data []byte, deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.SetWriteDeadline(deadline); err != nil {
		log.Printf("ws: SetWriteDeadline 失败: %v", err)
		return err
	}
	return w.conn.WriteMessage(messageType, data)
}

func (w *wsClient) writeJSON(v any, deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.SetWriteDeadline(deadline); err != nil {
		log.Printf("ws: SetWriteDeadline 失败: %v", err)
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, b)
}
