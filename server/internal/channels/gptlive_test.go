package channels

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"gorm.io/gorm"
)

// fakeLive stands in for a conversation-model endpoint. Each inbound message
// consumes one scripted decision; running out means the script and the
// conversation disagree, which is a test failure worth seeing.
type fakeLive struct {
	configured bool
	decisions  []liveagent.Result
	err        error
	seen       [][]liveagent.Message
}

func (f *fakeLive) Configured() bool { return f.configured }

func (f *fakeLive) Complete(_ context.Context, req liveagent.Request) (liveagent.Result, error) {
	f.seen = append(f.seen, req.Messages)
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

func liveEscalate(request string) liveagent.Result {
	return liveagent.Result{ToolName: askProjectAgentTool, Args: map[string]string{"request": request}}
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
	g.m.handleInbound(context.Background(), g.rc, InboundMessage{
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

// Anything that needs the repository is handed over, and the handover carries
// the request plus the attachments the user sent with it.
func TestLiveModelEscalationCarriesTextAndAttachments(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{
		configured: true,
		decisions:  []liveagent.Result{liveEscalate("按这张截图修登录页")},
	})

	g.say("m1", "按这张图修一下", Image{
		Data: []byte("PNGDATA"), MimeType: "image/png", Filename: "shot.png",
	})

	if len(g.agent) != 1 {
		t.Fatalf("escalation did not reach the agent: %v", g.agent)
	}
	got := g.agent[0]
	if got.Text != "按这张图修一下" {
		t.Fatalf("agent saw text %q", got.Text)
	}
	if len(got.Images) != 1 || got.Images[0].Filename != "shot.png" {
		t.Fatalf("attachment did not survive routing: %+v", got.Images)
	}
	if !strings.Contains(got.EscalationReason, "登录页") {
		t.Fatalf("escalation reason = %q, the agent cannot tell why it was called", got.EscalationReason)
	}
	// The bytes are stored too, so a later turn can replay them.
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

// A model that fails must cost latency, not the reply.
func TestConversationModelFailureStillAnswersTheUser(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, err: liveagent.ErrBudgetExhausted})

	g.say("m1", "什么进度了")

	if len(g.agent) != 1 {
		t.Fatalf("a failed model call swallowed the message: %v", g.agent)
	}
	if got := sentTexts(g.fa); len(got) != 1 || got[0] != "agent-answer" {
		t.Fatalf("sends = %v want the agent's answer", got)
	}
}

// The two layers must see one conversation. After the model answers twice on
// its own, the turn it hands over has to arrive with those exchanges attached —
// otherwise the agent starts from a question with no context and asks the user
// to repeat what they just said.
func TestEscalatedTurnCarriesWhatTheOuterLayerAnswered(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("我在。"),
		liveAnswer("登录页那条我记得，你说的是首屏。"),
		liveEscalate("看仓库确认首屏改动"),
	}})

	g.say("m1", "在吗")
	g.say("m2", "登录页那个还记得吗")
	g.say("m3", "去仓库确认一下改了没")

	if len(g.agent) != 1 {
		t.Fatalf("agent turns = %d want exactly the escalated one", len(g.agent))
	}
	// One record, both layers: two turns the model answered, then the escalated
	// turn and the agent's reply to it.
	want := []string{
		"user: 在吗",
		"assistant: 我在。",
		"user: 登录页那个还记得吗",
		"assistant: 登录页那条我记得，你说的是首屏。",
		"user: 去仓库确认一下改了没",
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

	// The handoff the sandbox would receive names both earlier turns and the
	// replies the user actually got.
	thread, err := g.pm.GetThreadByID(g.threadID())
	if err != nil {
		t.Fatal(err)
	}
	current, err := g.pm.GetMessage(thread.ID, g.agent[0].RecordedMessageID)
	if err != nil {
		t.Fatalf("escalated turn has no transcript row: %v", err)
	}
	payload := g.bridge.buildHandoff(thread, current, g.agent[0], false)
	for _, must := range []string{"在吗", "我在。", "登录页那个还记得吗", "你说的是首屏"} {
		if !strings.Contains(payload.text, must) {
			t.Fatalf("handoff lost %q:\n%s", must, payload.text)
		}
	}
	if strings.Contains(payload.text, "去仓库确认一下改了没") {
		t.Fatalf("handoff repeated the current request as history:\n%s", payload.text)
	}
	if !strings.Contains(payload.text, "看仓库确认首屏改动") {
		t.Fatalf("handoff does not say why the turn arrived:\n%s", payload.text)
	}
}

// A picture sent one turn and referred to the next must still reach the layer
// that can open it.
func TestAttachmentFromAnEarlierTurnIsReplayedOnEscalation(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("收到，我看看。"),
		liveEscalate("按上一条的图改"),
	}})

	g.say("m1", "这是报错截图", Image{
		Data: []byte("PNGBYTES"), MimeType: "image/png", Filename: "err.png",
	})
	g.say("m2", "按刚才那张图修")

	thread, err := g.pm.GetThreadByID(g.threadID())
	if err != nil {
		t.Fatal(err)
	}
	current, err := g.pm.GetMessage(thread.ID, g.agent[0].RecordedMessageID)
	if err != nil {
		t.Fatal(err)
	}
	payload := g.bridge.buildHandoff(thread, current, g.agent[0], false)
	if len(payload.images) != 1 {
		t.Fatalf("history attachment was not replayed: %+v", payload.images)
	}
	if payload.images[0].Name != "err.png" || payload.images[0].MimeType != "image/png" {
		t.Fatalf("replayed attachment lost its identity: %+v", payload.images[0])
	}
	if payload.images[0].Data == "" {
		t.Fatal("replayed attachment carried no bytes")
	}
}

// An attachment too large to replay must be named, not dropped: the agent can
// fetch it, but only if it knows it exists. The current message's own
// attachments are never the ones sacrificed.
func TestOversizeHistoryAttachmentIsListedRatherThanDropped(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("收到。"),
		liveEscalate("接着上一张图"),
	}})

	huge := make([]byte, handoffAttachmentBudget)
	g.say("m1", "大文件", Image{Data: huge, MimeType: "application/pdf", Filename: "big.pdf"})
	g.say("m2", "按上面那份改", Image{
		Data: []byte("SMALL"), MimeType: "image/png", Filename: "now.png",
	})

	thread, err := g.pm.GetThreadByID(g.threadID())
	if err != nil {
		t.Fatal(err)
	}
	current, err := g.pm.GetMessage(thread.ID, g.agent[0].RecordedMessageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Images) != 1 || current.Images[0].Name != "now.png" {
		t.Fatalf("the current message's attachment was not kept intact: %+v", current.Images)
	}
	payload := g.bridge.buildHandoff(thread, current, g.agent[0], false)
	for _, img := range payload.images {
		if img.Name == "big.pdf" {
			t.Fatal("an attachment over budget was replayed anyway")
		}
	}
	if !strings.Contains(payload.text, "big.pdf") || !strings.Contains(payload.text, "get_attachment") {
		t.Fatalf("oversize attachment vanished silently:\n%s", payload.text)
	}
}

// A warm sandbox is caught up with what it missed; a fresh one, which
// remembers nothing, gets the bounded baseline instead of an empty delta.
func TestHandoffIsIncrementalForAWarmSandboxAndABaselineForAFreshOne(t *testing.T) {
	g := newGPTLive(t)
	g.m.SetLiveModel(&fakeLive{configured: true, decisions: []liveagent.Result{
		liveAnswer("早"), liveAnswer("嗯"), liveEscalate("看仓库"),
	}})
	g.say("m1", "早上好")
	g.say("m2", "在忙吗")
	g.say("m3", "去看下仓库")

	thread, err := g.pm.GetThreadByID(g.threadID())
	if err != nil {
		t.Fatal(err)
	}
	current, err := g.pm.GetMessage(thread.ID, g.agent[0].RecordedMessageID)
	if err != nil {
		t.Fatal(err)
	}

	fresh := g.bridge.buildHandoff(thread, current, g.agent[0], false)
	if !strings.Contains(fresh.text, "早上好") {
		t.Fatalf("a fresh sandbox got no baseline:\n%s", fresh.text)
	}

	// Pretend the sandbox already handled everything up to the second turn.
	msgs, err := g.pm.CanonicalWindow(thread.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.pm.SetHandoffCursor(thread.ID, msgs[3].ID); err != nil {
		t.Fatal(err)
	}
	thread.HandoffCursor = msgs[3].ID
	warm := g.bridge.buildHandoff(thread, current, g.agent[0], true)
	if strings.Contains(warm.text, "早上好") {
		t.Fatalf("a warm sandbox was shown history it already lived through:\n%s", warm.text)
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
