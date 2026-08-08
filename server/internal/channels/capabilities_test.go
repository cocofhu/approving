package channels

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestQQReplyCapabilityAndBilingualFormatting(t *testing.T) {
	if QQReplyCapability != "unsupported" {
		t.Fatalf("QQ reply capability=%q", QQReplyCapability)
	}
	if SupportsReplyReference(models.ChannelTypeQQ) {
		t.Fatal("QQ must not claim reply references")
	}
	// Naming the task reads as a reference in a sentence, not as a ticket header.
	if got := FormatTaskMessage("登录页", "阻塞", "需要权限", "继续", ""); got != "登录页那个：需要权限" {
		t.Fatalf("zh task message=%q", got)
	}
	if got := FormatTaskMessage("Login", "Blocked", "Need access", "continue", ""); got != "On \"Login\" — Need access" {
		t.Fatalf("en task message=%q", got)
	}
	for _, got := range []string{
		FormatTaskMessage("登录页", "阻塞", "需要权限", "继续", ""),
		FormatTaskMessage("Login", "Blocked", "Need access", "continue", ""),
	} {
		if strings.ContainsAny(got, "【｜】") {
			t.Fatalf("task message still wears a ticket header: %q", got)
		}
	}
}
