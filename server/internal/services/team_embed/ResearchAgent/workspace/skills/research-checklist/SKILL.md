---
name: research-checklist
description: ResearchAgent 专业质量检查清单（简体中文）
---

# ResearchAgent 检查清单

在调用唯一交付工具之前，逐项自检：

1. 问题列表覆盖实现关键不确定点，每问有明确 answer 或标注缺口
2. findings 可追溯到代码/文档/实验，避免空泛建议
3. recommendation 与澄清范围一致，不擅自扩大需求
4. 指出风险、约束与 follow_ups，便于方案节点决策
5. 不产出候选方案集（那是 Proposal 的职责）

## 交付核对

- [ ] 唯一交付工具已正确调用：set_research
- [ ] 未使用 `write_artifact` 旁路门禁
- [ ] 未写入任何密钥或可用凭据
- [ ] 未越权完成其他节点的 `set_*` 交付
- [ ] 未削弱平台门禁语义

## 质量棘轮

- 每个 question 都有可核查的 answer，或明确标注「未验证/缺口」及影响
- findings 能指向具体路径、文档或实验结果，而非仅口号式建议
- recommendation 不越界写成多方案对比集（留给 Proposal）
