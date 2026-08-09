package channels

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"gorm.io/gorm"
)

// fakeLive stands in for a conversation-model endpoint.
//
// Routing calls and phrasing calls are told apart by whether tools were
// offered, because that is the real difference between them: routing decides
// what to do and needs the tools, phrasing only rewords a conclusion the
// platform already has. Each routing call consumes one scripted decision;
// running out means the script and the conversation disagree, which is a test
// failure worth seeing.
type fakeLive struct {
	configured bool
	timeout    time.Duration
	decisions  []liveagent.Result
	err        error
	// report answers the phrasing call. nil leaves it unanswered, which is what
	// production does when the endpoint is slow: the conclusion goes out in the
	// work layer's own words.
	report *liveagent.Result
	// reports, when set, are consumed in order for successive no-tools calls
	// (retry ack then conclusion phrasing).
	reports []liveagent.Result
	seen    [][]liveagent.Message
	systems []string
}

func (f *fakeLive) Configured() bool { return f.configured }

func (f *fakeLive) Timeout() time.Duration { return f.timeout }

func (f *fakeLive) Complete(_ context.Context, req liveagent.Request) (liveagent.Result, error) {
	if len(req.Tools) == 0 {
		// Phrasing is a separate hop from tool routing: production may still
		// get a short ack out after a timed-out routing call.
		if len(f.reports) > 0 {
			next := f.reports[0]
			f.reports = f.reports[1:]
			return next, nil
		}
		if f.report == nil {
			return liveagent.Result{}, errors.New("fakeLive: no phrasing configured")
		}
		return *f.report, nil
	}
	f.seen = append(f.seen, req.Messages)
	f.systems = append(f.systems, req.System)
	if f.err != nil {
		return liveagent.Result{}, f.err
	}
	if len(f.decisions) == 0 {
		return liveagent.Result{}, errors.New("fakeLive: no scripted decision left")
	}
	next := f.decisions[0]
	f.decisions = f.decisions[1:]
	return next, nil
}

func liveAnswer(text string) liveagent.Result { return liveagent.Result{Text: text} }

// liveDispatch scripts a lookup delegation. user_reply is required in production;
// tests supply a Live-authored line so the platform never stitches one.
func liveDispatch(request, title string) liveagent.Result {
	return liveagent.Result{ToolName: dispatchPMTool, Args: map[string]string{
		"request": request, "difficulty": string(DifficultyLookup),
		"short_title": title,
		"user_reply":  "行，那块我让人去查，有进展回你。",
	}}
}

// liveHeavyDispatch scripts a delegation that answers the user first.
func liveHeavyDispatch(request, title, userReply string) liveagent.Result {
	return liveagent.Result{ToolName: dispatchPMTool, Args: map[string]string{
		"request": request, "difficulty": string(DifficultyHeavy),
		"short_title": title, "user_reply": userReply,
	}}
}

func liveGetStatus(taskID string) liveagent.Result {
	return liveagent.Result{ToolName: getStatusTool, Args: map[string]string{"task_id": taskID}}
}

func liveCancel(taskID string) liveagent.Result {
	return liveagent.Result{ToolName: cancelWorkTool, Args: map[string]string{"task_id": taskID}}
}

func liveRefine(request, taskID, userReply string) liveagent.Result {
	return liveagent.Result{ToolName: refineWorkTool, Args: map[string]string{
		"request": request, "task_id": taskID, "user_reply": userReply,
	}}
}

// gptLive is the inbound pipeline over a real database: DB-backed delivery
// policy, a bridge-backed transcript, and a fake adapter standing in for QQ.
type gptLive struct {
	t      *testing.T
	m      *Manager
	rc     *runningChannel
	fa     *fakeAdapter
	pm     *services.PmService
	bridge *ChannelBridge
	db     *gorm.DB
	agent  []InboundMessage
}

func newGPTLive(t *testing.T) *gptLive {
	t.Helper()
	fa := &fakeAdapter{}
	m, db := policyManager(t, fa, nil)
	if err := db.Create(&models.Project{
		ID: "proj", Name: "P", PmLeaderEnabled: true, PmLeaderAgent: "pm",
		SandboxEnv: []models.EnvEntry{}, Variables: []models.ProjectVariable{},
	}).Error; err != nil {
		t.Fatal(err)
	}
	pm := services.NewPmService(db, nil)
	bridge := NewChannelBridge(pm, nil, nil, MCPTokenHooks{})
	m.SetTranscript(bridge)
	m.SetRiskConfirmationService(services.NewRiskConfirmationService(db))
	m.SetTaskContextService(services.NewTaskContextService(db))
	m.SetLiveSampleService(services.NewLiveSampleService(db))

	rc := testRunningChannel(fa)
	rc.cfg.ProjectID = "proj"
	g := &gptLive{t: t, m: m, rc: rc, fa: fa, pm: pm, bridge: bridge, db: db}
	m.handleFunc = func(_ context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		g.agent = append(g.agent, in)
		return Reply{FinalSummary: "agent-answer"}, nil
	}
	return g
}

func (g *gptLive) say(id, text string, images ...Image) {
	g.t.Helper()
	g.m.dispatch(context.Background(), g.rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
		MessageID: id, Text: text, Images: images,
	})
}

func (g *gptLive) threadID() string {
	g.t.Helper()
	threads, err := g.pm.ListThreads("proj", SyntheticUserID("qq", SceneC2C, "user1"))
	if err != nil || len(threads) == 0 {
		g.t.Fatalf("no conversation thread was created (err=%v)", err)
	}
	return threads[0].ID
}

func (g *gptLive) transcript() []string {
	g.t.Helper()
	msgs, err := g.pm.CanonicalWindow(g.threadID(), 50)
	if err != nil {
		g.t.Fatal(err)
	}
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Role+": "+m.Content)
	}
	return out
}

// briefFor renders the work brief the sandbox would receive for the escalated
// turn, which is where "what does the agent actually get told" is decided.
func (g *gptLive) briefFor(in InboundMessage) string {
	g.t.Helper()
	thread, err := g.pm.GetThreadByID(g.threadID())
	if err != nil {
		g.t.Fatal(err)
	}
	current, err := g.pm.GetMessage(thread.ID, in.RecordedMessageID)
	if err != nil {
		g.t.Fatalf("escalated turn has no transcript row: %v", err)
	}
	return g.bridge.buildWorkBrief(thread, current, in)
}

// A configured conversation model answers chat itself, in one message, without
// ever opening a sandbox.
func TestLiveModelAnswersChatWithoutTheAgent(t *testing.T) {
	g := newGPTLive(t)
	live := &fakeLive{configured: true, decisions: []liveagent.Result{liveAnswer("我在，说吧。")}}
	g.m.SetLiveModel(live)

	g.say("m1", "你好")

	if got := sentTexts(g.fa); len(got) != 1 || got[0] != "我在，说吧。" {
		t.Fatalf("sends = %v want the model's single answer", got)
	}
	if len(g.agent) != 0 {
		t.Fatalf("a greeting opened a sandbox turn: %v", g.agent)
	}
	assertNoBannedOutbound(t, sentTexts(g.fa))
}

// Anything that needs the repository is delegated, and the delegation carries
// the request plus the attachments the user sent with it.
func TestDispatchCarriesTextAndAttachments(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{
		configured: true,
		decisions:  []liveagent.Result{liveDispatch("按这张截图修登录页", "登录页")},
	})

	g.say("m1", "按这张图修一下", Image{
		Data: []byte("PNGDATA"), MimeType: "image/png", Filename: "shot.png",
	})

	if len(g.agent) != 1 {
		t.Fatalf("delegation did not reach the agent: %v", g.agent)
	}
	got := g.agent[0]
	if got.Text != "按这张图修一下" {
		t.Fatalf("agent saw text %q", got.Text)
	}
	if len(got.Images) != 1 || got.Images[0].Filename != "shot.png" {
		t.Fatalf("attachment did not survive routing: %+v", got.Images)
	}
	if got.Dispatch == nil || !strings.Contains(got.Dispatch.Brief, "登录页") {
		t.Fatalf("agent cannot tell why it was called: %+v", got.Dispatch)
	}
	if got.Dispatch.ShortTitle != "登录页" {
		t.Fatalf("task title lost: %+v", got.Dispatch)
	}
	// The bytes are stored too, so a later turn can fetch them.
	msgs, err := g.pm.CanonicalWindow(g.threadID(), 10)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("transcript = %v err=%v", msgs, err)
	}
	if len(msgs[0].Images) != 1 || msgs[0].Images[0].Name != "shot.png" {
		t.Fatalf("attachment was not recorded: %+v", msgs[0].Images)
	}
}

// Without a configured endpoint the message goes to the agent. Nothing is
// dropped and nothing claims a conversation model was consulted.
func TestNoConversationModelSendsEverythingToTheAgent(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: false})

	g.say("m1", "你好")

	if len(g.agent) != 1 {
		t.Fatalf("message did not reach the agent: %v", g.agent)
	}
	if g.agent[0].EscalationReason != "" {
		t.Fatalf("a turn nobody routed claims a routing reason: %q", g.agent[0].EscalationReason)
	}
	if got := sentTexts(g.fa); len(got) != 1 || got[0] != "agent-answer" {
		t.Fatalf("sends = %v want the agent's answer", got)
	}
}

func TestRetryAffirmationAfterFailureRedispatchesInsteadOfConfirmAck(t *testing.T) {
	g := newGPTLive(t)
	if _, err := g.m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-fail-retry", ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		ShortTitle: "错误处理与日志", Status: "failed",
		OriginalRequirement: "修复 Approving 错误处理与日志链路",
		OriginChannel:       "qq", OriginScene: string(SceneC2C), OriginConversationID: "user1",
	}); err != nil {
		t.Fatal(err)
	}
	// Live can phrase the ack (no tools ⇒ report path) then we dispatch.
	ack := "行，那块错误处理我让人重新开干，有进展回你。"
	g.m.SetLiveModel(&fakeLive{configured: true, report: &liveagent.Result{Text: ack}})
	g.say("m-retry", "重跑啊")
	got := sentTexts(g.fa)
	if len(got) < 1 {
		t.Fatalf("sends = %v", got)
	}
	if got[0] != ack {
		t.Fatalf("retry ack should be Live-phrased GM voice, got %q want %q", got[0], ack)
	}
	if strings.Contains(got[0], "确认") || strings.Contains(got[0], "优先级") {
		t.Fatalf("retry ack still sounds like a ticket: %q", got[0])
	}
	if len(g.agent) != 1 {
		t.Fatalf("retry must reach the sandbox agent, got %d turns", len(g.agent))
	}
	if d := g.agent[0].Dispatch; d == nil || !strings.Contains(d.Brief, "错误处理") {
		t.Fatalf("sandbox brief missing original requirement: %+v", g.agent[0].Dispatch)
	}
}

func TestRetryAffirmationSkipsAckWhenLiveCannotPhrase(t *testing.T) {
	g := newGPTLive(t)
	if _, err := g.m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-fail-retry2", ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		ShortTitle: "错误处理与日志", Status: "failed",
		OriginalRequirement: "修复 Approving 错误处理与日志链路",
		OriginChannel:       "qq", OriginScene: string(SceneC2C), OriginConversationID: "user1",
	}); err != nil {
		t.Fatal(err)
	}
	g.m.SetLiveModel(&fakeLive{configured: true, err: liveagent.ErrBudgetExhausted})
	g.say("m-retry2", "重跑啊")
	got := sentTexts(g.fa)
	for _, line := range got {
		if strings.Contains(line, "确认") || strings.Contains(line, "重新开干") || strings.Contains(line, "重新安排") {
			t.Fatalf("must not stitch a platform retry sentence when Live fails: %v", got)
		}
	}
	if len(g.agent) != 1 {
		t.Fatalf("retry must still reach the sandbox agent, got %d turns", len(g.agent))
	}
}

func TestRetryAffirmationDropsAckThatQuotesLongTitle(t *testing.T) {
	g := newGPTLive(t)
	title := "重新执行上次因服务重启而中断的 Approvin"
	if _, err := g.m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-fail-retry3", ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		ShortTitle: title, Status: "failed",
		OriginalRequirement: "重新执行上次因服务重启而中断的 Approving 错误处理与日志链路的修复",
		OriginChannel:       "qq", OriginScene: string(SceneC2C), OriginConversationID: "user1",
	}); err != nil {
		t.Fatal(err)
	}
	bad := "行，「" + title + "」我让人重新开干，有进展回你。"
	g.m.SetLiveModel(&fakeLive{configured: true, reports: []liveagent.Result{
		{Text: bad},
		{Text: "那事还在跑，有结果回你。"},
	}})
	g.say("m-retry3", "重试下")
	got := sentTexts(g.fa)
	for _, line := range got {
		if strings.Contains(line, "「") || strings.Contains(line, "重新执行") {
			t.Fatalf("quoted long title must not be sent: %v", got)
		}
	}
	if len(g.agent) != 1 {
		t.Fatalf("retry must still dispatch, got %d turns", len(g.agent))
	}
}

func TestRetryAckUnusable(t *testing.T) {
	title := "重新执行上次因服务重启而中断的 Approvin"
	if !retryAckUnusable("行，「"+title+"」我让人重新开干，有进展回你。", title, "") {
		t.Fatal("quoted title must be unusable")
	}
	if !retryAckUnusable("已经重新重试过了，目前任务已经进到队列里了。", title, "") {
		t.Fatal("past-tense already-retried must be unusable")
	}
	if retryAckUnusable("行，那事我正让人重新开干，有进展回你。", title, "") {
		t.Fatal("in-progress Live line must stay usable")
	}
}

func TestGetStatusSurfacesFailedTerminalTasks(t *testing.T) {
	g := newGPTLive(t)
	if _, err := g.m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		RunID: "run-fail-status", ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		ShortTitle: "统一错误码", Status: "failed",
		OriginChannel: "qq", OriginScene: string(SceneC2C), OriginConversationID: "user1",
	}); err != nil {
		t.Fatal(err)
	}
	raw := g.m.runGetStatus(g.rc, InboundMessage{UserID: "u1", ConversationID: "user1"}, "")
	if !strings.Contains(raw, "failed") || !strings.Contains(raw, "统一错误码") {
		t.Fatalf("get_status missing failed terminal: %s", raw)
	}
	if !strings.Contains(raw, "recent_terminal") && !strings.Contains(raw, `"status":"failed"`) {
		t.Fatalf("get_status should expose terminal status explicitly: %s", raw)
	}
	if !strings.Contains(raw, "禁止把空的在跑列表说成") {
		t.Fatalf("note must forbid inventing success from an empty active list: %s", raw)
	}
	if !strings.Contains(raw, "重试") || !strings.Contains(raw, "搁置") {
		t.Fatalf("failed status must ask the user to choose next step: %s", raw)
	}
	brief := g.m.buildDirectorContext(g.rc, InboundMessage{UserID: "u1", ConversationID: "user1"}).render()
	if !strings.Contains(brief, "failed") || !strings.Contains(brief, "不等于") {
		t.Fatalf("briefing must warn empty≠done and list failure: %s", brief)
	}
	if !strings.Contains(brief, "让对方选") {
		t.Fatalf("briefing must require offering choices on failure: %s", brief)
	}
}

func TestLiveCallTimeoutFollowsConfiguredLiveTimeout(t *testing.T) {
	m := managerWithLive(&fakeLive{configured: true, timeout: 300 * time.Second})
	if got := m.liveCallTimeout(45 * time.Second); got != 300*time.Second {
		t.Fatalf("liveCallTimeout = %v want settings value 300s", got)
	}
	m.SetLiveModel(&fakeLive{configured: true}) // Timeout() == 0
	if got := m.liveCallTimeout(45 * time.Second); got != 45*time.Second {
		t.Fatalf("liveCallTimeout = %v want fallback", got)
	}
}

func TestFallthroughAckUsesLiveNotStitchedConfirm(t *testing.T) {
	g := newGPTLive(t)
	identity := g.seedTask("run-focus1", "错误处理完整性")
	scope := g.m.taskScopeFor(g.rc, InboundMessage{UserID: "u1", ConversationID: "user1"})
	if _, err := g.m.taskContext.SetFocus(scope, identity, ""); err != nil {
		t.Fatal(err)
	}
	// Routing fails; phrasing (no tools) still works via reports.
	ack := "错误处理那块我让人接着查，有进展回你。"
	g.m.SetLiveModel(&fakeLive{
		configured: true,
		err:        liveagent.ErrBudgetExhausted,
		reports:    []liveagent.Result{{Text: ack}},
	})
	g.say("m-fix", "你看看怎么修复下呢")
	got := sentTexts(g.fa)
	if len(got) < 1 || got[0] != ack {
		t.Fatalf("fallthrough ack should be Live-phrased, got %v want %q first", got, ack)
	}
	if strings.Contains(got[0], "确认") || strings.Contains(got[0], "怎么修复") {
		t.Fatalf("fallthrough ack still stitched/echoed: %q", got[0])
	}
}

// A model that fails must still hand the turn to the agent. Platform must not
// stamp 「我这就去确认」when Live cannot phrase the fallthrough either.
func TestConversationModelFailureStillAnswersTheUser(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, err: liveagent.ErrBudgetExhausted})

	g.say("m1", "什么进度了")

	if len(g.agent) != 1 {
		t.Fatalf("a failed model call swallowed the message: %v", g.agent)
	}
	got := sentTexts(g.fa)
	if len(got) != 1 || got[0] != "agent-answer" {
		t.Fatalf("sends = %v want only the agent's answer (no stitched confirm ack)", got)
	}
}

// The prompt handed to the agent must not be the conversation.
//
// Replaying recent turns into every prompt is what this layer replaced: it grew
// without bound, crowded out the question, and taught the agent to repeat
// answers the user already had. What the agent gets instead is a pointer to
// where the history lives.
func TestWorkBriefDoesNotReplayTheConversation(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("我在。"),
		liveAnswer("登录页那条我记得，你说的是首屏。"),
		liveDispatch("看仓库确认首屏改动", "首屏改动"),
	}})

	g.say("m1", "在吗")
	g.say("m2", "登录页那个还记得吗")
	g.say("m3", "去仓库确认一下改了没")

	if len(g.agent) != 1 {
		t.Fatalf("agent turns = %d want exactly the delegated one", len(g.agent))
	}
	// One record, both layers: two turns the model answered, then the delegated
	// turn's acknowledgement, then the agent's reply to it.
	want := []string{
		"user: 在吗",
		"assistant: 我在。",
		"user: 登录页那个还记得吗",
		"assistant: 登录页那条我记得，你说的是首屏。",
		"user: 去仓库确认一下改了没",
		"assistant: 行，那块我让人去查，有进展回你。",
		"assistant: agent-answer",
	}
	got := g.transcript()
	if len(got) != len(want) {
		t.Fatalf("transcript = %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("transcript[%d] = %q want %q (all=%v)", i, got[i], want[i], got)
		}
	}

	brief := g.briefFor(g.agent[0])
	if strings.Contains(brief, "conversation_handoff") {
		t.Fatalf("the removed handoff block came back:\n%s", brief)
	}
	for _, replayed := range []string{"在吗", "我在。", "登录页那个还记得吗", "你说的是首屏"} {
		if strings.Contains(brief, replayed) {
			t.Fatalf("brief replayed the conversation (%q):\n%s", replayed, brief)
		}
	}
	if !strings.Contains(brief, "看仓库确认首屏改动") {
		t.Fatalf("brief does not say what the agent was asked for:\n%s", brief)
	}
	if !strings.Contains(brief, "get_messages") {
		t.Fatalf("brief does not tell the agent where the history is:\n%s", brief)
	}
}

// A picture sent one turn and referred to the next must still be reachable. It
// is named rather than resent: the pointer is everything get_attachment needs,
// and the bytes would be carried again on every later turn.
func TestHistoryAttachmentIsNamedRatherThanReplayed(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("收到，我看看。"),
		liveDispatch("按上一条的图改", "按图修改"),
	}})

	g.say("m1", "这是报错截图", Image{
		Data: []byte("PNGBYTES"), MimeType: "image/png", Filename: "err.png",
	})
	g.say("m2", "按刚才那张图修")

	brief := g.briefFor(g.agent[0])
	for _, must := range []string{"err.png", "image/png", "get_attachment", "index=0"} {
		if !strings.Contains(brief, must) {
			t.Fatalf("brief lost the attachment pointer %q:\n%s", must, brief)
		}
	}
	if strings.Contains(brief, "PNGBYTES") {
		t.Fatalf("attachment bytes were replayed into the prompt:\n%s", brief)
	}
	// The pointer names a message the agent can actually fetch.
	msgs, err := g.pm.CanonicalWindow(g.threadID(), 10)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("window = %v err=%v", msgs, err)
	}
	if !strings.Contains(brief, msgs[0].ID) {
		t.Fatalf("brief does not name the message holding the file:\n%s", brief)
	}
	// And the conversation layer knew the file existed, so it could point at it.
	if refs := g.agent[0].Dispatch.Attachments; len(refs) == 0 || refs[0].Name != "err.png" {
		t.Fatalf("delegation carried no attachment pointer: %+v", refs)
	}
}

// The file the user just sent travels with the turn as a file. Only history is
// fetched on demand; making the agent fetch what it was handed this second
// would be a round trip for nothing.
func TestCurrentAttachmentIsNotDeferredToContextStore(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveDispatch("看这张图", "看图"),
	}})

	g.say("m1", "看看这个", Image{
		Data: []byte("NOW"), MimeType: "image/png", Filename: "now.png",
	})

	if imgs := g.agent[0].Images; len(imgs) != 1 || imgs[0].Filename != "now.png" {
		t.Fatalf("the current attachment did not travel with the turn: %+v", imgs)
	}
	if brief := g.briefFor(g.agent[0]); strings.Contains(brief, "now.png") {
		t.Fatalf("the current attachment was listed as history to fetch:\n%s", brief)
	}
}

// Minute-scale work is acknowledged before it starts, and the acknowledgement
// says what is being worked on. The conclusion still follows: an
// acknowledgement is not an answer.
func TestHeavyWorkIsAcknowledgedBeforeItStarts(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveHeavyDispatch("排查登录页 500", "登录页报错", "我去翻一下登录页那个 500，翻到就回你。"),
	}})

	g.say("m1", "登录页报 500，看看什么原因")

	got := sentTexts(g.fa)
	if len(got) != 2 {
		t.Fatalf("sends = %v want an acknowledgement then the conclusion", got)
	}
	if !strings.Contains(got[0], "登录页") {
		t.Fatalf("acknowledgement says nothing verifiable: %q", got[0])
	}
	if got[1] != "agent-answer" {
		t.Fatalf("the conclusion was withheld after an acknowledgement: %v", got)
	}
	assertNoBannedOutbound(t, got)
}

// Empty / filler acknowledgements are rejected — the platform must not stitch a
// 「确认」sentence to stand in for missing Live voice.
func TestEmptyAcknowledgementIsRejectedNotStitched(t *testing.T) {
	text, flags := gateAcknowledgement("好的，稍等，我看一下", "登录页报错", "登录页报 500")
	if text != "" || len(flags) != 1 || flags[0] != "empty_ack" {
		t.Fatalf("filler ack must be rejected, got %q flags=%v", text, flags)
	}

	kept, flags := gateAcknowledgement("我去翻一下登录页那个 500，翻到就回你。", "登录页报错", "")
	if kept != "我去翻一下登录页那个 500，翻到就回你。" || len(flags) != 0 {
		t.Fatalf("an informative acknowledgement was rewritten: %q flags=%v", kept, flags)
	}

	plain, flags := gateAcknowledgement("", "", "项目的错误处理完整吗")
	if plain != "" || len(flags) != 1 {
		t.Fatalf("empty ack must stay empty (no stitched confirm): %q flags=%v", plain, flags)
	}
	confirm, flags := gateAcknowledgement("我这就去确认，有结果马上回你。", "登录页报错", "")
	if confirm != "" || len(flags) != 1 {
		t.Fatalf("confirm stamp must be rejected: %q flags=%v", confirm, flags)
	}
}

// Claiming "已经做完了" while the ledger still has in-flight work must not
// reach the user (the 调研-v3-at-50% false completion).
func TestPrematureDoneClaimWhileTaskRunningIsBlocked(t *testing.T) {
	g := newGPTLive(t)
	_ = g.seedTask("run-e50054fd", "快模型和 worker 架构精简分析")
	g.m.noteWorkProgress("proj", "run-e50054fd", "方案报告页", false)

	live := &fakeLive{
		configured: true,
		decisions:  []liveagent.Result{liveAnswer("快模型和 worker 架构精简分析已经做完了。结论是确实有可以精简的方法。")},
		report:     ptrResult(liveAnswer("还在方案报告页，还没跑完。")),
	}
	g.m.SetLiveModel(live)

	g.say("m1", "大概多久啊 你就弄这个吗")

	got := sentTexts(g.fa)
	if len(got) != 1 {
		t.Fatalf("sends = %v", got)
	}
	if claimsActiveWorkFinished(got[0]) {
		t.Fatalf("premature done still shipped: %q", got[0])
	}
	if !strings.Contains(got[0], "方案报告") && !strings.Contains(got[0], "还没") && !strings.Contains(got[0], "还在") {
		t.Fatalf("rewrite should say still running: %q", got[0])
	}
}

func ptrResult(r liveagent.Result) *liveagent.Result { return &r }

// "How's it going" is answered from the ledger, and answering it must not start
// anything. Opening a sandbox to find out where a task is, is how a progress
// question used to cost a container.
func TestProgressQuestionIsAnsweredFromTheLedgerWithoutStartingWork(t *testing.T) {
	g := newGPTLive(t)
	identity := g.seedTask("run-1", "登录页报错")
	g.m.noteWorkProgress("proj", "run-1", "正在查代码", false)

	live := &fakeLive{configured: true, decisions: []liveagent.Result{
		liveGetStatus(identity.ID),
		liveAnswer("还在跑，现在在查代码。"),
	}}
	g.m.SetLiveModel(live)

	g.say("m1", "好了没")

	if len(g.agent) != 0 {
		t.Fatalf("a progress question opened a sandbox turn: %v", g.agent)
	}
	if got := sentTexts(g.fa); len(got) != 1 || got[0] != "还在跑，现在在查代码。" {
		t.Fatalf("sends = %v want one status reply", got)
	}
	// The status the model answered from was the platform's, not its own idea
	// of one.
	last := live.seen[len(live.seen)-1]
	fed := last[len(last)-1].Content
	for _, must := range []string{getStatusTool, "登录页报错", "正在查代码"} {
		if !strings.Contains(fed, must) {
			t.Fatalf("tool result did not carry %q: %s", must, fed)
		}
	}
}

// Cancelling acts on a real task and reports what actually stopped.
func TestCancelStopsTheNamedTask(t *testing.T) {
	g := newGPTLive(t)
	identity := g.seedTask("run-1", "登录页报错")
	var cancelled []string
	g.m.SetRiskActionExecutor(func(_, runID, action string, _ map[string]string) error {
		cancelled = append(cancelled, action+":"+runID)
		return nil
	})

	live := &fakeLive{configured: true, decisions: []liveagent.Result{
		liveCancel(identity.ID),
		liveAnswer("好，登录页那个我停了。"),
	}}
	g.m.SetLiveModel(live)

	g.say("m1", "算了别弄了")

	if len(cancelled) != 1 || cancelled[0] != "cancel_run:run-1" {
		t.Fatalf("cancel actions = %v want the named run stopped", cancelled)
	}
	if got := sentTexts(g.fa); len(got) != 1 || got[0] != "好，登录页那个我停了。" {
		t.Fatalf("sends = %v want one confirmation", got)
	}
}

// With two tasks in flight and nothing to point at, cancelling must not guess.
// Stopping the wrong task is not a wording mistake that can be apologised for
// afterwards.
func TestAmbiguousCancelAsksInsteadOfGuessing(t *testing.T) {
	g := newGPTLive(t)
	g.seedTask("run-1", "登录页报错")
	g.seedTask("run-2", "导出超时")
	var cancelled []string
	g.m.SetRiskActionExecutor(func(_, runID, action string, _ map[string]string) error {
		cancelled = append(cancelled, action+":"+runID)
		return nil
	})

	live := &fakeLive{configured: true, decisions: []liveagent.Result{
		liveCancel(""),
		liveAnswer("你是说登录页那个，还是导出那个？"),
	}}
	g.m.SetLiveModel(live)

	g.say("m1", "算了别弄了")

	if len(cancelled) != 0 {
		t.Fatalf("an ambiguous cancel stopped something anyway: %v", cancelled)
	}
	last := live.seen[len(live.seen)-1]
	fed := last[len(last)-1].Content
	if !strings.Contains(fed, "ambiguous") {
		t.Fatalf("the model was not told the request was ambiguous: %s", fed)
	}
	if got := sentTexts(g.fa); len(got) != 1 || !strings.Contains(got[0], "还是") {
		t.Fatalf("sends = %v want a question back", got)
	}
}

// A sandbox-bound lookup still acknowledges first. Staying silent until the
// agent finishes was the "快模型没有先回复" failure: the wait flag assumed a
// sub-second SoT check that this path does not perform.
func TestLookupDispatchAcknowledgesBeforeTheAgent(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveDispatch("确认登录页那个修好没", "登录页报错"),
	}})

	g.say("m1", "登录页那个修复了没")

	got := sentTexts(g.fa)
	if len(got) != 2 {
		t.Fatalf("sends = %v want an acknowledgement then the conclusion", got)
	}
	if strings.Contains(got[0], "确认") || strings.TrimSpace(got[0]) == "" {
		t.Fatalf("acknowledgement must be Live-authored, not a confirm stamp: %q", got[0])
	}
	if got[1] != "agent-answer" {
		t.Fatalf("conclusion = %q want agent-answer", got[1])
	}
	if len(g.agent) != 1 {
		t.Fatalf("the check never reached the agent: %v", g.agent)
	}
}

// A cancelled task must stop reading as running, or the next status question is
// answered from the ledger with work nobody is doing.
func TestCancelUpdatesTheLedger(t *testing.T) {
	g := newGPTLive(t)
	identity := g.seedTask("run-1", "登录页报错")
	g.m.SetRiskActionExecutor(func(string, string, string, map[string]string) error { return nil })
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveCancel(identity.ID),
		liveAnswer("好，停了。"),
	}})

	g.say("m1", "算了别弄了")

	var stored models.TaskIdentity
	if err := g.db.First(&stored, "id = ?", identity.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "cancelled" || stored.TerminalAt == nil {
		t.Fatalf("cancelled task still reads as %q (terminal=%v)", stored.Status, stored.TerminalAt)
	}
	if focus := g.m.focusTaskID(g.rc, InboundMessage{
		Scene: SceneC2C, ConversationID: "user1", UserID: "u1",
	}); focus == identity.ID {
		t.Fatal("the conversation still points at the task it just cancelled")
	}
}

// With two things in flight an update has to say which one moved. With one, the
// conversation already says it and a title in front reads like a ticket header.
func TestProgressNamesTheTaskOnlyWhenSeveralAreRunning(t *testing.T) {
	g := newGPTLive(t)
	g.seedTask("run-1", "登录页报错")
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveDispatch("查导出超时", "导出超时"),
	}})
	g.m.handleFunc = nil
	g.m.handleFuncWithProgress = func(_ context.Context, _ ResolvedChannel, in InboundMessage,
		onProgress func(ProgressEvent)) (Reply, error) {
		g.agent = append(g.agent, in)
		onProgress(ProgressEvent{
			Kind: ProgressMilestone, Summary: "已经复现了", Stage: "repro",
			RunID: "run-1", Sendable: true,
		})
		return Reply{FinalSummary: "agent-answer"}, nil
	}

	g.say("m1", "导出也看看")

	got := sentTexts(g.fa)
	if len(got) != 3 {
		t.Fatalf("sends = %v want ack, labelled update, and conclusion", got)
	}
	if strings.Contains(got[0], "确认") || strings.TrimSpace(got[0]) == "" {
		t.Fatalf("acknowledgement must be Live-authored, not a confirm stamp: %q", got[0])
	}
	if !strings.HasPrefix(got[1], "「登录页报错」") {
		t.Fatalf("update does not say which task moved: %q", got[1])
	}
}

// Past the concurrency limit the platform declines and the conversation layer
// explains. A silent rejection and a platform-worded refusal are both worse:
// one loses the request, the other puts a template back in the conversation.
func TestConcurrencyLimitIsExplainedRatherThanImposedSilently(t *testing.T) {
	g := newGPTLive(t)
	for i, title := range []string{"登录页", "导出", "搜索"} {
		g.seedTask(string(rune('a'+i))+"-run", title)
	}
	live := &fakeLive{configured: true, decisions: []liveagent.Result{
		liveHeavyDispatch("再修一个", "订单页", "我去看订单页。"),
		liveAnswer("我手上还有三件事在跑，先停一个再开新的？"),
	}}
	g.m.SetLiveModel(live)

	g.say("m1", "订单页也修一下")

	if len(g.agent) != 0 {
		t.Fatalf("a rejected delegation reached the agent anyway: %v", g.agent)
	}
	last := live.seen[len(live.seen)-1]
	toolMsg := last[len(last)-1].Content
	if !strings.Contains(toolMsg, "rejected") || !strings.Contains(toolMsg, "refine_work") {
		t.Fatalf("decline must point the model at refine_work for follow-ups: %s", toolMsg)
	}
	if got := sentTexts(g.fa); len(got) != 1 || !strings.Contains(got[0], "三件事") {
		t.Fatalf("sends = %v want the limit explained in the model's own words", got)
	}
}

// An answered lookup must not linger in the ledger. The temporary dispatch:*
// row is not a Run, so without an explicit close it kept showing up as one of
// the "3 tasks still running" ghosts that poisoned later intent.
func TestAnsweredLookupLeavesNoGhostInTheLedger(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveDispatch("查错误处理是否完整", "错误处理"),
	}})

	g.say("m1", "项目的错误处理完整吗")

	tasks := g.m.taskLedger(g.rc, InboundMessage{UserID: "u1", ConversationID: "user1"})
	if len(tasks) != 0 {
		t.Fatalf("answered lookup still listed as running: %+v", tasks)
	}
}

// A scope follow-up ("重点看 Release 到现在") must hang on the focused task.
// Opening a fourth job and asking the user which of three to cancel is exactly
// the queue-politics reply that should never leave the conversation layer.
func TestRefineWorkFollowsUpWithoutOpeningANewTask(t *testing.T) {
	g := newGPTLive(t)
	focus := g.seedTask("run-err", "查主干错误处理")
	for i, title := range []string{"审计日志", "修 Run/PR"} {
		g.seedTask(string(rune('a'+i))+"-run", title)
	}
	if _, err := g.m.taskContext.SetFocus(g.m.taskScopeFor(g.rc, InboundMessage{
		UserID: "u1", ConversationID: "user1",
	}), focus, ""); err != nil {
		t.Fatal(err)
	}

	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveRefine("重点看最新 Release 到现在的改动，那段比较粗糙", focus.ID, "好，我按 Release 以后这段接着看。"),
	}})

	g.say("m1", "重点看看最新的Release 到现在的版本 做得比较粗糙")

	if len(g.agent) != 1 {
		t.Fatalf("refine did not reach the agent: %v", g.agent)
	}
	d := g.agent[0].Dispatch
	if d == nil || d.TaskID != focus.ID {
		t.Fatalf("refine opened or retargeted work: %+v", d)
	}
	if !strings.Contains(d.Brief, "Release") || !strings.Contains(d.Brief, "补充") {
		t.Fatalf("brief missing the follow-up: %q", d.Brief)
	}
	if got := sentTexts(g.fa); len(got) < 1 || !strings.Contains(got[0], "Release") {
		t.Fatalf("sends = %v want a refine ack, not a queue dump", got)
	}
	for _, line := range sentTexts(g.fa) {
		if strings.Contains(line, "队列") || strings.Contains(line, "先停") {
			t.Fatalf("refine path dumped queue politics: %q", line)
		}
	}
	tasks := g.m.taskLedger(g.rc, InboundMessage{UserID: "u1", ConversationID: "user1"})
	if len(tasks) != 3 {
		t.Fatalf("refine must not create a fourth ledger row: %d", len(tasks))
	}
}

// The agent's conclusion reaches the user through the conversation layer, in
// the voice the user has been hearing. Two speakers in one conversation is what
// made a delegated answer read like a different system replying.
func TestAgentConclusionIsReportedByTheConversationLayer(t *testing.T) {
	g := newGPTLive(t)
	phrased := liveAnswer("查过了，是缓存没刷新导致的。")
	live := &fakeLive{
		configured: true,
		decisions:  []liveagent.Result{liveDispatch("查登录页 500 的根因", "登录页报错")},
		report:     &phrased,
	}
	g.m.SetLiveModel(live)
	g.m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		g.agent = append(g.agent, in)
		if _, err := g.m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: "根因：缓存未刷新，已定位到 CacheWarmer。",
		}); err != nil {
			t.Fatalf("pm_reply failed: %v", err)
		}
		return Reply{}, nil
	}

	g.say("m1", "登录页 500 的根因是什么")

	got := sentTexts(g.fa)
	if len(got) != 2 {
		t.Fatalf("sends = %v want an acknowledgement then the director's conclusion", got)
	}
	if got[1] != "查过了，是缓存没刷新导致的。" {
		t.Fatalf("the work layer spoke to the user directly: %q", got[1])
	}
}

// When the conversation layer cannot phrase the conclusion, the conclusion
// still goes out. Degrading to the work layer's own words reads slightly off;
// staying silent loses the answer the user waited for.
func TestConclusionSurvivesAFailedPhrasingCall(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{
		configured: true,
		decisions:  []liveagent.Result{liveDispatch("查根因", "根因")},
		// report left nil: the phrasing call fails.
	})
	g.m.handleFunc = func(ctx context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		g.agent = append(g.agent, in)
		if _, err := g.m.DeliverConversationReply(ctx, ConversationReply{
			ProjectID: "proj", Scene: in.Scene, ConversationID: in.ConversationID,
			UserID: in.UserID, Text: "缓存没刷新。",
		}); err != nil {
			t.Fatalf("pm_reply failed: %v", err)
		}
		return Reply{}, nil
	}

	g.say("m1", "根因是什么")

	got := sentTexts(g.fa)
	if len(got) != 2 || got[1] != "缓存没刷新。" {
		t.Fatalf("sends = %v want ack then the conclusion in the work layer's own words", got)
	}
}

// A long agent conclusion must not be chopped mid-sentence at 240 runes before
// the director can phrase it. That cut is what produced unfinished outbound
// like 「因此目前审…」.
func TestLongConclusionIsNotChoppedBeforeTheDirector(t *testing.T) {
	g := newGPTLive(t)
	long := strings.Repeat("查过了，审计里没有模型调用事件。", 40) // well over 240 runes
	phrased := liveAnswer("审计里确实没有快模型调用记录，现在只落在决策样本表里。")
	g.m.SetLiveModel(&fakeLive{
		configured: true,
		decisions:  []liveagent.Result{liveDispatch("查审计里有没有快模型调用", "审计日志")},
		report:     &phrased,
	})
	g.m.handleFunc = func(_ context.Context, _ ResolvedChannel, in InboundMessage) (Reply, error) {
		g.agent = append(g.agent, in)
		return Reply{FinalSummary: long}, nil
	}

	g.say("m1", "审计日志里看不到快模型调用")

	got := sentTexts(g.fa)
	if len(got) != 2 {
		t.Fatalf("sends = %v want ack then summarised conclusion", got)
	}
	if got[1] != "审计里确实没有快模型调用记录，现在只落在决策样本表里。" {
		t.Fatalf("director did not get to phrase the long conclusion: %q", got[1])
	}
	if strings.Contains(got[1], "因此目前审") || strings.HasSuffix(got[1], "审…") {
		t.Fatalf("mid-sentence truncation came back: %q", got[1])
	}
}

// Every routing decision is recorded, including what the model was shown and
// what the user received. Without this, the next change to routing can only be
// argued about.
func TestRoutingDecisionsAreRecorded(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("我在，说吧。"),
	}})

	g.say("m1", "你好")

	var samples []models.LiveDecisionSample
	if err := g.db.Find(&samples).Error; err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d want one per decision", len(samples))
	}
	s := samples[0]
	if s.Route != routeReply {
		t.Fatalf("route = %q want %q", s.Route, routeReply)
	}
	if s.UserText != "你好" || !strings.Contains(s.Actions, "live_reply") {
		t.Fatalf("sample does not hold the exchange: %+v", s)
	}
	if !strings.Contains(s.RawCompletion, "我在，说吧。") {
		t.Fatalf("sample does not hold what the model produced: %s", s.RawCompletion)
	}
	if s.DirectorContext == "" || s.Transcript == "" {
		t.Fatalf("sample does not hold what the model was shown: %+v", s)
	}
}

// The agent's own working text is not the conversation. 「已发送」 is what a
// model writes after calling a tool; treating it as a reply is how the platform
// both sent it to the user and then read it back as something the user had
// seen.
func TestAgentInternalTextNeverBecomesConversationHistory(t *testing.T) {
	g := newGPTLive(t)
	thread := g.threadFromFirstMessage()

	if _, err := g.pm.AppendMessageSource(thread, "assistant", "已发送。\n已通过 QQ 回复用户。",
		models.MessageSourceAgentInternal, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	// A legacy row from before sources were tagged is treated the same way:
	// there is no evidence anyone read it, so it is not replayed.
	if _, err := g.pm.AppendMessageSource(thread, "assistant", "旧的内部叙述",
		"", nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	banned := []string{"已发送", "已通过 QQ 回复用户", "旧的内部叙述"}
	for _, line := range g.transcript() {
		for _, b := range banned {
			if strings.Contains(line, b) {
				t.Fatalf("agent working text entered the conversation: %q", line)
			}
		}
	}

	// And the next turn's conversation model must not read it back either. It
	// would answer as though the user had already been told.
	live := &fakeLive{configured: true, decisions: []liveagent.Result{liveAnswer("嗯。")}}
	g.m.SetLiveModel(live)
	g.say("m2", "刚才那个呢")
	if len(live.seen) != 1 {
		t.Fatalf("conversation model calls = %d want 1", len(live.seen))
	}
	for _, msg := range live.seen[0] {
		for _, b := range banned {
			if strings.Contains(msg.Content, b) {
				t.Fatalf("agent working text was fed back to the conversation model: %q", msg.Content)
			}
		}
	}
}

// A turn that failed still happened to the user. Dropping their message along
// with the missing answer is how a question the platform could not answer
// disappears from the record entirely.
func TestFailedTurnKeepsTheUsersMessageInTheRecord(t *testing.T) {
	g := newGPTLive(t)
	thread := g.threadFromFirstMessage()
	msgs, err := g.pm.CanonicalWindow(thread, 10)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("window = %v err=%v", msgs, err)
	}
	if _, err := g.pm.UpdateMessageFailure(thread, msgs[0].ID, "failed", services.PmFailSandbox); err != nil {
		t.Fatal(err)
	}
	after := g.transcript()
	if len(after) == 0 || !strings.Contains(after[0], "记一笔") {
		t.Fatalf("failed turn lost the user's own words: %v", after)
	}
}

// seedTask puts a live task in the ledger, as a dispatch or a Run would.
func (g *gptLive) seedTask(runID, title string) *models.TaskIdentity {
	g.t.Helper()
	identity, err := g.m.taskContext.EnsureIdentity(services.EnsureTaskIdentityInput{
		ProjectID: "proj", UserID: services.SyntheticQQUserID("u1"),
		RunID: runID, ShortTitle: title, Status: "running",
		OriginChannel: "qq", OriginScene: string(SceneC2C), OriginConversationID: "user1",
	})
	if err != nil {
		g.t.Fatal(err)
	}
	return identity
}

// threadFromFirstMessage seeds one recorded user message and returns the thread.
func (g *gptLive) threadFromFirstMessage() string {
	g.t.Helper()
	g.m.SetLiveModel(&fakeLive{configured: false})
	g.say("seed", "记一笔")
	g.agent = nil
	g.fa.mu.Lock()
	g.fa.sent = nil
	g.fa.mu.Unlock()
	return g.threadID()
}
