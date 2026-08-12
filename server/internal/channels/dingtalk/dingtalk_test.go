package dingtalk

import (
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/channels"
)

func TestSceneOf(t *testing.T) {
	if s, ok := sceneOf("1"); !ok || s != channels.SceneC2C {
		t.Fatalf("1 → c2c, got %q ok=%v", s, ok)
	}
	if s, ok := sceneOf("2"); !ok || s != channels.SceneGroup {
		t.Fatalf("2 → group, got %q ok=%v", s, ok)
	}
	if _, ok := sceneOf("9"); ok {
		t.Fatal("unknown conversationType must fail")
	}
}

func TestShouldAccept(t *testing.T) {
	c2c := inboundEvent{ConversationID: "cid", ConversationType: "1"}
	if !shouldAccept(c2c) {
		t.Fatal("c2c must accept")
	}
	groupNoAt := inboundEvent{ConversationID: "gid", ConversationType: "2"}
	if shouldAccept(groupNoAt) {
		t.Fatal("group without @ must drop")
	}
	groupAt := inboundEvent{ConversationID: "gid", ConversationType: "2", IsInAtList: true}
	if !shouldAccept(groupAt) {
		t.Fatal("group @ must accept")
	}
	groupAtUser := inboundEvent{
		ConversationID: "gid", ConversationType: "2", ChatbotUserID: "bot1",
		AtUsers: []atUser{{DingtalkID: "bot1"}},
	}
	if !shouldAccept(groupAtUser) {
		t.Fatal("group AtUsers bot must accept")
	}
}

func TestExtractTextAndImages(t *testing.T) {
	ev := inboundEvent{MsgType: "text", Text: "  @助手 你好世界  "}
	if got := extractText(ev); got != "你好世界" {
		t.Fatalf("text=%q", got)
	}
	rich := inboundEvent{
		MsgType: "richText",
		Content: map[string]any{
			"richText": []any{
				map[string]any{"text": "@bot 看看图"},
				map[string]any{"type": "picture", "downloadCode": "dl1", "pictureDownloadCode": "pic1"},
			},
		},
	}
	if got := extractText(rich); !strings.Contains(got, "看看图") {
		t.Fatalf("rich text=%q", got)
	}
	codes := imageDownloadCodes(rich)
	if len(codes) < 1 {
		t.Fatalf("expected download codes, got %v", codes)
	}
	pic := inboundEvent{MsgType: "picture", Content: map[string]any{"downloadCode": "imgA"}}
	if codes := imageDownloadCodes(pic); len(codes) != 1 || codes[0] != "imgA" {
		t.Fatalf("picture codes=%v", codes)
	}
	if !isUnsupportedMedia("audio") || !isUnsupportedMedia("file") || !isUnsupportedMedia("video") {
		t.Fatal("unsupported media types")
	}
}

func TestWebhookCacheTTL(t *testing.T) {
	c := newWebhookCache()
	c.put(channels.SceneGroup, "g1", "https://hook.example/x", time.Now().Add(2*time.Hour).UnixMilli(), "staff1")
	entry, ok := c.get(channels.SceneGroup, "g1")
	if !ok || entry.URL == "" || entry.StaffID != "staff1" {
		t.Fatalf("expected live webhook, got %+v ok=%v", entry, ok)
	}
	c.put(channels.SceneC2C, "c1", "https://hook.example/y", time.Now().Add(-time.Minute).UnixMilli(), "staff2")
	if _, ok := c.get(channels.SceneC2C, "c1"); ok {
		t.Fatal("expired webhook must not be returned")
	}
	// Staff id retained for OpenAPI after expiry sweep via rememberStaff path.
	c.rememberStaff(channels.SceneC2C, "c1", "staff2")
	if got := c.staffID(channels.SceneC2C, "c1"); got != "staff2" {
		t.Fatalf("staff=%q", got)
	}
}

func TestChooseOutbound(t *testing.T) {
	a := &Adapter{}
	mt, _, body := a.chooseOutbound("已收到，正在处理：hello")
	if mt != "text" || !strings.HasPrefix(body, "已收到") {
		t.Fatalf("ack should be text: %s %s", mt, body)
	}
	mt, title, body := a.chooseOutbound("# 评审结论\n- ok")
	if mt != "markdown" || title == "" || !strings.Contains(body, "评审结论") {
		t.Fatalf("markdown outbound: %s %s %s", mt, title, body)
	}
}

func TestSenderUserID(t *testing.T) {
	if got := senderUserID(inboundEvent{SenderStaffID: "s1", SenderID: "id"}); got != "s1" {
		t.Fatalf("prefer staffId, got %q", got)
	}
	if got := senderUserID(inboundEvent{SenderID: "id"}); got != "id" {
		t.Fatalf("fallback senderId, got %q", got)
	}
}
