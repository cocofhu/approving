## Approving v0.3.8-beta

相对 [`v0.3.7-beta`](https://github.com/cocofhu/approving/releases/tag/v0.3.7-beta)：PRs #531–#567；默认 GHCR pin 为 `*:0.3.8-beta`。

本地启动：`./start.sh -d`（拉取发布镜像）。

---

## 功能（Features）

### Token 分析 / 统计
- 全局 Token 分析柱状图支持三维度切换：项目 / 工作流 / 模型；维度对比堆叠展示（#531–#534）
- 柱状图部分值独立显示（Input/Output/Cache 各部分）（#535）
- 工作流 / 模型排名统计补全缺失的 token parts（#536）

### 首页
- 新增「新建工作流」Plus 卡片，基于 baseline 创建（#540）
- 基线工作流创建向导：项目多选 → 名称 + 仓库配置（#541–#545）
- 项目列表加载失败重试支持（#542）

### 平台状态指标 / StatusMetrics
- 四个指标按钮（Token / 今日 / 执行中 / 排队）改为导航至统计页，而非 pin tip（#547–#550）
- 紧凑模式指标 hover 显示 tip，click 导航到统计（#548）
- 支持 Ctrl/Cmd + 数字快捷键或 Enter/Space 激活（#549）

### 公开审批链接 / PublicGateApprovalView
- 审批界面语言切换器（顶右角），选择持久化到 localStorage（#551–#553）
- 工作流 Composer 重构：音并入底部导航栏（#554）
- 澄清/复审分离：Clarify 场景保留确认、Review 场景仅含输入（#555–#558）
- 上游上下文栏从页脚挪至 Stage 下方（#559）
- 冷启动审批场景（历史回合，禁止修改）支持单按钮确认（#560）

### 运行详情 / Run Detail
- 澄清/复审会话繁忙时，自动 patch chrome（status/progress/artifacts），保留对话流（g1.1 / g1.2）（#561–#563）
- 后台 focus 刷新改为静默 chrome patch，避免抖动（#564）
- 工件指纹不变时跳过重新加载，保持 iframe 挂载（#565）

### 启动引导 / ReactConnectingState
- 四步骤加载指示器（准备环境 → 启动 Agent → 连接 → 整理问题）（#566）
- 尊重 `prefers-reduced-motion`，禁用多步循环动画（#567）

### UI / 样式
- Token 统计配色优化（更高对比度）（#537）
- 工具栏操作统一为单行，减少折行（#534）
- 减少运动偏好时禁用所有转场动画（#567）

---

## 修复（Fixes）

- Locale 检测增加 localStorage 缓存查询，保留用户选择（#552）
- Token 分析 top10+other 聚合逻辑修正，input/output/cache 部分求和与 total 对齐（#531）
- 工作流排名 top10 分桶后处理，other 行补全所有维度数据（#536）
- 运行详情忙碌态下，reactSessions 和 clarify 对话缓冲不再被 REST 快照覆盖（g1.1）（#562）
- E2E 测试 testid 统一，PublicGate 确认流程指标更新（#555–#560）
- BootLoader 减速运动时禁用周期性状态更新（#567）

---

## 基础设施

- web LIB_DOMAIN_MAP 新增 mergeRunChrome.ts 导出（#563）
- E2E 测试覆盖 token 分析柱状图维度切换、localStorage locale 持久化（#541–#553）
- 运行详情 WebSocket 协议改为静默软刷新，never hard load（#564）
- Pin 默认 GHCR 到 `0.3.8-beta`

镜像由 `v*` tag 触发 `publish-image` / `publish-gateway` / `publish-sandbox` 构建；沙箱矩阵通常 30–90+ 分钟。三件套全绿前勿宣称 sandbox 版可用。
