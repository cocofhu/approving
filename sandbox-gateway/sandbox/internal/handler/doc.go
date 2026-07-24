// Package handler 实现 HTTP / WebSocket 适配层：解析入参、调用 service.Bridge、写响应。
// 业务规则（排队、广播、权限）均在 service；本包不持有队列或 Panel 生命周期逻辑。
package handler
