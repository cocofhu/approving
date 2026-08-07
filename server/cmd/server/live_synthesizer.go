package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/liveagent"
)

// synthesisTimeout bounds one phrasing call. Nobody is waiting on a live
// conversation for this, but a background event that has been pending for
// twenty seconds is stale, and the structured fallback is already good enough
// to send.
const synthesisTimeout = 12 * time.Second

// synthesisMaxTokens sizes a one-or-two-sentence report plus whatever the model
// thinks first.
const synthesisMaxTokens = 512

// synthesisSystemPrompt is the reporting voice. It is the same person the user
// has been talking to all along, which is the whole point of phrasing an
// outcome instead of pushing a template.
const synthesisSystemPrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

下面会给你一件事的结果。用一到两句话把它讲给对方听，就像同事之间当面说一样。

规矩：
- 只讲给出的事实，不要补充、不要推测、不要展开成报告。
- 不要出现任务编号、工作流名、执行环境、工具名这些内部说法。
- 不要说「请前往 Approving 查看」之类把人打发走的话。
- 结论要能独立看懂，不要只说「已完成」。
- 只输出要发出去的那句话，不要加前缀、标题或解释。`

// newLiveSynthesizer phrases a background event through the conversation model,
// so a result lands as part of the conversation rather than as a notification
// about it.
//
// It used to open a sandbox turn for this. That worked, but it spent a
// container and tens of seconds to reword two sentences the platform already
// had, and it borrowed the agent that might be busy doing real work. The
// conversation model is the layer that talks to users; phrasing is exactly its
// job.
//
// Anything that goes wrong here degrades to the caller's structured fallback,
// which is a complete message in its own right — synthesis improves how an
// outcome reads, it is not what makes it deliverable.
func newLiveSynthesizer(live *liveagent.Client) channels.SynthesisFunc {
	if live == nil {
		return nil
	}
	return func(ctx context.Context, req channels.SynthesisRequest) (string, error) {
		if strings.TrimSpace(req.Brief) == "" {
			return "", errors.New("synthesis brief is empty")
		}
		if !live.Configured() {
			return "", liveagent.ErrNotConfigured
		}
		callCtx, cancel := context.WithTimeout(ctx, synthesisTimeout)
		defer cancel()

		res, err := live.Complete(callCtx, liveagent.Request{
			System:    synthesisSystemPrompt,
			Messages:  []liveagent.Message{{Role: "user", Content: req.Brief}},
			MaxTokens: synthesisMaxTokens,
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(res.Text), nil
	}
}
