import {wsURL} from '../core/paths.js';
import {ChatView} from '../ui/chat_view.js';

/**
 * 与 Agent 会话就绪（顶栏「已连接」）时显示绿色状态点
 * @param {HTMLElement|null} statusEl #status
 * @param {boolean} live
 */
export function setHeaderConnLive(statusEl, live) {
    const wrap = statusEl?.closest?.('.cc-header-status-wrap');
    if (wrap) wrap.classList.toggle('cc-conn-live', live);
}

export class BrowserSession {
    /**
     * @param {{
     *   statusEl: HTMLElement,
     *   chat: ChatView,
     *   getCwd: () => string,
     *   getAutoPerm: () => boolean,
     *   onConnected?: (connected: boolean) => void,
     *   onRestartAvailable?: (available: boolean) => void,
     *   onQueueState?: (m: { busy?: boolean, queue_entries?: { text?: string, opId?: string }[] }) => void,
     *   onModelUpdate?: (model: { id: string, name: string } | null, currentModel: string) => void
     * }} opts
     */
    constructor(opts) {
        this.opts = opts;
        /** @type {WebSocket|null} */
        this.ws = null;
        /** ACP session/new 完成前禁止发 chat */
        this.panelReady = false;
        /** 超时主动 close 时避免 onclose 覆盖已写的超时文案 */
        this._suppressCloseStatus = false;
        /** 相同错误短时间内只打一条聊天通知，避免刷屏 */
        this._lastErrSig = '';
        this._lastErrAt = 0;
        /** 服务端可能先推 connected 再收到客户端 connect，避免重复「会话就绪」 */
        this._sessionId = '';
    }

    _setConn(connected) {
        this.opts.onConnected?.(connected);
    }

    /** WebSocket 已打开即可请求服务端重启 Agent（无需等 panelReady，便于 Agent 崩溃后自救） */
    _syncRestartBtn() {
        const ok = this.ws !== null && this.ws.readyState === WebSocket.OPEN;
        this.opts.onRestartAvailable?.(ok);
    }

    connect() {
        const el = this.opts.statusEl;
        if (!el) {
            console.error('acp-bridge: 找不到 #status，无法更新连接状态');
            return;
        }

        if (this.ws) {
            try {
                this.ws.close();
            } catch (_) {
            }
        }
        this.panelReady = false;
        el.textContent = '正在连接 WebSocket…';
        setHeaderConnLive(el, false);
        this._setConn(false);

        const url = wsURL();
        const hangMs = 20000;
        let hangTimer = setTimeout(() => {
            hangTimer = null;
            if (!this.ws || this.ws.readyState !== WebSocket.CONNECTING) return;
            this._suppressCloseStatus = true;
            try {
                this.ws.close();
            } catch (_) {
            }
            el.textContent =
                '无法连接 WebSocket（超时）。请用浏览器打开运行 acp-bridge 的地址（例如 http://127.0.0.1:8765 ），不要双击本地 HTML；若走反向代理需转发 WebSocket 到与页面同前缀的 /ws。当前尝试: ' +
                url;
            setHeaderConnLive(el, false);
            this._setConn(false);
        }, hangMs);

        const clearHang = () => {
            if (hangTimer != null) {
                clearTimeout(hangTimer);
                hangTimer = null;
            }
        };

        this.ws = new WebSocket(url);
        this.ws.onopen = () => {
            clearHang();
            el.textContent = '正在与 Agent 握手…';
            setHeaderConnLive(el, false);
            const msg = {
                op: 'connect',
                cwd: this.opts.getCwd(),
                fsRoot: '',
                mcpServers: null,
                autoPermission: this.opts.getAutoPerm(),
            };
            this.ws.send(JSON.stringify(msg));
            this._syncRestartBtn();
        };
        this.ws.onmessage = (ev) => {
            let m;
            try {
                m = JSON.parse(ev.data);
            } catch {
                return;
            }
            if (m.op === 'connected') {
                this._lastErrSig = '';
                this._lastErrAt = 0;
                const sid = m.sessionId || '';
                const prevSid = this._sessionId;
                const dup = this.panelReady && prevSid === sid;
                this._sessionId = sid;
                this.panelReady = true;
                this.opts.statusEl.textContent = '已连接';
                setHeaderConnLive(this.opts.statusEl, true);
                this._setConn(true);
                this.opts.chat.setPersistSessionId(sid);
                if (!dup) {
                    // 新 sessionId（含重启 Agent）：先清界面与旧会话本地快照，再按后端上下文恢复
                    if (prevSid && prevSid !== sid) {
                        this.opts.chat.clearPersistedLogForSession(prevSid);
                    }
                    this.opts.chat.clearConversationUi();
                    const eventLog = Array.isArray(m.eventLog) ? m.eventLog : [];
                    void (async () => {
                        if (eventLog.length > 0) {
                            this.opts.chat.replayEventLog(eventLog, m.userTimeline || []);
                        } else {
                            await this.opts.chat.restorePersistedIfSession(sid);
                            this.opts.chat.applyUserTimelineFromServer(m.userTimeline || []);
                        }
                        this.opts.chat.setHistoryPaging(m.totalTurns || 0, !!m.hasMoreTurns);
                    })();
                }
                this.opts.onModelUpdate?.(m.model || null, m.currentModel || '');
                this.opts.onQueueState?.(m);
            } else if (m.op === 'error') {
                const msg = m.message || '未知错误';
                const sig = `${m.agentExited ? 'e' : 'o'}:${msg}`;
                const now = Date.now();
                const dupChat = sig === this._lastErrSig && now - this._lastErrAt < 5000;
                if (!dupChat) {
                    this._lastErrSig = sig;
                    this._lastErrAt = now;
                }
                if (m.agentExited) {
                    this._sessionId = '';
                    this.panelReady = false;
                    this.opts.statusEl.textContent = `Agent 已退出 · ${msg}`;
                    setHeaderConnLive(this.opts.statusEl, false);
                    this._setConn(false);
                    if (!dupChat) this.opts.chat.appendNotice('error', msg);
                } else if (!this.panelReady) {
                    this.opts.statusEl.textContent = `连接失败 · ${msg}`;
                    setHeaderConnLive(this.opts.statusEl, false);
                    if (!dupChat) this.opts.chat.appendNotice('error', msg);
                } else {
                    this.opts.statusEl.textContent = `请求失败 · ${msg}`;
                    setHeaderConnLive(this.opts.statusEl, false);
                    if (!dupChat) this.opts.chat.appendNotice('error', msg);
                }
            } else if (m.op === 'event') {
                this.opts.chat.handleEvent(m.data);
            } else if (m.op === 'permission_request') {
                this.showPermission(m);
            } else if (m.op === 'queue_state') {
                this.opts.onQueueState?.(m);
            }
            this._syncRestartBtn();
        };
        this.ws.onclose = (ev) => {
            clearHang();
            this._sessionId = '';
            this.panelReady = false;
            if (this._suppressCloseStatus) {
                this._suppressCloseStatus = false;
                setHeaderConnLive(el, false);
                this._setConn(false);
                this._syncRestartBtn();
                return;
            }
            if (ev.code === 1006) {
                el.textContent = 'WebSocket 异常断开（多为代理未支持 WebSocket Upgrade，或网络中断）';
            } else if (ev.code !== 1000 && ev.code !== 1001) {
                el.textContent = `连接已断开（code ${ev.code}${ev.reason ? ' ' + ev.reason : ''}）`;
            } else {
                el.textContent = '连接已断开';
            }
            setHeaderConnLive(el, false);
            this._setConn(false);
            this._syncRestartBtn();
        };
        this.ws.onerror = () => {
            clearHang();
            this.panelReady = false;
            el.textContent = 'WebSocket 错误（无法建立连接）。请确认已启动 acp-bridge 且用 http(s) 访问同一主机，尝试地址: ' + url;
            setHeaderConnLive(el, false);
            this._setConn(false);
            this._syncRestartBtn();
        };
    }

    /** @param {{ rpcId: string, params: any }} m */
    showPermission(m) {
        const overlay = document.getElementById('perm');
        const text = document.getElementById('permText');
        const box = document.getElementById('permOpts');
        overlay.classList.add('active');
        text.textContent = JSON.stringify(m.params, null, 2);
        box.innerHTML = '';
        const opts = (m.params && m.params.options) || [];
        opts.forEach((o) => {
            const b = document.createElement('button');
            b.type = 'button';
            b.className = 'cc-btn cc-btn-block';
            const kind = (o.kind || '').toLowerCase();
            if (kind.includes('reject') || kind.includes('deny')) {
                b.classList.add('cc-perm-reject');
            } else if (kind.includes('allow') || kind.includes('approve')) {
                b.classList.add('cc-perm-approve');
            } else {
                b.classList.add('cc-btn-outline');
            }
            b.textContent = o.name || o.optionId;
            b.onclick = () => {
                this.ws.send(
                    JSON.stringify({op: 'permission', rpcId: String(m.rpcId), optionId: o.optionId})
                );
                overlay.classList.remove('active');
            };
            box.appendChild(b);
        });
    }

    sendChat(text, images = []) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            throw new Error('请先连接 WebSocket');
        }
        if (!this.panelReady) {
            throw new Error('请先等待 Agent 握手完成（状态为「已连接」后再发消息）');
        }
        const opId =
            typeof crypto !== 'undefined' && crypto.randomUUID
                ? crypto.randomUUID()
                : `c-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
        const msg = {op: 'chat', text, opId};
        if (images.length > 0) msg.images = images;
        this.ws.send(JSON.stringify(msg));
    }

    cancel() {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({op: 'cancel'}));
        }
    }

    restartAgent() {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            throw new Error('WebSocket 未连接');
        }
        this.opts.statusEl.textContent = '正在重启…';
        setHeaderConnLive(this.opts.statusEl, false);
        this.ws.send(JSON.stringify({op: 'restart_agent'}));
    }

    canSendChat() {
        return (
            this.panelReady &&
            this.ws !== null &&
            this.ws.readyState === WebSocket.OPEN
        );
    }
}
