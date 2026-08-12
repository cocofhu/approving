package dingtalk

import "errors"

// ErrUnspoken is returned for proactive OpenAPI pushes when the conversation
// has never spoken to the bot (aligns with WeCom HasSpoken precheck).
var ErrUnspoken = errors.New("未发言：该会话尚未向机器人发送过消息")
