package channels

import (
	"regexp"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"
)

// User-visible copy the platform writes itself.
//
// Everything here is a fallback. The conversation model does the talking; these
// lines are what goes out when it is unconfigured, unreachable, or answering
// something it was never asked — see operational_rules.go for the prompts that
// come first. Keeping them in one file is not tidiness: they were spread across
// eight files, which is why half of them only ever spoke Chinese while the
// other half had grown an English branch, and nobody could see the gap.
//
// Two rules for anything added here:
//
//   - Every function takes a language and answers in it. A user who has been
//     writing English all conversation should not get a Chinese apology when
//     the model times out.
//   - No prompts. Instructions for the model live in voice_phrase.go and
//     operational_rules.go; this file is only what the platform says when the
//     model cannot.
//
// Runtime term rewriting stays in copy_guard.go. It scrubs internal words out
// of anything on its way out, including these lines, and duplicating that work
// here would just create a second list to keep in sync.

// busyHintText is the only thing a user hears about queueing, and only when the
// backlog is genuinely full. Live conversations do not narrate their own
// plumbing: there is no per-message "received, working on it" and no queue
// position, because those crowd out the answer without informing anyone.
func busyHintText(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "Still working through the ones before this — give me a moment."
	}
	return "我这边还在处理前面几条，稍等一下。"
}

// turnFailureText maps an internal turn error onto a cause the user can act on.
func turnFailureText(err error, language string) string {
	en := services.NormalizeLanguage(language) == "en"
	if err == nil {
		if en {
			return "That one didn't go through. Send it again?"
		}
		return "这条我没处理成功，你再发一次试试。"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "超时"),
		strings.Contains(msg, "timeout"):
		if en {
			return "This is taking a while — I'll keep at it in the background and come back with it."
		}
		return "这条想得有点久，我先放到后台继续，有结果告诉你。"
	case strings.Contains(msg, "沙箱"), strings.Contains(msg, "sandbox"):
		if en {
			return "My execution environment won't come up right now. Try again shortly; if it keeps failing an admin needs to look."
		}
		return "我的执行环境暂时起不来，稍后再试一次；一直不行就需要管理员看一下。"
	case strings.Contains(msg, "no reply"), strings.Contains(msg, "empty"):
		if en {
			return "I couldn't land on an answer for that one. Put it another way and I'll try again."
		}
		return "这条我没能给出结论。换个说法我再试试。"
	case strings.Contains(msg, "未启用"), strings.Contains(msg, "disabled"):
		if en {
			return "This project doesn't have conversation turned on yet; an admin needs to enable it."
		}
		return "这个项目还没开启对话能力，需要管理员在后台启用。"
	default:
		if en {
			return "That one didn't go through. Say it again, or try asking a different way."
		}
		return "这条我没处理成功。你可以再说一次，或者换个问法。"
	}
}

// runAcceptanceText is the last-resort line when a Run is accepted but the
// conversation layer did not already speak.
//
// It says which task it is whenever the title is short enough to read as a
// name. This line used to drop the title on the grounds that a short_title is
// a truncated requirement rather than a name, and back then it was: they
// arrived as 「快模型和 wo」. Now that they are cut at word boundaries the only
// remaining objection is length, so length is what is actually checked —
// dropping the name outright produced 「好，那事我去弄」 in reply to 「修复下」,
// which names nothing at all.
//
// Do not append "feel free to keep chatting": the user already knows they can
// talk, and saying so reads like a helpdesk script.
func runAcceptanceText(shortTitle, language string) string {
	name := nameableTitle(shortTitle)
	if services.NormalizeLanguage(language) == "en" {
		if name != "" {
			return "Got it, " + name + " — I'll take it and come back when it's done."
		}
		return "Got it, I'll take that one and come back when it's done."
	}
	if name != "" {
		// The comma is doing work: a title can end in either script, and it is
		// the one separator that reads correctly after both.
		return "好，" + name + "，我去弄，完了告诉你。"
	}
	return "好，那事我去弄，完了告诉你。"
}

// placeholderTitles are what the title resolver produces for a run that has no
// name of its own. Saying one of these out loud is no better than the pronoun.
var placeholderTitles = map[string]bool{
	"未命名任务": true, "untitled task": true, "untitled": true,
}

// pauseFallbackText is what goes out when phrasing is unavailable. Like the
// outcome fallback it has to stand on its own, and it must not send the user
// somewhere else to find out what is being asked.
func pauseFallbackText(identity *models.TaskIdentity, pause TaskPause, language string) string {
	en := services.NormalizeLanguage(language) == "en"
	title := services.SanitizeShortTitle(identity.ShortTitle)
	subject := title
	if subject == "" {
		if en {
			subject = "That one"
		} else {
			subject = "刚才那件事"
		}
	} else if en {
		subject = "\"" + title + "\""
	}
	ask := leadingConclusion(pause.Ask, pauseAskRunes)
	if ask != "" {
		if en {
			return subject + " is waiting on you: " + ask + " Tell me how you want to go and I'll carry on."
		}
		return subject + "停下来等你拿主意：" + ask + "你说怎么走，我就接着做。"
	}
	if en {
		return subject + " has stopped and needs your call before it can go further. Tell me how you want to handle it."
	}
	return subject + "做到一半停下了，得你确认才能往下走。你说说想怎么处理。"
}

// outcomeFallbackText is the self-contained version sent when synthesis is
// unavailable. It never tells the user to go look somewhere else, and it must
// carry ResultSummary when present — an empty "弄完了" is what produced the
// hollow IM replies this path exists to avoid.
func outcomeFallbackText(identity *models.TaskIdentity, outcome TaskOutcome, language string) string {
	title := services.SanitizeShortTitle(identity.ShortTitle)
	en := services.NormalizeLanguage(language) == "en"
	// The digest goes to completedOutcomeFallback unscrubbed on purpose: it
	// chooses which sentences to quote by asking whether each one is clean, and
	// a pre-scrubbed digest looks clean everywhere. Outbound scrubbing happens
	// only in sendOutboundResult.
	facts := strings.TrimSpace(outcome.ResultSummary)
	switch strings.ToLower(strings.TrimSpace(outcome.Status)) {
	case "completed":
		return completedOutcomeFallback(title, facts, en)
	case "cancelled":
		if en {
			if title == "" {
				return "That one's been cancelled, so I've stopped work on it. Tell me if you want it picked back up."
			}
			return "\"" + title + "\" has been cancelled, so I've stopped work on it. Tell me if you want it picked back up."
		}
		if title == "" {
			return "刚才那个取消了，我停下了。要重新做的话说一声。"
		}
		return title + "取消了，我停下了。要重新做的话说一声。"
	default:
		reason := humanizeFailureReason(outcome.FailureReason, language)
		if en {
			if title == "" {
				return "That one didn't go through: " + reason + " Want me to retry, change the approach, or leave it for now?"
			}
			return "\"" + title + "\" didn't go through: " + reason + " Want me to retry, change the approach, or leave it for now?"
		}
		if title == "" {
			return "刚才那个没做成：" + reason + "你看是重试、换个做法，还是先搁置？"
		}
		return title + "没做成：" + reason + "你看是重试、换个做法，还是先搁置？"
	}
}

// outcomeFallbackFactsRunes bounds what the degraded path is allowed to quote.
// ResultSummary is written by the working agent for the platform, not for the
// user: a full one is a report with commit hashes, module names and headings.
// Quoting its opening conclusion is useful; pasting the whole thing is how a
// finished task arrived in QQ as 「弄完了。对照…基线（git: 90713d62 Merge #177）…」.
const outcomeFallbackFactsRunes = 160

// deliveryLine matches the link row the platform itself appends to a digest
// (services.AppendRunDeliveryURL). Lifting it out is what lets the link end the
// message instead of trailing a wall of text nobody read that far into.
var deliveryLine = regexp.MustCompile(`(?m)^[^\S\n]*(?:交付链接|Delivery)[：:][^\S\n]*(\S+)[^\S\n]*$`)

// humanizeFailureReason turns an aggregated failure cause into something a user
// can act on. The raw reason is diagnostic text and often mentions internals.
func humanizeFailureReason(reason, language string) string {
	en := services.NormalizeLanguage(language) == "en"
	lower := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case lower == "":
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "超时"),
		strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timed out"):
		if en {
			return "it ran out of time."
		}
		return "跑太久超时了。"
	case strings.Contains(lower, "permission"), strings.Contains(lower, "权限"),
		strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"):
		if en {
			return "it didn't have the access it needed."
		}
		return "权限不够。"
	case strings.Contains(lower, "sandbox"), strings.Contains(lower, "沙箱"):
		if en {
			return "the execution environment couldn't start."
		}
		return "执行环境没起来。"
	case strings.Contains(lower, "network"), strings.Contains(lower, "connection"),
		strings.Contains(lower, "网络"):
		if en {
			return "a network call kept failing."
		}
		return "网络一直连不上。"
	}
	if en {
		return "something went wrong partway through."
	}
	return "中途出了问题。"
}

// FormatCronPush builds the short body for a structured scheduled result.
// Unchanged uses a minimal template; changed/failed keep the (truncated) body.
//
// The language comes from the job itself, because that is the only signal
// there is: a scheduled push has no conversation behind it. The category counts
// as well as the body — an unchanged result has almost no body to read, and the
// category is the part a person named.
func FormatCronPush(category string, kind CronResultKind, body string) string {
	return formatCronPush(category, kind, body, services.DetectLanguage(category+"\n"+body, ""))
}

func formatCronPush(category string, kind CronResultKind, body, language string) string {
	body = strings.TrimSpace(body)
	cat := strings.TrimSpace(category)
	switch kind {
	case CronResultUnchanged:
		return unchangedTemplate(cat, language)
	case CronResultFailed:
		if body == "" {
			return failLabel(cat, language) + failedWord(language)
		}
		return failLabel(cat, language) + truncateRunes(body, 120)
	default:
		if body == "" {
			return catLabel(cat, language) + changedWord(language)
		}
		if lineEnd := strings.IndexAny(body, "\r\n"); lineEnd >= 0 {
			body = strings.TrimSpace(body[:lineEnd])
		}
		return truncateRunes(body, 120)
	}
}

func failedWord(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "failed"
	}
	return "失败"
}

func changedWord(language string) string {
	if services.NormalizeLanguage(language) == "en" {
		return "has changes"
	}
	return "有变化"
}

// FormatProgressText labels one progress event for a chat window.
func FormatProgressText(ev ProgressEvent) string {
	return formatProgressText(ev, services.DetectLanguage(ev.Summary, ""))
}

func formatProgressText(ev ProgressEvent, language string) string {
	sum := strings.TrimSpace(ev.Summary)
	if sum == "" {
		return ""
	}
	en := services.NormalizeLanguage(language) == "en"
	switch ev.Kind {
	case ProgressBlocker:
		if en {
			return "Stuck on this: " + sum
		}
		return "卡住了：" + sum
	case ProgressConfirm:
		if en {
			return "Need your call on this: " + sum
		}
		return "需要你定一下：" + sum
	default:
		// Milestones carry no label. The old "进度：" prefix turned the agent's
		// own words into a machine relaying them, and the content already says
		// it is progress.
		return sum
	}
}
