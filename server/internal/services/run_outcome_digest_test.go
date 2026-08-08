package services

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func digestDB(t *testing.T) *ArtifactService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Artifact{}); err != nil {
		t.Fatal(err)
	}
	return NewArtifactService(db)
}

func TestDigestedRunOutcomePrefersSummaryArtifact(t *testing.T) {
	arts := digestDB(t)
	if _, err := arts.Save("run-1", "n1", "noise.txt", "text", "ignore me"); err != nil {
		t.Fatal(err)
	}
	if _, err := arts.Save("run-1", "n1", "summary.md", "markdown",
		"# 检查结果\n\n主干错误处理覆盖了 Live fallthrough，但超时仍默认 8s。\n"); err != nil {
		t.Fatal(err)
	}
	got := arts.DigestedRunOutcome("run-1", 200)
	if !strings.Contains(got, "错误处理") || !strings.Contains(got, "8s") {
		t.Fatalf("digest = %q", got)
	}
	if strings.Contains(got, "ignore me") {
		t.Fatalf("preferred summary lost to noise: %q", got)
	}
}

func TestDigestedRunOutcomeFallsBackToTextArtifact(t *testing.T) {
	arts := digestDB(t)
	if _, err := arts.Save("run-2", "n1", "plan.json", "json", `{"items":[]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := arts.Save("run-2", "n1", "notes.md", "markdown", "Release 链路缺 ack。"); err != nil {
		t.Fatal(err)
	}
	got := arts.DigestedRunOutcome("run-2", 100)
	if !strings.Contains(got, "Release") {
		t.Fatalf("digest = %q", got)
	}
}

func TestDigestedRunOutcomeEmptyWithoutReadableArtifacts(t *testing.T) {
	arts := digestDB(t)
	if _, err := arts.Save("run-3", "n1", "plan.json", "json", `{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if got := arts.DigestedRunOutcome("run-3", 100); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestAppendRunDeliveryURL(t *testing.T) {
	const url = "https://github.com/org/repo/pull/42"
	if got := AppendRunDeliveryURL("主干超时已对齐。", url); !strings.Contains(got, url) ||
		!strings.Contains(got, "主干超时") {
		t.Fatalf("append = %q", got)
	}
	if got := AppendRunDeliveryURL("", url); got != "PR/MR："+url {
		t.Fatalf("empty digest = %q", got)
	}
	already := "见 " + url
	if got := AppendRunDeliveryURL(already, url); got != already {
		t.Fatalf("duplicate append = %q", got)
	}
	if got := AppendRunDeliveryURL("x", "  "); got != "x" {
		t.Fatalf("blank url = %q", got)
	}
}
