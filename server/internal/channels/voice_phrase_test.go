package channels

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/liveagent"
	"github.com/cocofhu/approving/internal/models"
)

// recordingLive captures what one phrasing call looked like on the wire.
type recordingLive struct {
	configured bool
	timeout    time.Duration
	answer     string
	err        error
	got        liveagent.Request
	calls      int
	// timeoutSeen is how long the call was actually given, read off the
	// context deadline the caller set.
	timeoutSeen time.Duration
}

func (r *recordingLive) Configured() bool       { return r.configured }
func (r *recordingLive) Timeout() time.Duration { return r.timeout }
func (r *recordingLive) Complete(ctx context.Context, req liveagent.Request) (liveagent.Result, error) {
	r.calls++
	r.got = req
	if deadline, ok := ctx.Deadline(); ok {
		r.timeoutSeen = time.Until(deadline)
	}
	if r.err != nil {
		return liveagent.Result{}, r.err
	}
	return liveagent.Result{Text: r.answer}, nil
}

// TestAckPhrasingKeepsItsOwnCeiling pins the one thing that had to survive
// merging the two phrasing paths: an ack is capped short no matter how patient
// the endpoint setting is, because the reply the user is waiting for comes
// after it. The conclusion path deliberately does not share that cap.
func TestAckPhrasingKeepsItsOwnCeiling(t *testing.T) {
	live := &recordingLive{configured: true, timeout: 5 * time.Minute, answer: "行，那事我去弄。"}
	m := &Manager{live: live}

	if got := m.phraseThroughLive(context.Background(), dispatchAckPhrasePrompt, "对方说：修一下"); got != "行，那事我去弄。" {
		t.Fatalf("ack = %q", got)
	}
	if live.timeoutSeen > ackPhraseTimeout || live.timeoutSeen < ackPhraseTimeout-time.Second {
		t.Fatalf("ack was given %v; it must stay near the %v ceiling", live.timeoutSeen, ackPhraseTimeout)
	}
	if live.got.MaxTokens != ackPhraseMaxTokens {
		t.Fatalf("ack max tokens = %d want %d", live.got.MaxTokens, ackPhraseMaxTokens)
	}

	// A shorter configured timeout wins over the ceiling — the ceiling is a
	// cap, not a floor.
	live.timeout = 2 * time.Second
	m.phraseThroughLive(context.Background(), dispatchAckPhrasePrompt, "对方说：再看看")
	if live.timeoutSeen > 2*time.Second {
		t.Fatalf("configured %v was ignored: call got %v", live.timeout, live.timeoutSeen)
	}
}

// A conclusion is worth waiting for, so it follows the configured timeout
// rather than the ack ceiling.
func TestConclusionPhrasingFollowsTheConfiguredTimeout(t *testing.T) {
	live := &recordingLive{configured: true, timeout: 90 * time.Second, answer: "跑完了，两个用例挂在超时上。"}
	m := &Manager{live: live}
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "proj"}}

	text, degraded := m.reportThroughDirector(context.Background(), rc,
		InboundMessage{Text: "怎么样了"}, "2 failed: timeout")
	if degraded || text != "跑完了，两个用例挂在超时上。" {
		t.Fatalf("report = %q degraded=%v", text, degraded)
	}
	if live.timeoutSeen <= ackPhraseTimeout {
		t.Fatalf("conclusion was capped at the ack ceiling: %v", live.timeoutSeen)
	}
	if live.got.MaxTokens != directorReportMaxTokens {
		t.Fatalf("conclusion max tokens = %d want %d", live.got.MaxTokens, directorReportMaxTokens)
	}
}

// Everything that can go wrong with the model has to land on the caller's own
// words rather than on an empty message. An empty completion counts as a
// failure: sending it would deliver silence and call it a reply.
func TestPhrasingFailuresFallBackRatherThanGoingSilent(t *testing.T) {
	rc := &runningChannel{cfg: models.ChannelConfig{ProjectID: "proj"}}
	for name, live := range map[string]*recordingLive{
		"unconfigured": {configured: false},
		"endpoint down": {configured: true, timeout: time.Second,
			err: errors.New("connection refused")},
		"empty answer": {configured: true, timeout: time.Second, answer: "   "},
	} {
		m := &Manager{live: live}
		text, degraded := m.reportThroughDirector(context.Background(), rc,
			InboundMessage{Text: "怎么样了"}, "2 failed: timeout")
		if !degraded || text != "2 failed: timeout" {
			t.Fatalf("%s: report = %q degraded=%v, want the caller's own words", name, text, degraded)
		}
		if got := m.phraseThroughLive(context.Background(), dispatchAckPhrasePrompt, "对方说：修一下"); got != "" {
			t.Fatalf("%s: ack = %q, want empty so the caller stays quiet", name, got)
		}
	}

	// An unconfigured model must not even be dialled.
	quiet := &recordingLive{configured: false}
	m := &Manager{live: quiet}
	m.phraseThroughLive(context.Background(), dispatchAckPhrasePrompt, "对方说：修一下")
	if quiet.calls != 0 {
		t.Fatalf("called an unconfigured endpoint %d times", quiet.calls)
	}
	if _, err := m.livePhrase(context.Background(), phraseRequest{
		System: "x", Messages: []liveagent.Message{{Role: "user", Content: "y"}},
		Timeout: time.Second,
	}); !errors.Is(err, errNoLiveModel) {
		t.Fatalf("err = %v want errNoLiveModel", err)
	}
}

// The ack prompts are worth nothing if the user text never reaches the model.
func TestAckPhrasingCarriesTheUserLineAndPrompt(t *testing.T) {
	live := &recordingLive{configured: true, timeout: time.Second, answer: "好。"}
	m := &Manager{live: live}
	m.phraseThroughLive(context.Background(), refineAckPhrasePrompt, "对方说：重点看 Release")

	if live.got.System != refineAckPhrasePrompt {
		t.Fatalf("system prompt was not passed through:\n%s", live.got.System)
	}
	if len(live.got.Messages) != 1 || !strings.Contains(live.got.Messages[0].Content, "重点看 Release") {
		t.Fatalf("user content = %+v", live.got.Messages)
	}
	if len(live.got.Tools) != 0 {
		t.Fatal("phrasing must not offer tools; it has nothing to decide")
	}
}
