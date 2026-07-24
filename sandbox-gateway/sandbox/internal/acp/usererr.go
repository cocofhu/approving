package acp

import (
	"context"
	"errors"
	"strings"
	"syscall"
)

// UserFacingAgentErr 将子进程 stdio 上的底层错误转成界面可读的说明（如 broken pipe）。
func UserFacingAgentErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	if errors.Is(err, syscall.EPIPE) || strings.Contains(low, "broken pipe") || strings.Contains(msg, "write |1:") {
		return "Agent 子进程已退出或无法再接收指令（管道已断）。请在本机终端查看「agent stderr」；确认 Agent 可正常启动并已完成鉴权，然后侧栏重新连接。"
	}
	if strings.Contains(low, "connection closed") {
		return "与 Agent 的通信已结束（stdout 已关闭），请重新连接。"
	}
	return msg
}

// UserFacingAny 握手、session/prompt 等路径的统一错误文案（优先覆盖常见 I/O 与超时）。
func UserFacingAny(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "等待超时：请确认 Agent 可正常启动并已完成鉴权，然后重试。"
	}
	if errors.Is(err, context.Canceled) {
		return "操作已取消（可能正在重新连接或会话已结束）。"
	}
	msg := err.Error()
	if hint := UserFacingAgentErr(err); hint != msg {
		return hint
	}
	return msg
}
