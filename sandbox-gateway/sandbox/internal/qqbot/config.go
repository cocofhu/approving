package qqbot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultMaxImageBytes = 8 << 20

// Config 是 QQ 官方机器人的本地配置（WebSocket 网关模式）。
type Config struct {
	Enabled           bool     `json:"enabled"`
	AppID             string   `json:"appId"`
	AppSecret         string   `json:"appSecret"`
	AllowOpenIDs      []string `json:"allowOpenIds,omitempty"`
	AllowGroupOpenIDs []string `json:"allowGroupOpenIds,omitempty"`
	MaxImageBytes     int64    `json:"maxImageBytes,omitempty"`
}

// PublicConfig 返回给前端使用，避免泄露完整 AppSecret。
type PublicConfig struct {
	Enabled           bool     `json:"enabled"`
	AppID             string   `json:"appId"`
	AppSecretSet      bool     `json:"appSecretSet"`
	Connected         bool     `json:"connected"`
	Status            string   `json:"status"`
	AllowOpenIDs      []string `json:"allowOpenIds,omitempty"`
	AllowGroupOpenIDs []string `json:"allowGroupOpenIds,omitempty"`
	MaxImageBytes     int64    `json:"maxImageBytes"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func DefaultConfig() Config {
	return Config{
		MaxImageBytes: defaultMaxImageBytes,
	}
}

func (s *Store) Load() (Config, error) {
	cfg := DefaultConfig()
	if s == nil || strings.TrimSpace(s.path) == "" {
		return cfg, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	cfg.normalize()
	return cfg, nil
}

func (s *Store) Save(cfg Config) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return errors.New("qq config path is empty")
	}
	cfg.normalize()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, append(b, '\n'), 0o600)
}

func (c *Config) normalize() {
	c.AppID = strings.TrimSpace(c.AppID)
	c.AppSecret = strings.TrimSpace(c.AppSecret)
	if c.MaxImageBytes <= 0 {
		c.MaxImageBytes = defaultMaxImageBytes
	}
	c.AllowOpenIDs = normalizeStringList(c.AllowOpenIDs)
	c.AllowGroupOpenIDs = normalizeStringList(c.AllowGroupOpenIDs)
}

func normalizeStringList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (c Config) Public() PublicConfig {
	c.normalize()
	return PublicConfig{
		Enabled:           c.Enabled,
		AppID:             c.AppID,
		AppSecretSet:      c.AppSecret != "",
		AllowOpenIDs:      append([]string(nil), c.AllowOpenIDs...),
		AllowGroupOpenIDs: append([]string(nil), c.AllowGroupOpenIDs...),
		MaxImageBytes:     c.MaxImageBytes,
	}
}
