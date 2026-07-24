package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"backend/internal/service"

	"github.com/gorilla/websocket"
)

const (
	apiBase       = "https://api.sgroup.qq.com"
	tokenEndpoint = "https://bots.qq.com/app/getAppAccessToken"
	gatewayURL    = "https://api.sgroup.qq.com/gateway"

	intentGroupAndC2C = 1 << 25
	intentInteraction = 1 << 26

	opDispatch  = 0
	opHeartbeat = 1
	opIdentify  = 2
	opResume    = 6
	opHello     = 10
	opHeartbAck = 11
	opInvalidSe = 9
)

var qqMentionRE = regexp.MustCompile(`<@!?\d+>`)

type Service struct {
	bridge *service.Bridge
	store  *Store

	httpClient *http.Client

	mu      sync.Mutex
	cfg     Config
	pending map[string]*turn
	current *turn

	tokenMu sync.Mutex
	token   string
	tokenAt time.Time

	// WebSocket 状态
	wsMu      sync.Mutex
	wsConn    *websocket.Conn
	wsCancel  context.CancelFunc
	wsRunning bool
	wsState   string // "disconnected", "connecting", "connected"
	sessionID string
	lastSeq   int64
}

type turn struct {
	opID       string
	target     replyTarget
	buf        strings.Builder
	createdAt  time.Time
	finishedAt time.Time
}

type replyTarget struct {
	Kind        string
	OpenID      string
	GroupOpenID string
	MsgID       string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresInRaw any    `json:"expires_in"`
}

type wsPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  int64           `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

type gatewayResponse struct {
	URL string `json:"url"`
}

type helloData struct {
	HeartbeatInterval int64 `json:"heartbeat_interval"`
}

type readyData struct {
	SessionID string `json:"session_id"`
}

func New(bridge *service.Bridge, store *Store) (*Service, error) {
	if bridge == nil {
		return nil, errors.New("qqbot: nil bridge")
	}
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	s := &Service{
		bridge: bridge,
		store:  store,
		cfg:    cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		pending: make(map[string]*turn),
	}
	bridge.SubscribeEvents(s.handleBridgeEvent)
	return s, nil
}

func (s *Service) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) PublicConfigWithStatus() PublicConfig {
	cfg := s.Config()
	pub := cfg.Public()
	switch {
	case !cfg.Enabled:
		pub.Status = "未启用"
	case cfg.AppID == "" || cfg.AppSecret == "":
		pub.Status = "配置不完整"
	default:
		s.wsMu.Lock()
		state := s.wsState
		s.wsMu.Unlock()
		switch state {
		case "connected":
			pub.Connected = true
			pub.Status = "已连接"
		case "connecting":
			pub.Status = "连接中…"
		default:
			pub.Status = "未连接"
		}
	}
	return pub
}

func (s *Service) UpdateConfig(next Config) error {
	current := s.Config()
	if strings.TrimSpace(next.AppSecret) == "" {
		next.AppSecret = current.AppSecret
	}
	next.normalize()
	if err := s.store.Save(next); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = next
	s.mu.Unlock()
	s.invalidateToken()
	// 配置变更后重新连接 WebSocket
	s.Reconnect()
	return nil
}

func (s *Service) Enabled() bool {
	cfg := s.Config()
	return cfg.Enabled && cfg.AppID != "" && cfg.AppSecret != ""
}

// Start 启动 WebSocket 网关连接（非阻塞）
func (s *Service) Start() {
	if !s.Enabled() {
		log.Printf("qqbot: 未启用或配置不完整，跳过 WebSocket 连接")
		return
	}
	go s.runGateway()
}

// Stop 断开 WebSocket 连接
func (s *Service) Stop() {
	s.wsMu.Lock()
	if s.wsCancel != nil {
		s.wsCancel()
	}
	s.wsMu.Unlock()
}

// Reconnect 断开并重新连接（非阻塞）
func (s *Service) Reconnect() {
	s.Stop()
	if s.Enabled() {
		s.wsMu.Lock()
		s.wsState = "connecting"
		s.wsMu.Unlock()
		go func() {
			// 等旧连接退出
			time.Sleep(500 * time.Millisecond)
			s.runGateway()
		}()
	} else {
		s.wsMu.Lock()
		s.wsState = "disconnected"
		s.wsMu.Unlock()
	}
}
