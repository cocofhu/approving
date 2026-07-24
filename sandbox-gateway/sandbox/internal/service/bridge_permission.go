package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"backend/internal/acp"
)

func (b *Bridge) permissionChooser(ctx context.Context, rpcID json.RawMessage, rawParams json.RawMessage) (string, error) {
	b.mu.Lock()
	auto := b.autoPermission
	b.mu.Unlock()
	if auto {
		return acp.DefaultPermissionForParams(rawParams)
	}
	key := acp.JSONRPCIDKey(rpcID)
	ch := make(chan string, 1)
	b.mu.Lock()
	b.permWait[key] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.permWait, key)
		b.mu.Unlock()
	}()
	b.Broadcast(map[string]any{
		"op":     "permission_request",
		"rpcId":  key,
		"params": rawParams,
	})
	log.Printf("perm %s: 等待用户选择 rpcIdKey=%q", b.AgentLogPrefix(), key)
	select {
	case <-ctx.Done():
		log.Printf("perm %s: 超时/取消 rpcIdKey=%q: %v", b.AgentLogPrefix(), key, ctx.Err())
		return "", ctx.Err()
	case opt := <-ch:
		log.Printf("perm %s: 已选择 rpcIdKey=%q optionId=%q", b.AgentLogPrefix(), key, opt)
		return opt, nil
	}
}

func (b *Bridge) ResolvePermission(rpcID, optionID string) error {
	b.mu.Lock()
	ch, ok := b.permWait[rpcID]
	b.mu.Unlock()
	if !ok {
		log.Printf("perm %s: ResolvePermission 无等待中的请求 rpcId=%q", b.AgentLogPrefix(), rpcID)
		return fmt.Errorf("unknown permission request %q", rpcID)
	}
	select {
	case ch <- optionID:
		log.Printf("perm %s: ResolvePermission 已投递 rpcId=%q optionId=%q", b.AgentLogPrefix(), rpcID, optionID)
		return nil
	default:
		log.Printf("perm %s: ResolvePermission 通道已满（重复提交?） rpcId=%q", b.AgentLogPrefix(), rpcID)
		return fmt.Errorf("permission channel full for %q", rpcID)
	}
}
