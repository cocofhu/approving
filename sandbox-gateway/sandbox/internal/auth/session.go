package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const cookieName = "agentchat_session"

// Store 保存已登录会话（进程内；重启后需重新登录）。
type Store struct {
	mu     sync.RWMutex
	tokens map[string]time.Time
	ttl    time.Duration
}

// NewStore 创建会话存储；ttl 为单次登录 Cookie/会话有效期。
func NewStore(ttl time.Duration) *Store {
	return &Store{
		tokens: make(map[string]time.Time),
		ttl:    ttl,
	}
}

// Issue 生成新令牌并登记过期时间。
func (s *Store) Issue() (token string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token = hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = time.Now().Add(s.ttl)
	return token, nil
}

// Valid 若令牌有效则返回 true，并顺带清理已过期的条目。
func (s *Store) Valid(token string) bool {
	if token == "" {
		return false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tokens[token]
	if !ok {
		return false
	}
	if now.After(exp) {
		delete(s.tokens, token)
		return false
	}
	return true
}

// Revoke 注销令牌（登出）。
func (s *Store) Revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}
