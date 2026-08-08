package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/channels"
	"github.com/cocofhu/approving/internal/liveagent"
)

// synthesisFallbackTimeout is used only when the Live client has no configured
// timeout. Production follows live_timeout_seconds via live.Timeout().
const synthesisFallbackTimeout = 45 * time.Second

// synthesisMaxTokens sizes a short report that still has room for key findings
// after a reasoning model spends budget deliberating.
const synthesisMaxTokens = 2048

// synthesisSystemPrompt is the reporting voice. It is the same person the user
// has been talking to all along, which is the whole point of phrasing an
// outcome instead of pushing a template.
const synthesisSystemPrompt = `你是这个项目的负责人本人，正在 IM 上和同事聊天。你的回复会原样发给对方。

下面会给你一件事的结果。用一两段人话讲给对方听：先结论，再带上 brief 里的关键发现。

规矩：
- 只讲给出的事实，不要补充、不要推测、不要展开成长报告。
- brief 里若有「关键发现」，必须写进具体点；禁止只说「弄完了 / 已完成 / 确实有办法 / 可以精简」这种空结论。
- brief 写明没有可读结论时：只承认还没整理出要点，禁止编造实质收获。
- brief 里若有链接或具体交付物，必须写进回复，不要藏着等对方再问。
- 禁止「想看细节的话跟我说」「详情再说」这类把实质内容推到下一轮的空话。
- 不要出现任务编号、工作流名、执行环境、工具名这些内部说法。
- 不要说「请前往 Approving 查看」之类把人打发走的话。
- 若 brief 只是刚重新开跑或还在队列/执行中，用现在进行时（正在重试、刚派下去），禁止「已经重试过了 / 重新重试过了」。
- 只输出要发出去的话，不要加前缀、标题或解释。`

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
		timeout := live.Timeout()
		if timeout <= 0 {
			timeout = synthesisFallbackTimeout
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
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
