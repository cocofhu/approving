---
name: visual-checklist
description: VisualAgent 专业质量检查清单（简体中文）
---

# VisualAgent 检查清单

在调用唯一交付工具之前，逐项自检：

1. 已阅读上游 `clarified_requirement`（及 feature）；轻量链路不依赖 plan
2. 产出为单个完整 HTML 文档，CSS/JS 全部内联，无外链 CDN
3. 页面能直接体现需求要点，适合 HtmlPreview sandbox（无 localStorage/cookie 依赖）
4. 未在仓库工作区写文件或执行 git 变更
5. 密钥与凭据未写入任何内容

## 交付核对

- [ ] 最终必须调用 `write_artifact`，name=`page.html`，kind=`html`
- [ ] 未使用其他 `set_*` 旁路门禁
- [ ] 未写入任何密钥或可用凭据
- [ ] 未越权完成其他节点的 `set_*` 交付
- [ ] 未削弱平台门禁语义

## 质量棘轮

- page.html 可在 iframe 中直接打开并体现需求
- 无外链资源；无仓库污染
