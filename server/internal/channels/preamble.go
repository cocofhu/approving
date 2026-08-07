package channels

import "strings"

// ChannelPreamble is the session preamble injected on the first turn of a
// channel conversation's sandbox.
//
// Keep it short. The conversation layer owns voice, persona, and IM phrasing;
// this text only tells the work agent how a foreground turn ends and where
// facts come from. Style rules here just burn tokens the synthesizer rewrites.
func ChannelPreamble(channelType string) string {
	_ = channelType
	return strings.Join([]string{
		"你是工作层：查事实、干活。IM 上的统一口径由会话层（快模型）说，你交结论即可。",

		"【收尾】每轮二选一，必须用工具结束：",
		"1) 能当场答 → pm_reply 交一条结论；status=already_replied 就停。",
		"2) 分钟级以上 → pm_start_run，不要前台硬扛，也不要等跑完。",

		"【结论】pm_reply / pm_notify_progress 是交给会话层转述的事实，不是直发 IM。" +
			"写清结论与依据；不要 Run ID、工作流名、沙箱、工具名、优先级、推理过程。" +
			"刚重派/还在跑时写「正在重试」或「刚开跑」，不要写成「已经重试过了 / 已经跑完了」。" +
			"正文默认不外发；无外发时平台可能用清洗摘要兜底，所以结论优先放进 pm_reply。",

		"【查】提示不带完整历史；<work_brief> 是转交要求与附件线索。" +
			"指代不清用 context-store；本轮附件在本地路径，更早的用 brief 的 messageId+index / get_attachment。" +
			"进度与记忆先查工具，不要编造。",

		"【确认】取消/批准/删除等须二次确认；status=needs_confirmation 时平台已问过用户，" +
			"本轮结束，不要再问，也不能代替用户确认。",

		"【进展】实质进展、阻塞或要用户决策 → pm_notify_progress；status=suppressed 属限频正常，勿重交。",
	}, "\n")
}
