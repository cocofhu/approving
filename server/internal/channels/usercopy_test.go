package channels

import (
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/cocofhu/approving/internal/models"
)

// hasHan reports whether s contains Chinese characters. Used to tell the two
// language branches apart without pinning exact wording, which would make every
// copy edit a test edit.
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// TestPlatformCopyAnswersInTheAskedLanguage is the point of collecting these in
// one file. Half of them used to be Chinese-only, so a user who had been
// writing English all conversation got a Chinese apology the moment the model
// was unavailable — and because the lines were spread across eight files,
// nobody could see which half.
func TestPlatformCopyAnswersInTheAskedLanguage(t *testing.T) {
	identity := &models.TaskIdentity{ShortTitle: "登录页性能"}
	cases := map[string]func(language string) string{
		"busyHint":       busyHintText,
		"turnFailure":    func(l string) string { return turnFailureText(errors.New("sandbox boom"), l) },
		"turnFailureNil": func(l string) string { return turnFailureText(nil, l) },
		"runAcceptance":  func(l string) string { return runAcceptanceText("登录页性能", l) },
		"pauseFallback": func(l string) string {
			return pauseFallbackText(identity, TaskPause{Ask: "要不要跳过这一步"}, l)
		},
		"outcomeFailed": func(l string) string {
			return outcomeFallbackText(identity, TaskOutcome{Status: "failed", FailureReason: "timeout"}, l)
		},
		"outcomeCancelled": func(l string) string {
			return outcomeFallbackText(identity, TaskOutcome{Status: "cancelled"}, l)
		},
		"failureReason": func(l string) string { return humanizeFailureReason("permission denied", l) },
		"cronUnchanged": func(l string) string { return formatCronPush("PR", CronResultUnchanged, "", l) },
		"cronFailed":    func(l string) string { return formatCronPush("PR", CronResultFailed, "", l) },
		"progressBlock": func(l string) string {
			return formatProgressText(ProgressEvent{Kind: ProgressBlocker, Summary: "x"}, l)
		},
		"progressConfirm": func(l string) string {
			return formatProgressText(ProgressEvent{Kind: ProgressConfirm, Summary: "x"}, l)
		},
	}
	for name, copy := range cases {
		zh, en := copy("zh-CN"), copy("en")
		if strings.TrimSpace(zh) == "" || strings.TrimSpace(en) == "" {
			t.Fatalf("%s: empty copy zh=%q en=%q", name, zh, en)
		}
		if zh == en {
			t.Fatalf("%s: one language for both — %q", name, zh)
		}
		// The identity and task titles are Chinese in these fixtures, so the
		// English branch may still carry Han; what it must not do is stay
		// entirely Chinese.
		if !hasHan(zh) {
			t.Fatalf("%s: zh-CN branch reads as English: %q", name, zh)
		}
	}
}

// An unrecognised language is not an error the user should see. It falls back
// to Chinese, the deployment default, rather than to an empty string.
func TestUnknownLanguageStillProducesCopy(t *testing.T) {
	for _, language := range []string{"", "  ", "fr", "klingon"} {
		if got := busyHintText(language); !hasHan(got) {
			t.Fatalf("busyHintText(%q) = %q, want the default language", language, got)
		}
	}
}

// A scheduled push has no conversation behind it, so the job's own words decide
// the language — the category included, because an unchanged result has barely
// any body to read from.
func TestScheduledPushReadsItsLanguageOffTheJob(t *testing.T) {
	if got := FormatCronPush("每小时PR", CronResultUnchanged, "x"); got != "PR：无变化" {
		t.Fatalf("a Chinese job reported in %q", got)
	}
	if got := FormatCronPush("Hourly PR", CronResultUnchanged, ""); hasHan(got) {
		t.Fatalf("an English job reported in %q", got)
	}
}
