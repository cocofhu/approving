---
description: 测试(test)节点行为
alwaysApply: false
---

# 测试节点

本节点是**测试节点**:对上游实现执行测试,产出结构化测试总结报告(对齐 IEEE 829 Test Summary Report)。

## 唯一交付:set_test_result

- 你的唯一结构化交付是调用 `set_test_result` MCP 工具写入测试总结:
  - `summary`:总体结论(必填,例如是否达标、覆盖范围);
  - `cases[]`:用例结果(`name` + `status`,`status` 取 `passed|failed|skipped`,可选 `detail`);
  - `defects[]`:发现的缺陷(`title`,可选 `severity` / `detail`);
  - 视情况补 `variances`(与计划偏差)、`assessment`(综合评估/是否可发布)。
- **如实记录**通过与失败,不要粉饰;通过/失败/跳过数量由平台按 `cases` 自动统计。
- 需运行实际测试时,可在工作区执行相应命令并把结果汇总进上述结构;`set_test_result` 是本节点的交付。

## 门禁:测试结论决定放行(重要)

- **测试是门禁**:只要有任一用例 `status=failed`,平台就判定本节点未通过,并按失败/回滚边把流程打回上游修复,不会带着失败用例继续往下走。
- 全部通过(无 failed 用例)才放行。若节点配置了 `block_on_skipped=true`,则 skipped 用例同样阻塞门禁;默认 `false` 时 skipped 不阻塞(与单仓现网一致)。
- 正因如此,更要**据实**标注每个用例的 status,不要为了让流程通过而谎报为 passed。

## 多仓 WORKSPACE 布局与汇总

当 Run 配置了 `repos` 全局变量(多仓模式)时:

- **目录约定**:主仓 clone 在 WORKSPACE 根目录(`/root/workspace`),附加仓在 `/root/workspace/<name>/`(如 `/root/workspace/frontend/`)。
- **自主探测测试命令**:在各仓子目录分别执行项目自带的测试(如 `go test ./...`、`npm test`、`pytest`),不要依赖 testMatrix 显式配置。
- **汇总至单一 set_test_result**:把所有仓的用例写入**同一个** `set_test_result.cases[]`;用例 `name` 建议加仓名前缀,如「[frontend] 单元测试」「[backend] API 测试」,便于 Run 详情按仓分组展示。
- **repoScope**:节点 config 可设 `repoScope`(默认 `all`)。为 `all` 时须覆盖 repos 中全部相关仓;为单仓名(如 `frontend`)时,**仅**在 `/root/workspace/frontend/` 执行测试与读写(首期 Prompt 软约束)。
- **跨仓 E2E**:不单独配置 integration 条目。在主仓或任一仓目录内自行拉起各仓服务(绑定 `127.0.0.1:<port>`),再跑 Playwright/E2E,结果如实汇总进 `cases`。

单仓模式(仅 `repo_url`、无 `repos`)时:WORKSPACE 仍为单一 git 根,按现网方式测试即可,cases 命名无需 `[repo]` 前缀。

## 浏览器 / 端到端(E2E)测试

沙箱已预装无头浏览器,可直接跑前端/全栈的浏览器 E2E,**不需要外部环境**:

- **已具备的能力**:容器内装好了 Chromium 与 Playwright 的系统依赖(共享浏览器目录 `PLAYWRIGHT_BROWSERS_PATH=/ms-playwright`),并装了中文字体,中文页面渲染/截图正常。项目自带 `@playwright/test` / `playwright` 时直接复用该浏览器;若项目 pin 了不同版本,`npx playwright install chromium` 会按需补齐(系统依赖已就绪,很快)。
- **自己把被测应用跑起来**:E2E 需要一个运行中的应用。请在容器内先装依赖并启动被测服务——后端(如 `go run ./cmd/server`、`python app.py`、`npm run start`)与前端(如 `npm ci && npm run build && npm run preview`,或 `npm run dev`)**绑定到 `127.0.0.1:<port>`**,再让浏览器 E2E 打 `http://127.0.0.1:<port>`。无内置数据库服务时,用应用自带的种子/内存/SQLite 数据或在测试里 mock 网络接口。多仓场景下可能需同时拉起主仓与附加仓的服务后再跑跨仓 E2E。
- **执行 E2E**:优先跑项目自带的 E2E(如 `npx playwright test`);项目没有现成用例时,可临时写一个最小 Playwright 无头脚本验证目标行为,跑完把结果如实汇总进 `set_test_result` 的 `cases`。
- **附上截图(只能用 CLI 上传,不支持内联 base64)**:做了浏览器/UI 测试时,把关键页面截图(最多 10 张)提交到测试结果,便于查看。流程:
  1. 用 Playwright 等把页面截成 PNG 文件(建议视口截图而非超大整页);
  2. 对每张图运行沙箱内置命令 `artifact-upload <文件> --caption "说明"`,它会把图片上传到产物存储并在 stdout 打印一个产物名(如 `screenshot-1750000000-ab12cd34.png`);
  3. 把这些产物名填进 `set_test_result` 的 `screenshots`,每张为 `{artifact: "<上一步打印的产物名>", caption: "说明"}`。平台只保留 artifact 引用(及 caption/mimeType),**不再写时回填**内联图片数据;展示侧按引用懒加载。
  - `screenshots` **只接受 `artifact` 引用**;不要(也无法)直接内联 base64 图片数据。若误带 `data` 会被剥离,元数据仍保留。
- **禁止「缺环境式」跳过**:**不得**仅以「没有完整 Web 应用/没有后端/没有浏览器/无法做手工或 Playwright 验收」为由把浏览器 E2E 标为 `skipped`——这些环境本节点都已具备,应当自起被测应用后据实执行。只有确有**具体技术原因**(例如该用例依赖真实第三方凭据、依赖无法在沙箱内提供的外部系统)才可 `skipped`,且必须在该用例 `detail` 写清真实原因,而非笼统跳过。
