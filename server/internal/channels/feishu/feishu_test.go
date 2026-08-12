package feishu

import (
	"strings"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

func TestRegionEndpoints(t *testing.T) {
	if OpenBaseURL(nil) != lark.FeishuBaseUrl {
		t.Fatalf("default region should be domestic")
	}
	if OpenBaseURL(map[string]any{"region": "lark"}) != lark.LarkBaseUrl {
		t.Fatalf("lark region should use international base")
	}
	if regionOf(map[string]any{"region": "international"}) != RegionLark {
		t.Fatal("international alias")
	}
}

func TestShouldAcceptGroupMention(t *testing.T) {
	bot := "ou_bot"
	p2p := inboundEvent{ChatID: "oc_1", ChatType: "p2p"}
	if !shouldAccept(p2p, bot) {
		t.Fatal("p2p must accept")
	}
	groupNoAt := inboundEvent{ChatID: "oc_g", ChatType: "group"}
	if shouldAccept(groupNoAt, bot) {
		t.Fatal("group without @ must drop")
	}
	groupAll := inboundEvent{
		ChatID: "oc_g", ChatType: "group",
		Mentions: []inboundMention{{Key: "@_all", Name: "所有人"}},
	}
	if shouldAccept(groupAll, bot) {
		t.Fatal("only @all must drop")
	}
	groupBot := inboundEvent{
		ChatID: "oc_g", ChatType: "topic_group",
		Mentions: []inboundMention{{Key: "@_user_1", OpenID: bot, Name: "助手"}},
	}
	if !shouldAccept(groupBot, bot) {
		t.Fatal("topic group @bot must accept as ordinary group")
	}
}

func TestExtractTextAndUnsupported(t *testing.T) {
	if got := extractText("text", `{"text":"hello @_user_1 world"}`); got != "hello world" {
		t.Fatalf("text=%q", got)
	}
	if !isUnsupportedMedia("file") || !isUnsupportedMedia("audio") || !isUnsupportedMedia("media") {
		t.Fatal("file/audio/media should be unsupported")
	}
	if keys := imageKeys("image", `{"image_key":"img_1"}`); len(keys) != 1 || keys[0] != "img_1" {
		t.Fatalf("image keys=%v", keys)
	}
}

func TestBuildPostHasMdAndCode(t *testing.T) {
	raw := buildPostContent("# 评审结论\n- 范围对齐\n```go\nfmt.Println(1)\n```")
	if !strings.Contains(raw, `"title":"评审结论"`) {
		t.Fatalf("missing title: %s", raw)
	}
	if !strings.Contains(raw, `"tag":"md"`) || !strings.Contains(raw, `"tag":"code_block"`) {
		t.Fatalf("expected md + code_block: %s", raw)
	}
	if strings.Contains(raw, "interactive") {
		t.Fatal("must not use interactive cards")
	}
}
