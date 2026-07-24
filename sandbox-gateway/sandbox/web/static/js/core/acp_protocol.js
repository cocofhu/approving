/**
 * ACP 在本项目中的数据流（便于对齐前端与 internal/acp）：
 *
 * 1) 传输层：宿主进程与 Agent 子进程通过 **stdin/stdout 各一行一条 JSON**（NDJSON）
 *    交互，语义为 **JSON-RPC 2.0**（见 internal/acp/conn.go：request / response / notification）。
 *
 * 2) 宿主 → Agent：initialize、session/new、session/prompt、session/cancel 等；参数与返回结构由
 *    Agent 协议定义，本仓库在 panel.go 中实现。
 *
 * 3) Agent → 宿主（通知）：**`session/update`**，params 含 `sessionId` 与 **`update`（任意 JSON）**。
 *    Panel 校验 sessionId 后原样包成浏览器事件：`{ type: 'session_update', sessionId, update }`。
 *
 * 4) Agent → 宿主（请求）：**`fs/read_text_file`**、**`fs/write_text_file`**、**`terminal/*`**、
 *    **`session/request_permission`** 等；由 Panel 处理并 JSON-RPC 应答（panel.go）。
 *
 * 5) 宿主 → 浏览器（WebSocket `op: event`）：除上述 session_update 外，还有 Panel **自行构造**的
 *    辅助事件，例如 **`prompt_done`**（session/prompt 返回后）、**`fs_write`**、**`terminal_create`**
 *   （便于 UI 展示副作用，与 ACP 流式 update 并列）。
 *
 * 6) `update` 内部常见字段：不同版本 Agent 可能使用 **`sessionUpdate` / `type` / `kind`** 表示子类型；
 *    亦可能出现 **camelCase**（如 agentMessageChunk）。前端应归一化后再分支。
 */

/** 常见工具名 → 简短中文标签（其余再做格式化） */
const TOOL_TITLE_ZH = {
    read_file: '读取文件',
    write: '写入文件',
    write_file: '写入文件',
    search_replace: '搜索替换',
    delete_file: '删除文件',
    list_dir: '列出目录',
    glob_file_search: '文件模式匹配',
    grep: '文本搜索',
    codebase_search: '代码语义搜索',
    read_lints: '读取诊断',
    run_terminal_cmd: '终端命令',
    web_search: '联网搜索',
    todo_write: '任务列表',
    plan: '计划',
};

/**
 * 从 session/update 的 payload 取出归一化的小写 snake_case 子类型。
 * @param {object} upd
 * @returns {string}
 */
export function normalizeSessionUpdateKind(upd) {
    if (!upd || typeof upd !== 'object') return '';
    let raw = upd.sessionUpdate ?? upd.session_update;
    if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
        raw = raw.type ?? raw.kind ?? raw.sessionUpdate ?? '';
    }
    if (raw == null || raw === '') {
        raw = upd.type ?? upd.kind ?? '';
    }
    let s = String(raw).trim();
    if (!s) return '';
    s = s.replace(/([a-z\d])([A-Z])/g, '$1_$2').replace(/-/g, '_');
    return s.toLowerCase();
}

/**
 * 工具卡标题：优先中文映射，否则把 snake_case 转成可读短名。
 * @param {string} title
 * @returns {string}
 */
export function humanizeToolTitle(title) {
    const t = String(title || '').trim();
    if (!t) return '工具';
    if (/[\u4e00-\u9fff]/.test(t)) return t;
    const key = t.toLowerCase().replace(/\s+/g, '_').replace(/-/g, '_');
    if (TOOL_TITLE_ZH[key]) return TOOL_TITLE_ZH[key];
    return t
        .replace(/_/g, ' ')
        .replace(/\s+/g, ' ')
        .replace(/\b\w/g, (c) => c.toUpperCase());
}

/**
 * 无 sessionUpdate 且无可识别字段时，跳过 UI（减少「收到会话更新」噪音）。
 * @param {object} flat
 * @param {string} kind
 */
export function shouldSilenceUnknownUpdate(flat, kind) {
    if (kind) return false;
    if (!flat || typeof flat !== 'object') return true;
    const keys = Object.keys(flat).filter((k) => k !== 'sessionId' && k !== 'session_id');
    return keys.length === 0;
}

/** 是否为工具调用类 session_update（含 MCP / 变体命名）。 */
export function isToolSessionUpdateKind(k) {
    if (!k) return false;
    switch (k) {
        case 'tool_call':
        case 'tool_call_update':
        case 'toolcall':
        case 'toolcall_update':
        case 'mcp_tool_call':
        case 'mcp_tool_call_update':
            return true;
        default:
            return k.includes('tool_call') || k.includes('toolcall');
    }
}
