/**
 * 会话更新 / 工具调用的纯函数归一化（无 DOM）。
 * ACP 事件形状与 kind 规则仍以 ../core/acp_protocol.js 为准。
 */

/**
 * 常见形态：{ sessionUpdate: { type, toolCall, ... } }，与外层合并后才有顶层的 kind / toolCall。
 */
export function mergeSessionUpdateEnvelope(upd) {
    if (!upd || typeof upd !== 'object') return upd;
    const su = upd.sessionUpdate ?? upd.session_update;
    if (su && typeof su === 'object' && !Array.isArray(su)) {
        const out = {...upd, ...su};
        delete out.sessionUpdate;
        delete out.session_update;
        return out;
    }
    return upd;
}

/** 展平 toolCall / call；空字符串 content 不占位，避免吞掉 toolCall.arguments（供 ToolCallCard args 展示） */
export function flattenToolPayload(upd) {
    if (!upd || typeof upd !== 'object') return upd;

    /*
     * ACP session/update 常见形如：
     *   tool_call:        { sessionUpdate: "tool_call", toolCallId, title, status, rawInput: {...} }
     *   tool_call_update:  { sessionUpdate: "tool_call_update", toolCallId, status, rawOutput: { content: "..." } }
     * rawOutput.content 需要提升到顶层 content，rawInput 需要提升到 arguments。
     */
    const ro = upd.rawOutput;
    if (ro && typeof ro === 'object' && upd.content === undefined) {
        /*
         * 直接将整个 rawOutput 序列化为 JSON 展示，兼容所有工具类型：
         *   Read File  → { content: "..." }
         *   Terminal   → { exitCode: 0, output: "..." }
         *   搜索/其他  → 任意结构
         * 不再逐个猜测字段名，统一展示完整结果。
         */
        let extracted;
        try {
            extracted = JSON.stringify(ro, null, 2);
        } catch (_) { /* ignore */
        }
        if (extracted && extracted !== '{}') {
            upd = {...upd, content: extracted};
        }
    }
    const ri = upd.rawInput;
    if (ri && typeof ri === 'object' && upd['arguments'] === undefined && Object.keys(ri).length > 0) {
        upd = {...upd, arguments: ri};
    }

    const tc = upd.toolCall || upd.tool || upd.call;
    if (tc && typeof tc === 'object') {
        const nestedContent =
            tc.content !== undefined
                ? tc.content
                : tc['arguments'] !== undefined
                    ? tc['arguments']
                    : tc.result !== undefined
                        ? tc.result
                        : undefined;
        const top = upd.content;
        const topEmpty =
            top === undefined ||
            top === null ||
            (typeof top === 'string' && top.trim() === '') ||
            (typeof top === 'object' &&
                !Array.isArray(top) &&
                Object.keys(top).length === 0);
        const mergedContent =
            topEmpty && nestedContent !== undefined ? nestedContent : top;

        const argsFromTc =
            tc['arguments'] !== undefined
                ? tc['arguments']
                : tc.args !== undefined
                    ? tc.args
                    : tc.input !== undefined
                        ? tc.input
                        : undefined;

        return {
            ...upd,
            toolCallId: upd.toolCallId || tc.toolCallId || tc.id || tc.callId,
            title: upd.title || tc.title || tc.name || tc.toolName,
            status: upd.status || tc.status || tc.state,
            content: mergedContent !== undefined ? mergedContent : top,
            arguments:
                upd['arguments'] !== undefined
                    ? upd['arguments']
                    : upd.args !== undefined
                        ? upd.args
                        : argsFromTc,
        };
    }
    return upd;
}

/** 是否像工具状态增量（无 sessionUpdate 时仍可能只推 { status }） */
export function looksLikeToolDelta(upd) {
    if (!upd || typeof upd !== 'object') return false;
    if (upd.toolCallId || upd.toolCall || upd.tool || upd.call) return true;
    const st = upd.status || upd.state;
    if (!st) return false;
    const keys = Object.keys(upd).filter((k) => !['content', 'raw', 'params'].includes(k));
    return keys.length <= 4;
}

/** 从 content 块里抠文本（兼容 text / 字符串 / 片段数组） */
export function extractTextFromContent(content) {
    if (content == null) return '';
    if (typeof content === 'string') return content;
    if (typeof content.text === 'string') return content.text;
    if (Array.isArray(content)) {
        return content.map((p) => extractTextFromContent(p)).join('');
    }
    if (typeof content === 'object' && Array.isArray(content.parts)) {
        return content.parts.map((p) => extractTextFromContent(p)).join('');
    }
    if (typeof content === 'object' && content.type === 'text' && typeof content.text === 'string') {
        return content.text;
    }
    return '';
}

export function statusIcon(status) {
    const s = String(status || 'pending').toLowerCase().replace(/-/g, '_');
    if (s === 'completed' || s === 'complete' || s === 'success') return '✓';
    if (s === 'failed' || s === 'error' || s === 'cancelled' || s === 'canceled') return '✗';
    if (s === 'in_progress' || s === 'running' || s === 'executing') return '⏳';
    return '◆';
}

export function formatToolContent(content) {
    if (content == null) return '';
    if (typeof content === 'string') return content;
    try {
        return JSON.stringify(content, null, 2);
    } catch {
        return String(content);
    }
}

/**
 * 判断对象是否像工具参数对象。
 * @param {object} o
 * @returns {boolean}
 */
function toolLikeObject(o) {
    if (!o || typeof o !== 'object' || Array.isArray(o)) return false;
    const keys = Object.keys(o);
    if (keys.length === 0) return false;
    return !(o.type === 'text' && typeof o.text === 'string' && keys.length <= 3);
}

/**
 * 从 ACP 多种形态里抠「工具入参」（与流式 content / result 区分）。
 * 参数常在 content（JSON 字符串）、function_call、或扁平字段里。
 * @param {object} upd
 * @returns {unknown|null}
 */
export function collectToolParams(upd) {
    if (!upd || typeof upd !== 'object') return null;
    const tc = upd.toolCall || upd.tool || upd.call;
    const fc = upd.functionCall || upd.function_call || tc?.functionCall || tc?.function_call;
    const item = upd.item || upd.rawItem;
    const itemFc = item && (item.functionCall || item.function_call);
    /* session/update 中 rawItem 可能包含完整的工具调用信息 */
    const rawItem = upd.rawItem || upd.item;
    const rawItemTc = rawItem && (rawItem.toolCall || rawItem.tool || rawItem.call);
    const candidates = [
        upd.params,
        upd.parameters,
        upd['arguments'],
        upd.args,
        upd.input,
        upd.toolInput,
        upd.toolInvocation,
        upd.rawInput,
        fc && fc['arguments'],
        fc && fc.args,
        itemFc && itemFc['arguments'],
        itemFc && itemFc.args,
        item && item['arguments'],
        rawItemTc && rawItemTc['arguments'],
        rawItemTc && rawItemTc.args,
        rawItemTc && rawItemTc.input,
        tc && tc.params,
        tc && tc.parameters,
        tc && tc['arguments'],
        tc && tc.args,
        tc && tc.input,
        tc && tc.toolArguments,
        tc && tc.rawArguments,
        upd.rawArguments,
        upd.toolArguments,
    ];
    for (const c of candidates) {
        if (c == null) continue;
        if (typeof c === 'string') {
            const t = c.trim();
            if (!t) continue;
            if ((t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))) {
                try {
                    const j = JSON.parse(t);
                    if (toolLikeObject(j)) return j;
                } catch (_) {
                    /* 非 JSON 仍可作为原始参数串 */
                }
            }
            return t;
        }
        if (typeof c === 'object') {
            if (Array.isArray(c) && c.length > 0) return c;
            if (!Array.isArray(c) && toolLikeObject(c)) return c;
        }
    }
    /* content 整段即 JSON 工具参数（flatten 后常见） */
    const raw = upd.content;
    if (raw != null) {
        if (typeof raw === 'object' && toolLikeObject(raw)) return raw;
        if (typeof raw === 'string') {
            const t = raw.trim();
            if (t.startsWith('{') || t.startsWith('[')) {
                try {
                    const j = JSON.parse(t);
                    if (typeof j === 'object' && j !== null && toolLikeObject(j)) return j;
                } catch (_) {
                }
            }
        }
    }
    return null;
}

/** 超长字符串中间省略展示。 */
export function truncateMiddle(str, headChars, tailChars) {
    const s = String(str);
    const chars = [...s];
    if (chars.length <= headChars + tailChars + 8) {
        return {text: s};
    }
    const omitted = chars.length - headChars - tailChars;
    const text =
        chars.slice(0, headChars).join('') +
        `\n\n... (omitted ${omitted} chars) ...\n\n` +
        chars.slice(-tailChars).join('');
    return {text};
}

/** 取较长 / 更完整的参数快照（后续增量可能补全字段） */
export function pickRicherParams(prev, next) {
    if (next == null) return prev;
    if (prev == null) return next;
    const a = formatToolContent(prev);
    const b = formatToolContent(next);
    return b.length >= a.length ? next : prev;
}
