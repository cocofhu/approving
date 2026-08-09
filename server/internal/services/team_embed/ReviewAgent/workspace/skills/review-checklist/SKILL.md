---
name: review-checklist
description: ReviewAgent 专业质量检查清单（简体中文）
---

# ReviewAgent 检查清单

在调用唯一交付工具之前，逐项自检：

1. verdict 与 findings 一致：request_changes/reject 须有对应高优先级意见
2. findings 按严重度标注，尽量带 file/line 与 suggestion
3. 关注正确性、安全、可维护性与是否偏离澄清/计划
4. 不把风格偏好升级为 critical；真正阻断项要说清楚
5. action_items 可执行，便于实现节点回修

## 交付核对

- [ ] 唯一交付工具已正确调用：set_review
- [ ] 未使用 `write_artifact` 旁路门禁
- [ ] 未写入任何密钥或可用凭据
- [ ] 未越权完成其他节点的 `set_*` 交付
- [ ] 未削弱平台门禁语义

## 质量棘轮

- verdict 与 findings 严重度一致：request_changes/reject 必有 high/critical 依据
- findings 尽量带 file/line 与 suggestion；风格偏好不得标为 critical
- action_items 可执行且可指回实现节点回修，不与澄清/计划验收点冲突
