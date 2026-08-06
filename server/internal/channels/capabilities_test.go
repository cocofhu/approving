package channels

import (
	"strings"
	"testing"
)

func TestQQReplyCapabilityFallbackAndBilingualFormatting(t *testing.T) {
	if QQReplyCapability != "unsupported" {
		t.Fatalf("QQ reply capability=%q", QQReplyCapability)
	}
	zh := QQReplyFallback("请继续", "en")
	if !strings.Contains(zh, "不支持引用回复") || !strings.Contains(zh, "序号") {
		t.Fatalf("zh fallback=%q", zh)
	}
	en := QQReplyFallback("please continue", "zh-CN")
	if !strings.Contains(en, "unavailable") || !strings.Contains(en, "number") {
		t.Fatalf("en fallback=%q", en)
	}
	if got := FormatTaskMessage("登录页", "阻塞", "需要权限", "继续", ""); got != "【登录页｜阻塞】 需要权限" {
		t.Fatalf("zh task message=%q", got)
	}
	if got := FormatTaskMessage("Login", "Blocked", "Need access", "continue", ""); got != "【Login｜Blocked】 Need access" {
		t.Fatalf("en task message=%q", got)
	}
}
