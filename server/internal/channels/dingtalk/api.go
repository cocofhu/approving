package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cocofhu/approving/internal/channels"
)

const (
	openAPIHost = "https://api.dingtalk.com"
	oapiHost    = "https://oapi.dingtalk.com"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

type tokenCache struct {
	mu      sync.Mutex
	token   string
	expires time.Time
}

func (t *tokenCache) get(ctx context.Context, appKey, appSecret string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Now().Before(t.expires) {
		return t.token, nil
	}
	body, _ := json.Marshal(map[string]string{
		"appKey":    appKey,
		"appSecret": appSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openAPIHost+"/v1.0/oauth2/accessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%w: oauth status=%d body=%s", channels.ErrAdapterAuth, resp.StatusCode, truncateErr(raw))
	}
	var out struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
		Code        string `json:"code"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("%w: 解析凭证响应失败", channels.ErrAdapterAuth)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		msg := strings.TrimSpace(out.Message)
		if msg == "" {
			msg = strings.TrimSpace(out.Code)
		}
		if msg == "" {
			msg = truncateErr(raw)
		}
		return "", fmt.Errorf("%w: %s", channels.ErrAdapterAuth, msg)
	}
	exp := out.ExpireIn
	if exp <= 0 {
		exp = 7200
	}
	if exp > 300 {
		exp -= 300
	}
	t.token = out.AccessToken
	t.expires = time.Now().Add(time.Duration(exp) * time.Second)
	return t.token, nil
}

func replySessionWebhook(ctx context.Context, webhook, msgType, title, text string) error {
	webhook = strings.TrimSpace(webhook)
	if webhook == "" {
		return fmt.Errorf("dingtalk: empty sessionWebhook")
	}
	var body map[string]any
	switch msgType {
	case "markdown":
		body = map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": firstNonEmpty(title, "Approving"),
				"text":  text,
			},
		}
	default:
		body = map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": text},
		}
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk webhook %s: status=%d body=%s", msgType, resp.StatusCode, truncateErr(respBody))
	}
	return nil
}

func sendOpenAPI(ctx context.Context, token, robotCode string, scene channels.Scene, conversationID, staffID, msgKey, msgParam string) error {
	robotCode = strings.TrimSpace(robotCode)
	if robotCode == "" {
		return fmt.Errorf("dingtalk: empty robotCode")
	}
	var (
		url  string
		body map[string]any
	)
	switch scene {
	case channels.SceneGroup:
		url = openAPIHost + "/v1.0/robot/groupMessages/send"
		body = map[string]any{
			"robotCode":          robotCode,
			"openConversationId": conversationID,
			"msgKey":             msgKey,
			"msgParam":           msgParam,
		}
	case channels.SceneC2C:
		uid := strings.TrimSpace(staffID)
		if uid == "" {
			uid = strings.TrimSpace(conversationID)
		}
		if uid == "" {
			return fmt.Errorf("dingtalk: missing staffId for c2c OpenAPI")
		}
		url = openAPIHost + "/v1.0/robot/oToMessages/batchSend"
		body = map[string]any{
			"robotCode": robotCode,
			"userIds":   []string{uid},
			"msgKey":    msgKey,
			"msgParam":  msgParam,
		}
	default:
		return fmt.Errorf("dingtalk: unsupported scene %q for OpenAPI", scene)
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk openapi %s: status=%d body=%s", msgKey, resp.StatusCode, truncateErr(respBody))
	}
	var envelope struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestid"`
	}
	_ = json.Unmarshal(respBody, &envelope)
	if envelope.Code != "" && !strings.EqualFold(envelope.Code, "OK") && envelope.Code != "0" {
		return fmt.Errorf("dingtalk openapi %s: %s %s", msgKey, envelope.Code, envelope.Message)
	}
	return nil
}

func downloadByCode(ctx context.Context, token, robotCode, downloadCode string) ([]byte, string, error) {
	body, _ := json.Marshal(map[string]string{
		"downloadCode": downloadCode,
		"robotCode":    robotCode,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		openAPIHost+"/v1.0/robot/messageFiles/download", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("dingtalk download url: status=%d body=%s", resp.StatusCode, truncateErr(raw))
	}
	var out struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || strings.TrimSpace(out.DownloadURL) == "" {
		return nil, "", fmt.Errorf("dingtalk download url empty")
	}
	return downloadPublic(ctx, out.DownloadURL)
}

func downloadPublic(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("download image %s: %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxInboundImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxInboundImageBytes {
		return nil, "", errTooLarge
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	return data, mime, nil
}

func uploadMedia(ctx context.Context, token string, data []byte, filename string) (string, error) {
	if filename == "" {
		filename = "image.png"
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("media", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	_ = w.Close()
	url := fmt.Sprintf("%s/media/upload?access_token=%s&type=image", oapiHost, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 || strings.TrimSpace(out.MediaID) == "" {
		return "", fmt.Errorf("dingtalk upload media: %d %s", out.ErrCode, out.ErrMsg)
	}
	return out.MediaID, nil
}

func markdownMsgParam(title, text string) string {
	b, _ := json.Marshal(map[string]string{
		"title": firstNonEmpty(title, "Approving"),
		"text":  text,
	})
	return string(b)
}

func textMsgParam(text string) string {
	b, _ := json.Marshal(map[string]string{"content": text})
	return string(b)
}

func imageMsgParam(mediaID string) string {
	b, _ := json.Marshal(map[string]string{"photoURL": mediaID})
	return string(b)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truncateErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

var errTooLarge = fmt.Errorf("image exceeds 10 MiB")
