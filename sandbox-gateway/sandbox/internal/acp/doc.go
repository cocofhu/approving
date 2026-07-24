// Package acp 实现与 Agent（ACP JSON-RPC over stdio）的进程间通信：Conn、Panel、终端与部分内建 handler。
// 不包含浏览器 WebSocket、不包含用户消息排队；排队与广播在 service 包。
package acp
