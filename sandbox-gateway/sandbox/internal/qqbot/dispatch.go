package qqbot

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"backend/internal/correl"
	"backend/internal/service"
)

func (s *Service) handleDispatch(ctx context.Context, p wsPayload) {
	switch p.T {
	case "READY":
		var ready readyData
		if err := json.Unmarshal(p.D, &ready); err != nil {
			log.Printf("qqbot: READY 解析失败: %v", err)
		}
		s.wsMu.Lock()
		s.sessionID = ready.SessionID
		s.wsState = "connected"
		s.wsMu.Unlock()
		log.Printf("qqbot: READY session=%s", ready.SessionID)

	case "RESUMED":
		s.wsMu.Lock()
		s.wsState = "connected"
		s.wsMu.Unlock()
		log.Printf("qqbot: RESUMED 会话恢复成功")

	case "C2C_MESSAGE_CREATE":
		s.handleC2CMessage(ctx, p.D)

	case "GROUP_AT_MESSAGE_CREATE":
		s.handleGroupMessage(ctx, p.D)

	default:
		log.Printf("qqbot: 忽略事件 t=%s", p.T)
	}
}

func (s *Service) handleC2CMessage(ctx context.Context, raw json.RawMessage) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Printf("qqbot: 解析 C2C 消息失败: %v", err)
		return
	}

	author, _ := data["author"].(map[string]any)
	openID := firstString(author["user_openid"], data["user_openid"])
	msgID := firstString(data["id"])
	content := strings.TrimSpace(firstString(data["content"]))

	if openID == "" {
		log.Printf("qqbot: C2C 消息缺少 user_openid")
		return
	}

	cfg := s.Config()
	if !allowed(openID, cfg.AllowOpenIDs) {
		log.Printf("qqbot: C2C openid=%s 不在允许列表", openID)
		return
	}

	text := strings.TrimSpace(qqMentionRE.ReplaceAllString(content, ""))
	target := replyTarget{Kind: "c2c", OpenID: openID, MsgID: msgID}

	imageURLs := extractImageURLs(data)
	images := make([]service.PromptImage, 0, len(imageURLs))
	for _, u := range imageURLs {
		img, err := s.downloadImage(ctx, u, cfg.MaxImageBytes)
		if err != nil {
			log.Printf("qqbot: 下载图片失败: %v", err)
			continue
		}
		images = append(images, img)
	}

	if text == "" && len(images) == 0 {
		log.Printf("qqbot: C2C 空消息 openid=%s", openID)
		return
	}

	s.dispatchToBridge(text, target, images)
}

func (s *Service) handleGroupMessage(ctx context.Context, raw json.RawMessage) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Printf("qqbot: 解析群消息失败: %v", err)
		return
	}

	groupOpenID := firstString(data["group_openid"])
	author, _ := data["author"].(map[string]any)
	memberOpenID := firstString(author["member_openid"])
	msgID := firstString(data["id"])
	content := strings.TrimSpace(firstString(data["content"]))

	if groupOpenID == "" {
		log.Printf("qqbot: 群消息缺少 group_openid")
		return
	}

	cfg := s.Config()
	if !allowed(groupOpenID, cfg.AllowGroupOpenIDs) {
		log.Printf("qqbot: 群 %s 不在允许列表", groupOpenID)
		return
	}

	text := strings.TrimSpace(qqMentionRE.ReplaceAllString(content, ""))
	target := replyTarget{Kind: "group", GroupOpenID: groupOpenID, OpenID: memberOpenID, MsgID: msgID}

	imageURLs := extractImageURLs(data)
	images := make([]service.PromptImage, 0, len(imageURLs))
	for _, u := range imageURLs {
		img, err := s.downloadImage(ctx, u, cfg.MaxImageBytes)
		if err != nil {
			log.Printf("qqbot: 下载图片失败: %v", err)
			continue
		}
		images = append(images, img)
	}

	if text == "" && len(images) == 0 {
		log.Printf("qqbot: 群空消息 group=%s", groupOpenID)
		return
	}

	s.dispatchToBridge(text, target, images)
}

func (s *Service) dispatchToBridge(text string, target replyTarget, images []service.PromptImage) {
	opID := "qq-" + correl.ID()
	s.registerTurn(opID, target)
	if err := s.bridge.ChatWithOpID(text, opID, "chat", images); err != nil {
		s.forgetTurn(opID)
		log.Printf("qqbot: 消息入队失败: %v", err)
		return
	}
	log.Printf("qqbot: 消息已入队 kind=%s opID=%s textLen=%d images=%d", target.Kind, opID, len(text), len(images))
}

// ─── Bridge 事件处理 ───

func (s *Service) registerTurn(opID string, target replyTarget) {
	if strings.TrimSpace(opID) == "" {
		return
	}
	s.mu.Lock()
	s.pending[opID] = &turn{opID: opID, target: target, createdAt: time.Now()}
	s.mu.Unlock()
}

func (s *Service) forgetTurn(opID string) {
	s.mu.Lock()
	delete(s.pending, opID)
	if s.current != nil && s.current.opID == opID {
		s.current = nil
	}
	s.mu.Unlock()
}
