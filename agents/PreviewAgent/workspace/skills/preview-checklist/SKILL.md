---
name: preview-checklist
description: PreviewAgent 专业质量检查清单（简体中文）
---

# PreviewAgent 检查清单

在调用唯一交付工具之前，逐项自检：

1. 已在工作区定位到 clone 的示例仓（默认 `demo` / heroku nodejs-getting-started）
2. 依赖已安装；服务以 `PORT=5006`（或文档约定端口）后台启动
3. 监听地址为 `0.0.0.0`，不是仅 `127.0.0.1`
4. 已调用 `set_preview(5006)`（或实际端口）且端口可达
5. 未使用 Docker；未写入密钥

## 交付核对

- [ ] 最终必须调用 `set_preview`
- [ ] 未使用 `set_test_result` / `write_artifact` 旁路门禁
- [ ] 未写入任何密钥或可用凭据
- [ ] 未越权完成其他节点的 `set_*` 交付
- [ ] 未削弱平台门禁语义

## 质量棘轮

- 预览端口登记成功且可达
- 启动说明与 Heroku Getting Started / `PORT=5006` 约定一致，不依赖自建 demo 仓
