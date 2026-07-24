package models

import "strings"

// AgentPrompts holds a per-Agent override of the platform-injected prompt text
// and sandbox rule files. It is persisted in the Agent's agent.json under the
// "prompts" key and read back by the runtime when a workflow node references
// that Agent (via skill_profile). Every field is optional: an empty field falls
// back to the built-in default (the Default* constants below), so an Agent that
// omits "prompts" behaves exactly like before this became configurable.
//
// The templated fields (ProducesContract, ProducesRetry) use a `{name}`
// placeholder for the declared produces file name, substituted at use time.
// A named placeholder (not fmt %s) keeps a misconfigured value from breaking
// formatting.
type AgentPrompts struct {
	// UpstreamArtifactsHeader is prepended before the list of upstream produces
	// files seeded into the agent prompt. Emitted only when upstream artifacts
	// exist.
	UpstreamArtifactsHeader string `json:"upstreamArtifactsHeader,omitempty"`
	// ProducesContract is the mandatory-produces clause appended when the node
	// declares `produces`. Supports the `{name}` placeholder.
	ProducesContract string `json:"producesContract,omitempty"`
	// ReactOpenSuffix is appended to the opening turn of a ReAct clarify node.
	ReactOpenSuffix string `json:"reactOpenSuffix,omitempty"`
	// ProducesRetry is the re-prompt sent when a react node finished without
	// writing its declared produces. Supports the `{name}` placeholder.
	ProducesRetry string `json:"producesRetry,omitempty"`
	// PlanContract is appended to a plan node's prompt: the plan node's sole
	// deliverable is calling set_plan.
	PlanContract string `json:"planContract,omitempty"`
	// ImplementContract is appended to an implement node's prompt: read the plan
	// and mark per-item progress via update_plan_status.
	ImplementContract string `json:"implementContract,omitempty"`
	// PlanIncompleteRetry is the re-prompt sent to an implement node that
	// finished with plan items still not done. Supports the `{items}`
	// placeholder for the incomplete-item list.
	PlanIncompleteRetry string `json:"planIncompleteRetry,omitempty"`
	// ClarifiedRequirementContract is appended to a react (clarify) node: its
	// deliverable is calling set_clarified_requirement.
	ClarifiedRequirementContract string `json:"clarifiedRequirementContract,omitempty"`
	// ClarifiedOpenQuestionsRetry is the re-prompt sent to a react node that
	// tried to finish while unresolved open_questions remain: it must raise them
	// as ask_question so the user resolves every one. Supports the `{items}`
	// placeholder for the unresolved-question list.
	ClarifiedOpenQuestionsRetry string `json:"clarifiedOpenQuestionsRetry,omitempty"`
	// ImplementResultContract is appended to an implement node: after finishing
	// the plan it must summarize the work via set_implementation_result.
	ImplementResultContract string `json:"implementResultContract,omitempty"`
	// ResearchContract / TestContract / ReviewContract / ProposalContract are
	// appended to the respective framework-card nodes; each names the sole
	// structured deliverable that node's set_* tool writes.
	ResearchContract string `json:"researchContract,omitempty"`
	TestContract     string `json:"testContract,omitempty"`
	ReviewContract   string `json:"reviewContract,omitempty"`
	ProposalContract string `json:"proposalContract,omitempty"`
	// MRContract is appended to a submit_mr node: resolve conflicts against the
	// target branch, push the source branch, and open a merge request. Supports
	// the `{source}` and `{target}` branch placeholders.
	MRContract string `json:"mrContract,omitempty"`
	// VisualContract is appended to a visual node: its sole deliverable is a
	// single self-contained HTML page (inline CSS/JS, no external resources).
	VisualContract string `json:"visualContract,omitempty"`
	// PreviewContract is appended to an app_preview node: build/start the app
	// and register preview ports via set_preview.
	PreviewContract string `json:"previewContract,omitempty"`
	// PreviewRetry is the re-prompt when app_preview finished without set_preview.
	PreviewRetry string `json:"previewRetry,omitempty"`
	// StructuredRetry is the generic re-prompt sent when a framework node
	// finished without writing its reserved structured product. Supports the
	// `{name}` (artifact) and `{tool}` (set_* tool) placeholders.
	StructuredRetry string `json:"structuredRetry,omitempty"`
	// OutcomeContract is appended to every Agent-class node: must call
	// node_complete before finishing.
	OutcomeContract string `json:"outcomeContract,omitempty"`
	// OutcomeRetry is the re-prompt when the agent finished without node_complete.
	OutcomeRetry string `json:"outcomeRetry,omitempty"`
}

// Built-in default prompt fragments. These are the exact strings the platform
// injected before prompts became per-Agent configurable; kept here so an empty
// Agent field reproduces the original behavior.
const (
	DefaultUpstreamArtifactsHeader = "\n\n## 上游产物(只读输入)\n以下产物由上游节点产出,请用 `read_artifact` MCP 工具按名读取(它们不在工作区,不要去文件系统找):\n"
	DefaultProducesContract        = "\n## 产物契约(强制)\n完成前必须在工作目录(/root/workspace)写出文件 `{name}`,这是本节点的强制产物;未写出将判定为失败。\n"
	DefaultReactOpenSuffix         = "\n\n这是一次多轮澄清对话:先提出需要澄清的关键问题,等待我的回复后再继续,不要一次性给出最终结论。"
	DefaultProducesRetry           = "【必须完成】本节点尚未写入声明的产物 `{name}`,这是唯一未完成的强制要求。现在立即调用 write_artifact 工具写入 `{name}`(内容为本次澄清得到的结论),不要再提问、不要输出其它内容、不要给出解释——只需完成这次写入。"
	DefaultPlanContract            = "\n\n## 计划契约(强制)\n你是计划节点,唯一交付是调用 `set_plan` 工具写入一份最多两级(大目标→小目标)的结构化计划;不要写代码、改仓库或写其它产物文件。set_plan 调用成功即完成本节点。\n"
	DefaultImplementContract       = "\n\n## 实现契约(强制)\n你是实现节点:先用 `get_plan` 读取计划,按大目标→小目标逐项落地。**进度标记是硬性要求**:每开始一项先调用 `update_plan_status(id, \"in_progress\")`,该项做完立即调用 `update_plan_status(id, \"done\")`。平台仅凭这些状态判断完成度——只把代码写好却不标记,会被判为未完成并反复催促。结束前必须让所有叶子项都为 `done`。\n"
	DefaultPlanIncompleteRetry     = "以下计划项尚未标记为完成:\n{items}\n如果这些项对应的工作其实已经做完,请**立即**对每一项调用 `update_plan_status(id, \"done\")` 把状态补上,不要重复已完成的实现;若确有未完成的,先实现再标记。所有项都标记 done 前不要结束。"

	DefaultClarifiedRequirementContract = "\n\n## 需求契约(强制)\n你是需求澄清节点,唯一交付是调用 `set_clarified_requirement` 写入完整需求规格(对齐 ISO/IEC/IEEE 29148 / PRD 子集)。\n\n**必填字段**:`title`、`summary`、`background`、`goals[]`(≥1)、`in_scope[]`(≥1)、`out_of_scope[]`(≥1)、`functional_requirements[]`(≥1;每条含 `title`+`detail`+≥1 `acceptance_criteria`;`priority` 取 must|should|could,缺省 must)、`assumptions[]`/`dependencies[]`/`constraints[]`(各≥1;无实质内容时写明确「无额外…(已与用户确认)」,禁止省略键)。\n\n**可选字段**(有则写):`success_metrics`、`personas`、`user_scenarios`、`non_functional_requirements`(category: performance|security|usability|reliability|compatibility|other;可含 metric)、`external_interfaces`、`data_entities`、`business_rules`、`edge_cases`、`limitations`、`risks`、`glossary`。\n\n**禁止**:排期/里程碑/交付日期;技术选型、架构或详细 API/DB 设计(留给调研/方案节点)。不要写代码、改仓库或写其它产物文件。\n\n**澄清是门禁:任何还不确定、需要用户拍板的点,都必须通过 `ask_question` 让用户做选择,不能把疑问塞进 `open_questions` 就结束。** 用 `ask_question` 提问时,如果你对某个选项有明确倾向,请把它的 `recommended` 置为 true(每题最多一个),便于用户一键确认;在自动模式下平台会直接采用推荐项。调用 `set_clarified_requirement` 结束澄清时 `open_questions` 必须为空(留空或不传)。不要替用户擅自决定。\n\n## Demo 预览(可选)\n当问题涉及 **UI/交互/布局** 等视觉决策时,可为每个选项附带 `demoHtml`:以 `<!doctype html>` 开头的完整自包含 HTML 文档,前端用 iframe 并排或选中预览。**非 UI 类问题不要写 demoHtml。**\n- 每选项最多一个 Demo;允许引用 CDN(Tailwind、图标库等)。\n- ≤3 个含 Demo 的选项时界面三列并排对比;>3 时降级为选中后单预览。\n- 同一题内各选项 `label` 须唯一,便于历史轮次还原已选态。\n\n### demoHtml 运行环境(强制)\ndemoHtml 运行于 Gates HtmlPreview 的 sandbox iframe(sandbox=\"allow-scripts allow-forms\",无 allow-same-origin,文档为 opaque origin)。\n禁止:读取/写入 localStorage、sessionStorage;禁止:依赖 cookie 或同源 Web Storage 的持久化/登录态。\n需要完整 SPA、持久化或真实浏览器能力时,改走 app_preview(noVNC),不要在 srcdoc 中硬做,也不得引导恢复 allow-same-origin。\n"
	DefaultImplementResultContract      = "\n\n## 实现结果契约(强制)\n计划全部完成后:\n1. **提交并推送**:工作区根 `/root/workspace` 不是 git 仓库,每个仓库位于 `/root/workspace/<name>/`。对每个有改动的仓分别 `cd` 进其目录,各自 `git add` + `git commit`,再 `git push` 该仓的工作分支到远端(origin,多个仓可用同一分支名)。下游节点在全新克隆里工作,不推送就拿不到你的代码,不要遗漏任一改动仓。\n2. 然后调用 `set_implementation_result` 工具写入结构化的实现结果说明(概述 + 主要改动 + 测试情况 + 破坏性变更/后续),并在其中说明各仓的工作分支名。\n"
	DefaultResearchContract             = "\n\n## 调研契约(强制)\n你是调研节点,唯一交付是调用 `set_research` 工具写入结构化调研结论(概述 + 调研问题及结论/关键发现,可含建议与参考)。不要改仓库或写其它产物文件。\n"
	DefaultTestContract                 = "\n\n## 测试契约(强制)\n你是测试节点,唯一交付是调用 `set_test_result` 工具写入结构化测试总结(总体结论 + 用例结果 + 缺陷/偏差/评估)。请如实记录通过与失败,不要粉饰。\n\n**测试是门禁**:只要有用例失败(status=failed),平台会判定本节点未通过并把流程按失败/回滚边打回上游修复。因此务必据实填写每个用例的 status,不要为了通过而谎报。若节点配置了 `block_on_skipped=true`,则 skipped 用例同样会阻塞门禁(默认 false,仅 failed 阻塞,与单仓现网一致)。\n\n**仓库测试布局**:工作区根 `/root/workspace` 不是仓库,每个仓库位于 `/root/workspace/<name>/`。请在各仓子目录分别探测并执行测试(如 `go test ./...`、`npm test`),将结果汇总到**单一** `set_test_result.cases[]`;用例 `name` 建议加仓名前缀便于阅读,如「[frontend] 单元测试」「[backend] API 测试」。跨仓 E2E 时自行拉起各仓服务(绑定 `127.0.0.1:<port>`)后执行,无需 testMatrix 配置。\n\n**浏览器/端到端 E2E**:沙箱已预装无头 Chromium 与 Playwright 系统依赖(`PLAYWRIGHT_BROWSERS_PATH=/ms-playwright`)及中文字体。需要验证前端/全栈行为时,请在容器内自行启动被测应用(后端 + 前端绑定 `127.0.0.1:<port>`),再以无头方式跑项目 E2E 或临时 Playwright 脚本打 `http://127.0.0.1:<port>`。**不得仅以「没有完整 Web 应用/后端/浏览器,无法做 Playwright 验收」为由把浏览器 E2E 标为 skipped**——环境已具备,应自起应用后据实执行;确有具体技术原因无法执行时,才可 skipped 且须在该用例 detail 写明真实原因。做了浏览器/UI 测试时,把关键页面截图(最多 10 张)提交到测试结果:先把截图存成 PNG 文件,再用沙箱内置命令 `artifact-upload <文件> --caption \"说明\"` 上传(它会打印一个产物名),然后在 `set_test_result` 的 `screenshots` 里用 `{artifact: \"<打印出的产物名>\", caption: \"说明\"}` 引用;平台只保留 artifact 引用(及 caption/mimeType),**不再写时回填**内联图片数据,展示侧按引用懒加载。`screenshots` **只接受 artifact 引用,不支持内联 base64**。\n"
	DefaultReviewContract               = "\n\n## 评审契约(强制)\n你是评审节点,唯一交付是调用 `set_review` 工具写入结构化评审结论(结论 verdict + 概述 + 按严重度排列的意见与建议)。verdict 取 approve|approve_with_comments|request_changes|reject。\n\n**评审是门禁**:verdict 为 request_changes 或 reject 时,平台会判定本节点未通过并把流程按失败/回滚边打回上游整改;approve 或 approve_with_comments 才放行。请根据代码/设计实际质量如实给出 verdict。\n"
	DefaultProposalContract             = "\n\n## 方案契约(强制)\n你是方案节点,唯一交付是调用 `set_proposals` 工具写入结构化候选方案集(背景 + 至少 1 个方案,含优缺点/权衡/工作量/风险);如有推荐方案将其 recommended 置为 true。可给出多个方案供后续确认。\n"
	DefaultMRContract                   = "\n\n## 合并请求契约(强制)\n你是提交 MR 节点,目标是让源分支 `{source}` 能干净地合入目标分支 `{target}` 并存在一个对应的合并请求（MR/PR）。工作区根 `/root/workspace` 不是仓库,**先 `cd` 进目标仓目录(`/root/workspace/<name>/`)再执行以下所有 `git` 与对应 CLI（`glab`/`gh`）命令**。请依次完成:\n1. **对齐目标分支并解冲突**:`git fetch origin {target}`,把 `origin/{target}` 合入当前源分支(merge 或 rebase 均可),**逐个解决所有冲突**后 `git add` 已解决文件并提交。\n2. **推送**:`git push origin {source}`(源分支)。**无论后续能否自动建单,都必须先完成本步。**\n3. **按远端主机选型创建/复用合并请求**(按主机与环境变量匹配,不按 Token 有无或 CLI 轮询):\n   - **GitLab**（远端主机为 gitlab.com,或与 `GITLAB_URL` 主机一致）:用 `glab mr create --source-branch {source} --target-branch {target} --fill --yes`(若已存在则复用)。\n   - **GitHub**（远端主机为 github.com,或与 `GITHUB_URL` 主机一致,含 GHE）:用 `gh pr create --base {target} --head {source} --fill`(若已存在则复用);成功时 PR Web URL 写入同一字段 `outputs.mr_url`。\n   - **匹配不上**（如 Gitea 等）或不支持自动建单:不要假装已建单。\n   凭据由沙箱提供（`GITLAB_*` / `GITHUB_*`）;需预装对应 CLI（`glab` / `gh`）。成功时合并请求 Web URL 写入 `outputs.mr_url`（字段名不变）。\n4. **标记完成**:\n   - 建单成功:调用 `node_complete`(status=success, outputs 含 mr_url)。\n   - 无法建单（托管商不匹配、缺少 `glab`/`gh`、建单命令失败等）:**仍须先完成步骤 1–2**,再调用 `node_complete`(status=failed);`summary`/`error` 必须显式包含「冲突已解决」「源分支已推送」,并说明建单/CLI/托管商原因。**不采用**「推送成功即可 success、mr_url 可空」。\n平台不再代验推送/MR/冲突——以你的 node_complete 为准。\n"
	DefaultStructuredRetry              = "【必须完成】本节点尚未写入结构化产物 `{name}`,这是本节点唯一的强制交付,缺它即判失败。现在立即调用 `{tool}` 工具写入它(内容为本节点应产出的结论),不要再提问、不要输出其它内容——只需完成这次调用。"
	DefaultClarifiedOpenQuestionsRetry  = "【必须澄清】你写入的需求里仍有以下待确认问题没有和用户敲定:\n{items}\n澄清节点是门禁,不能带着未确认的问题结束。请现在用 `ask_question` 工具把这些问题逐一抛给用户做选择(每个问题给出候选选项),等用户确认后再重新调用 `set_clarified_requirement` 更新结论并清空 open_questions。不要直接结束澄清,也不要替用户擅自拍板。"
	DefaultVisualContract               = "\n\n## 视觉网页契约(强制)\n你是视觉网页节点,唯一交付是一个**单文件、自包含**的网页 `page.html`,把上游需求做成可直接打开预览的可视化 demo/原型,供人在「人工门禁」里确认后再开工。\n\n### 怎么交付(只有一种方式)\n**只调用 `write_artifact` 工具写入产物**:name 传 `page.html`,content 传完整 HTML,kind 传 `html`。\n**严禁在项目/工作区里写任何文件**——不要 `echo >`、不要新建/修改仓库文件、不要 `git add`,以免污染仓库改动。平台会把该产物登记为本次运行产物并用 iframe 预览。最终必须存在名为 `page.html` 的产物,否则判定为失败。\n\n### 硬性要求\n1. 一个完整的 HTML 文档(以 `<!doctype html>` 开头,含 `<html><head><body>`)。\n2. **所有** CSS 写进 `<style>`、所有 JS 写进 `<script>`,**全部内联**在这一个文件内。\n3. **不引用任何外部资源**(不要外链 CSS/JS/字体/图片 CDN);如需图形用内联 SVG 或 CSS 绘制。\n4. 页面会被放进 iframe 预览,做到简洁美观、能直接体现需求即可,只产出这一个 `page.html`。\n\n### page.html 运行环境(强制)\npage.html 运行于 Gates HtmlPreview 的 sandbox iframe(sandbox=\"allow-scripts allow-forms\",无 allow-same-origin,文档为 opaque origin)。\n禁止:读取/写入 localStorage、sessionStorage;禁止:依赖 cookie 或同源 Web Storage 的持久化/登录态。\n需要完整 SPA、持久化或真实浏览器能力时,改走 app_preview(noVNC),不要在 srcdoc 中硬做,也不得引导恢复 allow-same-origin。\n"
	DefaultPreviewContract              = "\n\n## 应用预览契约(强制)\n你是应用预览节点:在沙箱内构建并启动上游实现的应用,再用 `set_preview(port, label?)` 注册预览端口(可多次,如分别注册「前端」「API」)。这是本节点唯一交付;**不要调用 set_test_result**,本节点无结构化 JSON 产物。\n\n**直接在沙箱内原生启动进程**(如 `npm run dev`、`go run`、`python -m ...`)。沙箱内没有 Docker,**不要用 `docker`/`docker compose`**。前台启动即可,无需 nohup —— 调用 set_preview 后平台会托管保活。\n\n只需满足两条:\n\n1. **监听 `0.0.0.0:<port>`,不要只绑 `127.0.0.1`**:预览经容器网桥 IP 反向代理,只绑回环代理连不上(502)。Vite 加 `--host 0.0.0.0`;Node/Express `app.listen(port, '0.0.0.0')`;Python `--host 0.0.0.0`。\n2. **注册端口**:应用起来后调用 `set_preview(port, label?)`,port 必填、label 可选。\n\n应用**照常服务在根路径 `/` 即可,无需关心任何 base/子路径**——平台代理会透明地把资源和链接改写到预览子路径下。\n\n**预览是门禁**:会话正常结束且至少成功注册 1 次后,节点进入 waiting_human 人工审批;未注册时平台按 max_rounds(默认 3)同会话催促,超限仍无则节点失败。\n"
	DefaultPreviewRetry                 = "【必须完成】你尚未调用 `set_preview` 注册预览端口,这是本节点的强制要求。请在沙箱内**原生**启动应用(不要用 docker),让它监听 `0.0.0.0:<port>`(不能只绑 127.0.0.1;应用照常服务在根路径 `/` 即可,无需配置 base)。随后调用 `set_preview(port, label?)` 注册至少一个端口再结束本轮。"
	DefaultOutcomeContract              = "\n\n## 完成标记契约(强制)\n结束本节点前**必须**调用 `node_complete` 标记结果:`status` 取 `success` 或 `failed`;可选 `summary` / `error` / `outputs` / `checks`。写完产物(`set_*` / `write_artifact`)后再调用。未标记将被判定为节点失败。平台先做默认校验(产物/门禁等),通过后才可能做业务 RPC 校验。\n"
	DefaultOutcomeRetry                 = "【必须完成】你尚未调用 `node_complete` 标记本节点完成结果,这是强制要求。现在立即调用 `node_complete(status=\"success\"|\"failed\", summary?, error?, outputs?)`,不要再提问或输出其它内容——只需完成这次调用。\n"
)

// UpstreamHeader returns the configured upstream-artifacts header or the
// built-in default. Nil-safe.
func (p *AgentPrompts) UpstreamHeader() string {
	if p != nil && strings.TrimSpace(p.UpstreamArtifactsHeader) != "" {
		return p.UpstreamArtifactsHeader
	}
	return DefaultUpstreamArtifactsHeader
}

// ProducesContractFor returns the produces-contract clause with {name}
// substituted. Nil-safe.
func (p *AgentPrompts) ProducesContractFor(name string) string {
	tmpl := DefaultProducesContract
	if p != nil && strings.TrimSpace(p.ProducesContract) != "" {
		tmpl = p.ProducesContract
	}
	return strings.ReplaceAll(tmpl, "{name}", name)
}

// ReactOpenSuffixText returns the react opening-turn suffix or the default.
// Nil-safe.
func (p *AgentPrompts) ReactOpenSuffixText() string {
	if p != nil && strings.TrimSpace(p.ReactOpenSuffix) != "" {
		return p.ReactOpenSuffix
	}
	return DefaultReactOpenSuffix
}

// ProducesRetryFor returns the produces re-prompt with {name} substituted.
// Nil-safe.
func (p *AgentPrompts) ProducesRetryFor(name string) string {
	tmpl := DefaultProducesRetry
	if p != nil && strings.TrimSpace(p.ProducesRetry) != "" {
		tmpl = p.ProducesRetry
	}
	return strings.ReplaceAll(tmpl, "{name}", name)
}

// PlanContractText returns the plan-node contract clause or the default.
// Nil-safe.
func (p *AgentPrompts) PlanContractText() string {
	if p != nil && strings.TrimSpace(p.PlanContract) != "" {
		return p.PlanContract
	}
	return DefaultPlanContract
}

// ImplementContractText returns the implement-node contract clause or the
// default. Nil-safe.
func (p *AgentPrompts) ImplementContractText() string {
	if p != nil && strings.TrimSpace(p.ImplementContract) != "" {
		return p.ImplementContract
	}
	return DefaultImplementContract
}

// PlanIncompleteRetryFor returns the implement-node completion re-prompt with
// the incomplete-item list substituted for {items}. Nil-safe.
func (p *AgentPrompts) PlanIncompleteRetryFor(items []string) string {
	tmpl := DefaultPlanIncompleteRetry
	if p != nil && strings.TrimSpace(p.PlanIncompleteRetry) != "" {
		tmpl = p.PlanIncompleteRetry
	}
	var b strings.Builder
	for _, it := range items {
		b.WriteString("- ")
		b.WriteString(it)
		b.WriteString("\n")
	}
	return strings.ReplaceAll(tmpl, "{items}", strings.TrimRight(b.String(), "\n"))
}

// contractText is the shared nil-safe accessor for a fixed contract clause.
func contractText(override, def string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	return def
}

// ClarifiedRequirementContractText returns the react-node requirement contract.
func (p *AgentPrompts) ClarifiedRequirementContractText() string {
	if p == nil {
		return DefaultClarifiedRequirementContract
	}
	return contractText(p.ClarifiedRequirementContract, DefaultClarifiedRequirementContract)
}

// ClarifiedOpenQuestionsRetryFor returns the react-node gate re-prompt with the
// unresolved-question list substituted for {items}. Nil-safe.
func (p *AgentPrompts) ClarifiedOpenQuestionsRetryFor(items []string) string {
	tmpl := DefaultClarifiedOpenQuestionsRetry
	if p != nil && strings.TrimSpace(p.ClarifiedOpenQuestionsRetry) != "" {
		tmpl = p.ClarifiedOpenQuestionsRetry
	}
	var b strings.Builder
	for _, it := range items {
		b.WriteString("- ")
		b.WriteString(it)
		b.WriteString("\n")
	}
	return strings.ReplaceAll(tmpl, "{items}", strings.TrimRight(b.String(), "\n"))
}

// ImplementResultContractText returns the implement-node result contract.
func (p *AgentPrompts) ImplementResultContractText() string {
	if p == nil {
		return DefaultImplementResultContract
	}
	return contractText(p.ImplementResultContract, DefaultImplementResultContract)
}

// ResearchContractText returns the research-node contract.
func (p *AgentPrompts) ResearchContractText() string {
	if p == nil {
		return DefaultResearchContract
	}
	return contractText(p.ResearchContract, DefaultResearchContract)
}

// TestContractText returns the test-node contract.
func (p *AgentPrompts) TestContractText() string {
	if p == nil {
		return DefaultTestContract
	}
	return contractText(p.TestContract, DefaultTestContract)
}

// ReviewContractText returns the review-node contract.
func (p *AgentPrompts) ReviewContractText() string {
	if p == nil {
		return DefaultReviewContract
	}
	return contractText(p.ReviewContract, DefaultReviewContract)
}

// ProposalContractText returns the proposal-node contract.
func (p *AgentPrompts) ProposalContractText() string {
	if p == nil {
		return DefaultProposalContract
	}
	return contractText(p.ProposalContract, DefaultProposalContract)
}

// MRContractFor returns the submit_mr-node contract with the {source} and
// {target} branch placeholders substituted. Nil-safe.
func (p *AgentPrompts) MRContractFor(source, target string) string {
	tmpl := DefaultMRContract
	if p != nil && strings.TrimSpace(p.MRContract) != "" {
		tmpl = p.MRContract
	}
	tmpl = strings.ReplaceAll(tmpl, "{source}", source)
	return strings.ReplaceAll(tmpl, "{target}", target)
}

// VisualContractText returns the visual-node contract clause or the default.
// Nil-safe.
func (p *AgentPrompts) VisualContractText() string {
	if p != nil && strings.TrimSpace(p.VisualContract) != "" {
		return p.VisualContract
	}
	return DefaultVisualContract
}

// PreviewContractText returns the app_preview-node contract clause or the default.
func (p *AgentPrompts) PreviewContractText() string {
	if p != nil && strings.TrimSpace(p.PreviewContract) != "" {
		return p.PreviewContract
	}
	return DefaultPreviewContract
}

// PreviewRetryText returns the app_preview re-prompt when set_preview was not called.
func (p *AgentPrompts) PreviewRetryText() string {
	if p != nil && strings.TrimSpace(p.PreviewRetry) != "" {
		return p.PreviewRetry
	}
	return DefaultPreviewRetry
}

// StructuredRetryFor returns the generic structured-product re-prompt with the
// artifact {name} and its {tool} substituted. Nil-safe.
func (p *AgentPrompts) StructuredRetryFor(name, tool string) string {
	tmpl := DefaultStructuredRetry
	if p != nil && strings.TrimSpace(p.StructuredRetry) != "" {
		tmpl = p.StructuredRetry
	}
	tmpl = strings.ReplaceAll(tmpl, "{name}", name)
	return strings.ReplaceAll(tmpl, "{tool}", tool)
}

// OutcomeContractText returns the node_complete contract clause. Nil-safe.
func (p *AgentPrompts) OutcomeContractText() string {
	if p != nil && strings.TrimSpace(p.OutcomeContract) != "" {
		return p.OutcomeContract
	}
	return DefaultOutcomeContract
}

// OutcomeRetryText returns the re-prompt when node_complete was not called.
func (p *AgentPrompts) OutcomeRetryText() string {
	if p != nil && strings.TrimSpace(p.OutcomeRetry) != "" {
		return p.OutcomeRetry
	}
	return DefaultOutcomeRetry
}
