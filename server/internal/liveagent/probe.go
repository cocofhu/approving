package liveagent

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cocofhu/approving/internal/textutil"
)

// ProbeCheck is one property of an endpoint that either holds or does not.
// Reason is written for whoever is filling in the settings form, so it names
// what to go and change rather than what the transport reported.
type ProbeCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// Probe check names. They are stable identifiers the UI translates; the Reason
// text is already human-readable and is shown as-is.
const (
	// CheckReachable is a real completion round trip. It is the one check that
	// gates the others: nothing else is worth asking if the endpoint is silent.
	CheckReachable = "reachable"
	// CheckToolCalls is whether the model can return a tool call. The routing
	// layer decides to escalate by calling a tool, so a model that cannot do
	// this answers everything itself and never hands work to the project — a
	// failure that looks like normal operation from the outside, which is
	// exactly why it is worth a check of its own.
	CheckToolCalls = "tool_calls"
)

// ProbeReport is the outcome of testing one endpoint.
type ProbeReport struct {
	Configured bool         `json:"configured"`
	OK         bool         `json:"ok"`
	Checks     []ProbeCheck `json:"checks"`
	// LatencyMS is the round trip of the reachability call. This layer is only
	// worth having while it is fast, so the number is the point of the test as
	// much as the pass/fail is: it is what a sensible timeout is chosen from.
	LatencyMS int64 `json:"latencyMs"`
	// Sample is a short excerpt of what the model actually said, so an operator
	// can confirm it is the model they think they configured.
	Sample string `json:"sample,omitempty"`
}

// probeTool is deliberately trivial. The check is whether the endpoint supports
// tool calling at all, not whether the model is any good at choosing.
var probeTool = ToolSpec{
	Name:        "report_ready",
	Description: "Report that you are ready. Call this tool instead of writing a reply.",
	Params: []Param{{
		Name: "status", Description: `Always the word "ready".`,
		Enum: []string{"ready"}, Required: true,
	}},
}

// Probe tests one endpoint without disturbing the configured one.
//
// It runs against a throwaway client so an operator can check a form they have
// not saved yet, and so a test never counts as traffic in the runtime stats.
func Probe(ctx context.Context, ep Endpoint) ProbeReport {
	report := ProbeReport{Configured: ep.Configured()}
	if !report.Configured {
		report.Checks = []ProbeCheck{{
			Name: CheckReachable, OK: false,
			Reason: "接口地址和模型名称都填了才能测试。",
		}}
		return report
	}

	c := New()
	c.cur.store(ep)
	timeout := c.cur.load().Timeout

	started := time.Now()
	res, err := c.Complete(ctx, Request{
		System:    "You are a health check. Reply with the single word: ok",
		Messages:  []Message{{Role: "user", Content: "ok?"}},
		MaxTokens: 512,
	})
	report.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		report.Checks = []ProbeCheck{{
			Name: CheckReachable, OK: false, Reason: describeFailure(err, timeout),
		}}
		return report
	}
	report.Checks = append(report.Checks, ProbeCheck{Name: CheckReachable, OK: true})
	report.Sample = excerpt(res.Text, 120)

	// Only worth asking once we know the endpoint answers, and skipping it on
	// failure also keeps a broken endpoint from costing two full timeouts.
	tool, err := c.Complete(ctx, Request{
		System: "You are being checked for tool-calling support. " +
			"Call the report_ready tool with status=ready. Do not write a reply.",
		Messages:  []Message{{Role: "user", Content: "Are you ready?"}},
		Tools:     []ToolSpec{probeTool},
		MaxTokens: 512,
	})
	switch {
	case err != nil:
		report.Checks = append(report.Checks, ProbeCheck{
			Name: CheckToolCalls, OK: false, Reason: describeFailure(err, timeout),
		})
	case tool.ToolName != probeTool.Name:
		report.Checks = append(report.Checks, ProbeCheck{
			Name: CheckToolCalls, OK: false,
			Reason: "这个模型没有发出工具调用，只回了文字。它可以直接答话，" +
				"但不会把需要动手的活儿转交给项目——建议换一个支持 function calling 的模型。",
		})
	default:
		report.Checks = append(report.Checks, ProbeCheck{Name: CheckToolCalls, OK: true})
	}

	report.OK = true
	for _, ch := range report.Checks {
		if !ch.OK {
			report.OK = false
			break
		}
	}
	return report
}

// describeFailure turns a call error into something worth acting on.
//
// It never carries the response body: that can echo request fields, including
// the conversation, and none of it is safe to put on a settings page. What it
// does carry is which part of the form is likely wrong.
func describeFailure(err error, timeout time.Duration) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotConfigured):
		return "接口地址和模型名称都填了才能测试。"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "等了 " + strconv.Itoa(int(timeout.Seconds())) + " 秒没有响应。" +
			"端点可能太慢或卡住了；这一层只有快才有意义，慢的话不如直接交给沙箱。"
	case errors.Is(err, ErrBudgetExhausted):
		return "模型把额度都花在思考上，没有产出回答。换一个不那么爱推理的模型会更合适。"
	case errors.Is(err, ErrEmptyResponse):
		return "端点接受了请求但没有返回内容。"
	case errors.Is(err, ErrBadResponse):
		return "端点返回的内容看不懂，可能不是 OpenAI 兼容接口。检查地址是不是少了 /v1。"
	}

	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return describeStatus(statusErr.StatusCode)
	}

	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr):
		return "域名解析不了，检查接口地址里的主机名。"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "连接被拒绝，检查地址和端口，以及端点是不是在跑。"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "网络到不了这个地址，检查它是否在本机能访问的网段里。"
	}
	return "连不上这个端点，检查接口地址。"
}

// describeStatus maps a rejection to the field that most likely caused it.
func describeStatus(code int) string {
	switch {
	case code == 401 || code == 403:
		return "端点拒绝了密钥（HTTP " + strconv.Itoa(code) + "）。检查密钥，本机内网模型通常留空即可。"
	case code == 404:
		return "地址不对（HTTP 404）。OpenAI 兼容地址一般以 /v1 结尾，" +
			"系统会自己拼上 /chat/completions。也可能是模型名称不存在。"
	case code == 400 || code == 422:
		return "请求被拒绝（HTTP " + strconv.Itoa(code) + "）。最常见的原因是模型名称写错了。"
	case code == 429:
		return "被限流了（HTTP 429）。稍后再试，或换一个额度更宽的端点。"
	case code >= 500:
		return "端点自己出错了（HTTP " + strconv.Itoa(code) + "）。这是端点侧的问题，不是这里的配置。"
	}
	return "端点返回了 HTTP " + strconv.Itoa(code) + "。"
}

func excerpt(s string, max int) string {
	return textutil.SoftTruncateRunes(strings.Join(strings.Fields(s), " "), max)
}
