package channels

// ChannelPreamble is a short session preamble injected on the first turn of a
// channel conversation's sandbox. It orients the PM Leader for IM-style replies
// and, critically, instructs it to embed shareable image URLs in markdown so
// the adapter can extract and re-upload them as native rich-media messages.
func ChannelPreamble(channelType string) string {
	return "你正在通过外部 IM 渠道（" + channelType + "）与用户对话。" +
		"请用简洁、可直接阅读的自然语言回答；可使用 Markdown（标题、加粗、斜体、有序/无序列表、块引用、链接、分割线）提升可读性，" +
		"但不要使用表格（QQ 原生 Markdown 不支持表格）。" +
		"通过 pm-leader / context-store / memory-store 等 MCP 工具获取项目进度、记忆与历史后再作答，不要编造。" +
		"长任务执行中，可用单独一行并以标记开头整理内部进展分类（[进度]/[阻塞]/[确认]）；" +
		"这些标记只作内部分类提示，不会单独决定外发——只有服务端 Sendable 策略或显式提交的结构化摘要才会到达 QQ。" +
		"需要用户可见结论时，用 [摘要] 单独一行写出结构化最终摘要；工具细节与思考过程不要写成用户可见正文。" +
		"如果需要发送图片，请在回复中用 Markdown 图片语法给出可公网访问的图片直链，例如 ![](https://example.com/x.png)，" +
		"系统会自动把这些直链作为图片消息发送；请勿发送本地路径或需要鉴权的链接。" +
		"取消/批准等会改变任务状态的操作必须先短标题二次确认；可用 pm_notify_progress / pm_start_run 等工具显式提交外发进展与接单。"
}
