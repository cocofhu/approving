// Package correl 生成短随机 ID，用于把多层级日志串成一条「业务链路」。
//
// 约定字段（与日志中键名一致，便于 grep）：
//   - cid：一条 WebSocket 连接，从握手到断开
//   - oid：一次用户 chat 入队到对应 session/prompt 执行结束（可与 cid 同时出现）
//   - sid：ACP sessionId，由 Agent 在 session/new 后确定，stdio JSON-RPC 侧日志统一带上
//
// 排查示例：先看到 ws 报错里的 cid/oid，再在同一时间段搜 sid= 与 acp: 行。
package correl

import (
	"crypto/rand"
	"encoding/hex"
)

// ID 返回 8 个十六进制字符（4 字节随机），碰撞概率足够用于日志关联。
func ID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}
