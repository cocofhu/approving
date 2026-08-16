package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func reviewEvent(node string, iter, round int, text, artifact string) models.FeedbackEvent {
	return models.FeedbackEvent{
		RunID: "r", Seq: round, Round: round, Kind: models.FeedbackKindReview,
		NodeID: node, Iteration: iter, Text: text, ArtifactName: artifact,
	}
}

// Every audit round stays visible while ReAct rounds cite one shared conclusion.
func TestRunHistoryListsEveryFeedbackRoundWithSharedArtifactCitation(t *testing.T) {
	fh := twoGateRun()
	fh.feedback = []models.FeedbackEvent{
		reviewEvent("design", 2, 1, "证据不足", "feedback.review.design.i2.json"),
		reviewEvent("design", 2, 2, "图表改柱状图", "feedback.review.design.i2.json"),
		reviewEvent("design", 2, 3, "补上原始链接", "feedback.review.design.i2.json"),
	}
	h := NewHost(&memStore{})
	h.SetHistoryProvider(fh)

	out, err := h.RunHistory("r", "design", false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"证据不足", "图表改柱状图", "补上原始链接"} {
		if !strings.Contains(out, want) {
			t.Fatalf("round %q missing from overview:\n%s", want, out)
		}
	}
	cite := "read_artifact feedback.review.design.i2.json"
	if !strings.Contains(out, cite) {
		t.Fatalf("missing shared conclusion citation %q:\n%s", cite, out)
	}
	if strings.Contains(out, "i2r") {
		t.Fatalf("ReAct history must not require per-round artifact names:\n%s", out)
	}
	if !strings.Contains(out, "#2.3") {
		t.Fatalf("rounds must be addressable as iteration.round:\n%s", out)
	}
}

// A design-stage push-back must not reach the coding stage.
func TestRunHistoryFeedbackScopeIsolation(t *testing.T) {
	fh := twoGateRun()
	fh.feedback = []models.FeedbackEvent{
		reviewEvent("design", 1, 1, "设计要用直角", "feedback.review.design.i1r1.json"),
		{RunID: "r", Seq: 2, Round: 1, Kind: models.FeedbackKindGate, NodeID: "gate1", Iteration: 1,
			Text: "门禁意见:配色偏暗", ArtifactName: "feedback.gate.gate1.i1r1.json"},
	}
	h := NewHost(&memStore{})
	h.SetHistoryProvider(fh)

	design, _ := h.RunHistory("r", "design", false, false)
	if !strings.Contains(design, "设计要用直角") || !strings.Contains(design, "配色偏暗") {
		t.Fatalf("design must see its own round and its gate's round:\n%s", design)
	}
	code, _ := h.RunHistory("r", "code", false, false)
	if strings.Contains(code, "设计要用直角") || strings.Contains(code, "配色偏暗") {
		t.Fatalf("design-stage feedback leaked into the coding stage:\n%s", code)
	}
	all, _ := h.RunHistory("r", "code", true, false)
	if !strings.Contains(all, "设计要用直角") {
		t.Fatalf("all=true must drop the scope:\n%s", all)
	}
}

// The rollback entries were always in the trace; the reader simply ignored
// them, leaving a re-run node blind to the fact that it was sent back.
func TestRunHistorySurfacesRollbackToTargetNode(t *testing.T) {
	fh := twoGateRun()
	fh.run.Trace = []models.TraceEntry{
		{NodeID: "gate2", Event: "rollback", To: "code", Kind: models.EdgeRollback,
			Detail: "attempt=2 携带 [last_error] 回滚到 checkpoint"},
	}
	h := NewHost(&memStore{})
	h.SetHistoryProvider(fh)

	code, _ := h.RunHistory("r", "code", false, false)
	if !strings.Contains(code, "回退") || !strings.Contains(code, "attempt=2") {
		t.Fatalf("the rolled-back node must see why it is running again:\n%s", code)
	}
	design, _ := h.RunHistory("r", "design", false, false)
	if strings.Contains(design, "attempt=2") {
		t.Fatalf("rollback belongs to its target node only:\n%s", design)
	}
	// A rollback is a machine decision, not human feedback.
	onlyFb, _ := h.RunHistory("r", "code", false, true)
	if strings.Contains(onlyFb, "attempt=2") {
		t.Fatalf("only_feedback must exclude rollbacks:\n%s", onlyFb)
	}
}

func TestRunHistoryOnlyFeedbackKeepsHumanRounds(t *testing.T) {
	fh := twoGateRun()
	fh.feedback = []models.FeedbackEvent{
		reviewEvent("design", 1, 1, "人工打回意见", "feedback.review.design.i1r1.json"),
	}
	h := NewHost(&memStore{})
	h.SetHistoryProvider(fh)

	out, _ := h.RunHistory("r", "design", false, true)
	if !strings.Contains(out, "人工打回意见") {
		t.Fatalf("only_feedback now means all human feedback:\n%s", out)
	}
	if strings.Contains(out, "初版设计") {
		t.Fatalf("only_feedback must drop plain node executions:\n%s", out)
	}
}

func TestExecutionDetailRendersFeedbackRounds(t *testing.T) {
	fh := twoGateRun()
	fh.feedback = []models.FeedbackEvent{
		reviewEvent("design", 2, 1, "第二版仍需调整", "feedback.review.design.i2r1.json"),
		reviewEvent("design", 1, 1, "第一次的意见", "feedback.review.design.i1r1.json"),
	}
	h := NewHost(&memStore{})
	h.SetHistoryProvider(fh)

	d, err := h.ExecutionDetail("r", "design", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d, "第二版仍需调整") || !strings.Contains(d, "feedback.review.design.i2r1.json") {
		t.Fatalf("detail must show the rounds of that execution:\n%s", d)
	}
	if strings.Contains(d, "第一次的意见") {
		t.Fatalf("detail is scoped to one execution:\n%s", d)
	}
}

// The prompt clause exists so an agent actually reads the ledger; injecting it
// when there is nothing to read would be pure noise.
func TestFeedbackBriefOnlyReportsInScopeRounds(t *testing.T) {
	fh := twoGateRun()
	fh.feedback = []models.FeedbackEvent{
		reviewEvent("design", 1, 1, "用直角", "feedback.review.design.i1.json"),
		{RunID: "r", Seq: 2, Round: 1, Kind: models.FeedbackKindClarify, NodeID: "design", Iteration: 1,
			Text: "自动采纳", IndexOnly: true},
	}
	h := NewHost(&memStore{})
	h.SetHistoryProvider(fh)

	n, cites := h.FeedbackBrief("r", "design")
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	if len(cites) != 1 || !strings.Contains(cites[0], "feedback.review.design.i1.json") {
		t.Fatalf("only rounds with products can be cited: %v", cites)
	}
	if n, _ := h.FeedbackBrief("r", "code"); n != 0 {
		t.Fatalf("out-of-scope node must report zero, got %d", n)
	}
	if n, _ := h.FeedbackBrief("r", ""); n != 0 {
		t.Fatalf("unknown node must report zero, got %d", n)
	}
}

func TestFoldHistoryLinesKeepsHeadAndTail(t *testing.T) {
	line := strings.Repeat("x", 500)
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("%02d-%s", i, line)
	}
	got := foldHistoryLines(lines)
	if len(got) >= len(lines) {
		t.Fatalf("oversized overview must fold, got %d lines", len(got))
	}
	if got[0] != lines[0] {
		t.Fatal("the earliest constraint must survive")
	}
	if got[len(got)-1] != lines[len(lines)-1] {
		t.Fatal("the newest instruction must survive")
	}
	marker := 0
	for _, l := range got {
		if strings.Contains(l, "省略") {
			marker++
		}
	}
	if marker != 1 {
		t.Fatalf("exactly one fold marker expected, got %d", marker)
	}
	// Small inputs pass through untouched.
	small := []string{"a", "b", "c"}
	if len(foldHistoryLines(small)) != 3 {
		t.Fatal("within budget must not fold")
	}
}

func TestFoldFeedbackArtifactsCollapsesRounds(t *testing.T) {
	in := []ArtifactInfo{
		{Name: "research.json", Node: "design", Size: 10},
		{Name: "feedback.review.design.i1r1.json", Node: "design", Size: 20},
		{Name: "feedback.review.design.i1r2.json", Node: "design", Size: 30},
		{Name: FeedbackIndexArtifactName, Size: 40},
	}
	out := foldFeedbackArtifacts(in)
	if len(out) != 2 {
		t.Fatalf("rounds must fold behind the index, got %d: %+v", len(out), out)
	}
	var index *ArtifactInfo
	for i := range out {
		if out[i].Name == FeedbackIndexArtifactName {
			index = &out[i]
		}
		if strings.HasPrefix(out[i].Name, FeedbackArtifactPrefix) {
			t.Fatalf("per-round product still listed: %q", out[i].Name)
		}
	}
	if index == nil || !strings.Contains(index.Note, "2") {
		t.Fatalf("the index entry must say how many rounds it stands for: %+v", index)
	}
	// Rounds present without an index still get a pointer to read.
	out2 := foldFeedbackArtifacts([]ArtifactInfo{{Name: "feedback.gate.g1.i1r1.json"}})
	if len(out2) != 1 || out2[0].Name != FeedbackIndexArtifactName {
		t.Fatalf("missing index must be synthesized: %+v", out2)
	}
	// Nothing to fold ⇒ list unchanged.
	plain := []ArtifactInfo{{Name: "plan.json"}}
	if got := foldFeedbackArtifacts(plain); len(got) != 1 || got[0].Name != "plan.json" {
		t.Fatalf("plain list changed: %+v", got)
	}
}

// The overview line is often all an agent reads about a round, so each kind and
// action has to name itself in plain words rather than leak a raw enum.
func TestFeedbackLineLabelsAndGist(t *testing.T) {
	kinds := map[string]string{
		models.FeedbackKindClarify: "澄清",
		models.FeedbackKindReview:  "复审",
		models.FeedbackKindGate:    "门禁",
		models.FeedbackKindPreview: "预览问题单",
		"something-else":           "反馈",
	}
	for kind, want := range kinds {
		if got := feedbackKindLabel(kind); got != want {
			t.Errorf("kind %q → %q, want %q", kind, got, want)
		}
	}

	actions := []struct {
		name string
		ev   models.FeedbackEvent
		want string
	}{
		{"auto answer", models.FeedbackEvent{Kind: models.FeedbackKindClarify, Action: "auto_answer"}, "自动采纳推荐项"},
		{"review", models.FeedbackEvent{Kind: models.FeedbackKindReview}, "人工打回"},
		{"clarify", models.FeedbackEvent{Kind: models.FeedbackKindClarify}, "人工回答"},
		{"gate", models.FeedbackEvent{Kind: models.FeedbackKindGate, Action: "revise"}, "人工决定 action=revise"},
		{"preview", models.FeedbackEvent{Kind: models.FeedbackKindPreview}, "人工问题单"},
		{"unknown", models.FeedbackEvent{Kind: "x"}, "人工反馈"},
	}
	for _, c := range actions {
		if got := feedbackActionLabel(c.ev); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}

	gists := []struct {
		name string
		ev   models.FeedbackEvent
		want string
	}{
		{"text first", models.FeedbackEvent{Text: "\n第一行\n第二行"}, "第一行"},
		{"annotation note", models.FeedbackEvent{
			Annotations: []models.ReactAnnotation{{Note: ""}, {Note: "补链接"}}}, "补链接"},
		{"human turn", models.FeedbackEvent{
			Turns: []models.ReactMessage{{Role: "agent", Text: "机器"}, {Role: "human", Text: "人说的"}}}, "人说的"},
		{"nothing", models.FeedbackEvent{}, "(无正文)"},
	}
	for _, c := range gists {
		if got := feedbackGist(c.ev); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// A round whose node no longer exists in the graph must not become visible
// everywhere — an unresolvable scope is a closed scope.
func TestFeedbackInScopeRequiresAKnownNode(t *testing.T) {
	g := twoGateRun().run.Graph
	ev := reviewEvent("design", 1, 1, "意见", "")
	if feedbackInScope(g, ev, "") {
		t.Fatal("no current node ⇒ nothing is in scope")
	}
	if !feedbackInScope(g, ev, "design") {
		t.Fatal("a node sees its own rounds")
	}
	if feedbackInScope(g, ev, "code") {
		t.Fatal("a review round belongs to its own node only")
	}
	orphan := models.FeedbackEvent{Kind: models.FeedbackKindGate, NodeID: "ghost-gate"}
	if feedbackInScope(g, orphan, "code") {
		t.Fatal("a gate that is not in the graph reviews nothing")
	}
}

// The ledger records what humans said; it must not be rewritable through the
// human-edit path, or it stops being evidence.
func TestFeedbackProductsRejectHumanEdit(t *testing.T) {
	for _, name := range []string{FeedbackIndexArtifactName, "feedback.review.design.i1r1.json"} {
		if _, err := ValidateHumanArtifactContent(name, `{"a":1}`); err == nil {
			t.Fatalf("%s must not be human-editable", name)
		}
	}
	if _, err := ValidateHumanArtifactContent("research.json",
		`{"summary":"s","findings":[{"title":"t","detail":"d"}]}`); err != nil {
		t.Fatalf("a normal product must stay editable: %v", err)
	}
}
