package channels

import (
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

const (
	// QQReplyCapability is fixed: QQ gives us no usable quote/reply reference,
	// so task addressing must degrade to natural language selection.
	QQReplyCapability = "unsupported"
	// ReplyCapabilitySupported marks channels that do expose reply references.
	ReplyCapabilitySupported = "supported"
)

// ReplyCapability reports whether a channel type exposes a usable reply/quote
// reference for binding a message to a task.
func ReplyCapability(channelType string) string {
	switch strings.TrimSpace(strings.ToLower(channelType)) {
	case models.ChannelTypeQQ, "":
		return QQReplyCapability
	default:
		return ReplyCapabilitySupported
	}
}

// SupportsReplyReference is the boolean form of ReplyCapability.
func SupportsReplyReference(channelType string) bool {
	return ReplyCapability(channelType) == ReplyCapabilitySupported
}

// QQReplyFallback explains that native quote/reply is unavailable and asks the
// user to select by number or short title.
func QQReplyFallback(currentMessage, recentLanguage string) string {
	if services.DetectLanguage(currentMessage, recentLanguage) == "en" {
		return "QQ reply references are unavailable here. Please choose a task by number or short title."
	}
	return "当前 QQ 会话不支持引用回复，请按序号或短标题选择任务。"
}

// FormatTaskMessage names the task a message is about. Use it only when the
// name carries information — several tasks are live, or the user named this one
// — because a reference attached to every line reads like a ticket queue.
func FormatTaskMessage(shortTitle, kind, body, currentMessage, recentLanguage string) string {
	language := services.DetectLanguage(currentMessage, recentLanguage)
	body = strings.TrimSpace(body)
	if body == "" {
		return services.TaskStatusSentence(shortTitle, kind, language)
	}
	prefix := services.FormatTaskType(shortTitle, kind, language)
	if prefix == "" {
		return body
	}
	return prefix + body
}
