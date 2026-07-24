/**
 * 内置 session/update 处理器（原 chat_view.handleEvent 分支）。
 * 扩展：在其它模块 import { registerSessionUpdateHandler } from './session_update_dispatch.js'
 * 并 register({ id, priority, match, handle })，priority 高于内置即可抢先处理。
 */

import {isToolSessionUpdateKind, shouldSilenceUnknownUpdate,} from '../core/acp_protocol.js';
import {extractTextFromContent, looksLikeToolDelta,} from '../ui/chat_payload.js';

/** @param {{ register: (h: object) => void }} router */
export function installBuiltinSessionHandlers(router) {
    router.register({
        id: 'agent_message_chunk',
        priority: 100,
        match: (ctx) => ctx.kind === 'agent_message_chunk',
        handle(ctx) {
            const t = extractTextFromContent(ctx.flat.content);
            if (t) ctx.chatView.appendStreamAgent(t);
        },
    });

    router.register({
        id: 'agent_thought_chunk',
        priority: 100,
        match: (ctx) => ctx.kind === 'agent_thought_chunk',
        handle(ctx) {
            const t = extractTextFromContent(ctx.flat.content);
            if (t) ctx.chatView.appendStreamThought(t);
        },
    });

    router.register({
        id: 'user_message_chunk',
        priority: 100,
        match: (ctx) => ctx.kind === 'user_message_chunk',
        handle(ctx) {
            const t = extractTextFromContent(ctx.flat.content);
            if (t) {
                ctx.chatView.endStream();
                ctx.chatView.appendMetaInAssistantTurn('notice', `用户消息引用 · ${t}`);
            }
        },
    });

    router.register({
        id: 'plan',
        priority: 90,
        match: (ctx) => ctx.kind === 'plan' && !!ctx.flat.entries,
        handle(ctx) {
            const lines = ctx.flat.entries
                .map(
                    (e) =>
                        `- [${e.status}] ${e.content}${e.priority ? ` (${e.priority})` : ''}`
                )
                .join('\n');
            ctx.chatView.appendToolCard('plan', lines, true, 'cc-tool-plan');
        },
    });

    router.register({
        id: 'tool_call_kinds',
        priority: 85,
        match: (ctx) => isToolSessionUpdateKind(ctx.kind),
        handle(ctx) {
            ctx.chatView._mergeToolCall(ctx.flat);
        },
    });

    router.register({
        id: 'available_commands_update',
        priority: 50,
        match: (ctx) => ctx.kind === 'available_commands_update',
        handle(ctx) {
            const list = ctx.flat.availableCommands || ctx.flat.commands || [];
            const names = list.map((c) => c && c.name).filter(Boolean);
            let summary = '可用命令列表已更新';
            if (list.length) {
                const head = names.slice(0, 6).join('、');
                summary = `已加载 ${list.length} 条可用命令${head ? `（${head}${names.length > 6 ? '…' : ''}）` : ''}`;
            }
            ctx.chatView.appendMetaInAssistantTurn('notice', summary);
        },
    });

    router.register({
        id: 'silent_meta',
        priority: 10,
        match: (ctx) =>
            ctx.kind === 'current_mode_update' || ctx.kind === 'session_info_update',
        handle() {
            /* 纯元数据，不刷 UI */
        },
    });

    router.register({
        id: 'tool_delta_heuristic',
        priority: 5,
        match: (ctx) => !ctx.kind && looksLikeToolDelta(ctx.flat),
        handle(ctx) {
            ctx.chatView._mergeToolCall(ctx.flat);
        },
    });

    router.register({
        id: 'fallback',
        priority: -1000,
        match: () => true,
        handle(ctx) {
            if (shouldSilenceUnknownUpdate(ctx.flat, ctx.kind)) return;
            if (ctx.kind) {
                console.debug('acp-bridge: 未展示 session_update kind=', ctx.kind, ctx.flat);
            }
        },
    });
}
