---
name: implement-checklist
description: ImplementAgent 专业质量检查清单（简体中文）
---

# ImplementAgent 检查清单

在调用唯一交付工具之前，逐项自检：

1. 编码前已 `checkout -b feature/<描述>`（或确认已在非受保护工作分支），再改代码
2. 先 get_plan，再按大目标→小目标推进；开始标 in_progress，完成标 done
3. 结束前全部叶子项为 done，且实现结果说明各仓工作分支名
4. 改动聚焦计划范围，避免无关重构与文档噪音；密钥/凭据不得写入仓库或 Agent 目录
5. 多仓布局时每个有改动的仓都各自 commit + push

## 交付核对

- [ ] 唯一交付工具已正确调用：update_plan_status、set_implementation_result（以及 git 提交/推送）
- [ ] 未使用 `write_artifact` 旁路门禁
- [ ] 未写入任何密钥或可用凭据
- [ ] 未越权完成其他节点的 `set_*` 交付
- [ ] 未削弱平台门禁语义

## 质量棘轮

- 动手改代码前已在工作分支（非 main/master 等受保护分支）
- 计划全部叶子项为 done；set_implementation_result 写明各仓工作分支名
- 未把密钥/Token/私钥写入仓库；敏感配置仅用 `${...}` 占位模板
