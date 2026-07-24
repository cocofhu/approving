import {renderMermaidInLightboxMount} from '../core/md.js';
import {apiPath} from '../core/paths.js';
import {ChatView} from '../ui/chat_view.js';
import {createQueuePanelUpdater, queueStateHasPendingWork} from '../ui/queue_panel.js';
import {BrowserSession, setHeaderConnLive} from '../ws/session.js';

const logEl = document.getElementById('log');
const scrollEl = document.getElementById('chatScroll');
const statusEl = document.getElementById('status');
const inputEl = document.getElementById('input');
const btnSend = document.getElementById('btnSend');
const btnCancel = document.getElementById('btnCancel');
const btnRestartAgent = document.getElementById('btnRestartAgent');
const queuePanel = document.getElementById('queuePanel');
const btnModelPicker = document.getElementById('btnModelPicker');
const modelLabel = document.getElementById('modelLabel');
const modelModal = document.getElementById('modelModal');
const modelModalBackdrop = document.getElementById('modelModalBackdrop');
const modelSearch = document.getElementById('modelSearch');
const modelList = document.getElementById('modelList');
const modelModalClose = document.getElementById('modelModalClose');
const btnTheme = document.getElementById('btnTheme');
const btnAttach = document.getElementById('btnAttach');
const fileInput = document.getElementById('fileInput');
const attachPreview = document.getElementById('attachPreview');
const btnQQConfig = document.getElementById('btnQQConfig');
const qqDrawer = document.getElementById('qqDrawer');
const qqDrawerBackdrop = document.getElementById('qqDrawerBackdrop');
const qqDrawerClose = document.getElementById('qqDrawerClose');
const qqConfigForm = document.getElementById('qqConfigForm');
const qqConfigStatus = document.getElementById('qqConfigStatus');
const qqConnectionStatus = document.getElementById('qqConnectionStatus');
const html = document.documentElement;

const THEME_STORAGE_KEY = 'acp-bridge-theme';

const domOk =
    statusEl && logEl && scrollEl && inputEl && btnSend && btnCancel;

if (!domOk) {
    console.error('acp-bridge: 页面缺少必要节点，请确认通过本服务打开（勿用过期缓存的 HTML）', {
        statusEl: !!statusEl,
        logEl: !!logEl,
        scrollEl: !!scrollEl,
        inputEl: !!inputEl,
        btnSend: !!btnSend,
        btnCancel: !!btnCancel,
    });
    if (statusEl) {
        statusEl.textContent =
            '页面不完整（缺少按钮或输入框）。请强制刷新 (Ctrl+Shift+R) 或用 http://本机:8765 打开，勿用旧的离线 HTML。';
        setHeaderConnLive(statusEl, false);
    }
}

/** 未在 localStorage 固定主题时，跟随系统明暗 */
function bindSystemThemeListener() {
    try {
        if (localStorage.getItem(THEME_STORAGE_KEY)) return;
    } catch (_) {
        return;
    }
    const mq = matchMedia('(prefers-color-scheme: light)');
    mq.addEventListener('change', () => {
        try {
            if (localStorage.getItem(THEME_STORAGE_KEY)) return;
        } catch (_) {
            return;
        }
        html.setAttribute('data-theme', mq.matches ? 'light' : 'dark');
    });
}

const chat = domOk ? new ChatView(logEl, scrollEl) : null;

/** 与会话就绪（可发消息）一致，来自 BrowserSession onConnected */
let sessionLive = false;
/** 最近一次 connected / queue_state 中的队列字段，用于顶栏停止按钮是否显示 */
let lastQueuePayload = null;

const updateQueuePanel = createQueuePanelUpdater(queuePanel);

function syncStopBtnVisibility() {
    if (!btnCancel) return;
    const show =
        sessionLive &&
        lastQueuePayload != null &&
        queueStateHasPendingWork(lastQueuePayload);
    btnCancel.classList.toggle('cc-stop-btn--visible', show);
}

function setConnected(connected) {
    sessionLive = connected;
    if (!connected) lastQueuePayload = null;
    syncStopBtnVisibility();
    syncSendState();
    if (connected) {
        loadModelList();
    }
}

function setRestartAvailable(available) {
    if (btnRestartAgent) btnRestartAgent.disabled = !available;
}

let _allModels = [];
let _currentModelId = '';
let _modelFixed = false;

function setModelInfo(_model, currentModel) {
    if (currentModel) {
        _currentModelId = currentModel;
        updateModelLabel();
    }
}

function updateModelLabel() {
    if (!modelLabel) return;
    if (!_currentModelId || _currentModelId === 'auto') {
        modelLabel.textContent = 'Auto';
    } else {
        const found = _allModels.find(m => m.id === _currentModelId);
        modelLabel.textContent = found ? found.name : _currentModelId;
    }
}

let _modelListLoaded = false;
async function loadModelList() {
    if (_modelListLoaded && _allModels.length) return;
    try {
        const res = await fetch(apiPath('api/models'), {credentials: 'include', headers: {'Accept': 'application/json'}});
        if (!res.ok) {
            console.warn('acp-bridge: /api/models 响应异常', res.status);
            return;
        }
        const data = await res.json();
        _allModels = data.models || [];
        if (data.current) _currentModelId = data.current;
        _modelFixed = !!data.fixed;
        updateModelLabel();
        _modelListLoaded = true;
        if (btnModelPicker) {
            btnModelPicker.disabled = _modelFixed;
            if (_modelFixed) btnModelPicker.title = '模型已由启动参数锁定';
        }
    } catch (e) {
        console.warn('acp-bridge: 加载模型列表失败', e);
    }
}

async function openModelModal() {
    if (!modelModal || !modelList) return;
    modelModal.hidden = false;
    if (modelModalBackdrop) modelModalBackdrop.hidden = false;
    if (modelSearch) { modelSearch.value = ''; modelSearch.focus(); }
    if (!_allModels.length) {
        modelList.innerHTML = '<div class="cc-model-empty">加载中…</div>';
        await loadModelList();
    }
    renderModelList('');
}

function closeModelModal() {
    if (modelModal) modelModal.hidden = true;
    if (modelModalBackdrop) modelModalBackdrop.hidden = true;
}

function renderModelList(filter) {
    if (!modelList) return;
    const q = filter.toLowerCase().trim();
    const filtered = q
        ? _allModels.filter(m => m.id.toLowerCase().includes(q) || (m.name || '').toLowerCase().includes(q))
        : _allModels;
    if (!filtered.length) {
        modelList.innerHTML = '<div class="cc-model-empty">无匹配模型</div>';
        return;
    }
    const frag = document.createDocumentFragment();
    for (const m of filtered) {
        const el = document.createElement('div');
        el.className = 'cc-model-item' + (m.id === _currentModelId ? ' active' : '');
        el.dataset.id = m.id;
        let html = `<span>${m.name || m.id}</span>`;
        if (m.isDefault) html += `<span class="cc-model-item-default">default</span>`;
        if (m.id !== m.name) html += `<span class="cc-model-item-id">${m.id}</span>`;
        el.innerHTML = html;
        el.addEventListener('click', () => selectModel(m.id));
        frag.appendChild(el);
    }
    modelList.innerHTML = '';
    modelList.appendChild(frag);
}

async function selectModel(modelId) {
    closeModelModal();
    if (btnModelPicker) btnModelPicker.disabled = true;
    if (statusEl) {
        statusEl.textContent = '正在切换模型…';
        setHeaderConnLive(statusEl, false);
    }
    try {
        const res = await fetch(apiPath('api/model'), {
            method: 'POST',
            credentials: 'include',
            headers: {'Content-Type': 'application/json', 'Accept': 'application/json'},
            body: JSON.stringify({model: modelId}),
        });
        if (!res.ok) {
            const err = await res.text();
            throw new Error(err);
        }
        _currentModelId = modelId;
        updateModelLabel();
    } catch (e) {
        console.error('acp-bridge: 切换模型失败', e);
        if (statusEl) statusEl.textContent = '切换失败: ' + (e.message || String(e));
    } finally {
        if (btnModelPicker && !_modelFixed) btnModelPicker.disabled = false;
    }
}

if (btnModelPicker) btnModelPicker.addEventListener('click', openModelModal);
if (modelModalClose) modelModalClose.addEventListener('click', closeModelModal);
if (modelModalBackdrop) modelModalBackdrop.addEventListener('click', closeModelModal);
if (modelSearch) modelSearch.addEventListener('input', () => renderModelList(modelSearch.value));
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modelModal && !modelModal.hidden) closeModelModal();
});

const onQueueState = (m) => {
    updateQueuePanel(m);
    lastQueuePayload = m && typeof m === 'object' ? m : null;
    syncStopBtnVisibility();
};

const session = domOk
    ? new BrowserSession({
        statusEl,
        chat,
        getCwd: () => '',
        getAutoPerm: () => true,
        onConnected: setConnected,
        onRestartAvailable: setRestartAvailable,
        onQueueState,
        onModelUpdate: setModelInfo,
    })
    : null;

function syncSendState() {
    if (!btnSend || !inputEl) return;
    const hasText = inputEl.value.trim().length > 0;
    const hasFiles = pendingFiles.length > 0;
    btnSend.disabled = !(hasText || hasFiles) || !session?.canSendChat();
}

if (btnTheme) {
    btnTheme.addEventListener('click', () => {
        const next = html.getAttribute('data-theme') === 'light' ? 'dark' : 'light';
        html.setAttribute('data-theme', next);
        try {
            localStorage.setItem(THEME_STORAGE_KEY, next);
        } catch (_) {
        }
    });
}

/* ── QQ 机器人配置弹窗 ── */
function setQQConfigStatus(text, kind = '') {
    if (!qqConfigStatus) return;
    qqConfigStatus.textContent = text || '';
    qqConfigStatus.classList.toggle('cc-qq-config-status--error', kind === 'error');
    qqConfigStatus.classList.toggle('cc-qq-config-status--ok', kind === 'ok');
}

function setQQConnectionStatus(cfg) {
    if (!qqConnectionStatus) return;
    const text = cfg?.status || '未检测';
    qqConnectionStatus.textContent = text;
    qqConnectionStatus.title = text;
    qqConnectionStatus.classList.toggle('cc-qq-status--ok', !!cfg?.connected);
    qqConnectionStatus.classList.toggle('cc-qq-status--error', !!cfg?.enabled && !cfg?.connected);
}

function splitLines(value) {
    return String(value || '')
        .split(/\r?\n/)
        .map((s) => s.trim())
        .filter(Boolean);
}

function joinLines(value) {
    return Array.isArray(value) ? value.join('\n') : '';
}

async function loadQQConfig() {
    if (!qqConfigForm) return;
    setQQConfigStatus('正在加载配置…');
    const res = await fetch(apiPath('api/qq/config'), {headers: {'Accept': 'application/json'}});
    if (!res.ok) throw new Error(await res.text());
    const cfg = await res.json();
    document.getElementById('qqEnabled').checked = !!cfg.enabled;
    document.getElementById('qqAppId').value = cfg.appId || '';
    document.getElementById('qqAppSecret').value = '';
    document.getElementById('qqAppSecret').placeholder = cfg.appSecretSet ? '已保存，留空表示不修改' : 'QQ 机器人 AppSecret';
    document.getElementById('qqAllowOpenIds').value = joinLines(cfg.allowOpenIds);
    document.getElementById('qqAllowGroupOpenIds').value = joinLines(cfg.allowGroupOpenIds);
    setQQConnectionStatus(cfg);
    setQQConfigStatus('配置已加载', 'ok');
}

function openQQDrawer() {
    if (!qqDrawer || !qqDrawerBackdrop) return;
    qqDrawer.hidden = false;
    qqDrawerBackdrop.hidden = false;
    loadQQConfig().catch((err) => setQQConfigStatus(err.message || String(err), 'error'));
}

function closeQQDrawer() {
    if (qqDrawer) qqDrawer.hidden = true;
    if (qqDrawerBackdrop) qqDrawerBackdrop.hidden = true;
}

if (btnQQConfig) btnQQConfig.addEventListener('click', openQQDrawer);
if (qqDrawerClose) qqDrawerClose.addEventListener('click', closeQQDrawer);
if (qqDrawerBackdrop) qqDrawerBackdrop.addEventListener('click', closeQQDrawer);

let _qqPollTimer = null;

function pollQQStatus(maxAttempts = 10, interval = 2000) {
    let attempts = 0;
    if (_qqPollTimer) clearInterval(_qqPollTimer);
    _qqPollTimer = setInterval(async () => {
        attempts++;
        try {
            const res = await fetch(apiPath('api/qq/config'), {headers: {'Accept': 'application/json'}});
            if (!res.ok) return;
            const cfg = await res.json();
            setQQConnectionStatus(cfg);
            if (cfg.connected || !cfg.enabled || attempts >= maxAttempts) {
                clearInterval(_qqPollTimer);
                _qqPollTimer = null;
                if (cfg.connected) setQQConfigStatus('已连接', 'ok');
            }
        } catch (_) {
            // ignore
        }
    }, interval);
}

if (qqConfigForm) {
    qqConfigForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        try {
            setQQConfigStatus('正在保存…');
            const payload = {
                enabled: document.getElementById('qqEnabled').checked,
                appId: document.getElementById('qqAppId').value.trim(),
                appSecret: document.getElementById('qqAppSecret').value.trim(),
                allowOpenIds: splitLines(document.getElementById('qqAllowOpenIds').value),
                allowGroupOpenIds: splitLines(document.getElementById('qqAllowGroupOpenIds').value),
            };
            const res = await fetch(apiPath('api/qq/config'), {
                method: 'POST',
                headers: {'Content-Type': 'application/json', 'Accept': 'application/json'},
                body: JSON.stringify(payload),
            });
            if (!res.ok) throw new Error(await res.text());
            const cfg = await res.json();
            document.getElementById('qqAppSecret').value = '';
            document.getElementById('qqAppSecret').placeholder = cfg.appSecretSet ? '已保存，留空表示不修改' : 'QQ 机器人 AppSecret';
            setQQConnectionStatus(cfg);
            setQQConfigStatus('配置已保存', 'ok');
            if (cfg.enabled && !cfg.connected) {
                pollQQStatus();
            }
        } catch (err) {
            setQQConfigStatus(err.message || String(err), 'error');
        }
    });
}

/* ── 切换工具调用 / 思考显示 ── */
const DETAIL_STORAGE_KEY = 'acp-bridge-hide-detail';
const btnToggleDetail = document.getElementById('btnToggleDetail');

/** 从 localStorage 恢复上次的隐藏状态 */
try {
    if (localStorage.getItem(DETAIL_STORAGE_KEY) === '1') {
        document.body.classList.add('cc-hide-detail');
    }
} catch (_) {
}

if (btnToggleDetail) {
    btnToggleDetail.addEventListener('click', () => {
        const hidden = document.body.classList.toggle('cc-hide-detail');
        try {
            localStorage.setItem(DETAIL_STORAGE_KEY, hidden ? '1' : '0');
        } catch (_) {
        }
    });
}

/* ── 附件管理 ── */

/** @type {{ file: File, dataURL?: string }[]} */
const pendingFiles = [];
const MAX_ATTACH = 10;
const IMAGE_MIME_RE = /^image\/(png|jpe?g|gif|webp|svg\+xml|bmp)$/i;

/** 将 File 读为 base64 data URL */
function readAsDataURL(file) {
    return new Promise((resolve, reject) => {
        const r = new FileReader();
        r.onload = () => resolve(r.result);
        r.onerror = () => reject(r.error);
        r.readAsDataURL(file);
    });
}

/** 添加文件到待发送列表 */
async function addFiles(files) {
    for (const f of files) {
        if (pendingFiles.length >= MAX_ATTACH) break;
        const entry = {file: f};
        if (IMAGE_MIME_RE.test(f.type)) {
            try {
                entry.dataURL = await readAsDataURL(f);
            } catch (_) {
            }
        }
        pendingFiles.push(entry);
    }
    renderAttachPreview();
    syncSendState();
}

function removeFile(idx) {
    pendingFiles.splice(idx, 1);
    renderAttachPreview();
    syncSendState();
}

function clearFiles() {
    pendingFiles.length = 0;
    renderAttachPreview();
}

function renderAttachPreview() {
    if (!attachPreview) return;
    attachPreview.innerHTML = '';
    if (pendingFiles.length === 0) {
        attachPreview.hidden = true;
        return;
    }
    attachPreview.hidden = false;
    pendingFiles.forEach((entry, i) => {
        const item = document.createElement('div');
        item.className = 'cc-attach-item';
        if (entry.dataURL) {
            const img = document.createElement('img');
            img.src = entry.dataURL;
            img.alt = entry.file.name;
            item.appendChild(img);
        } else {
            const fd = document.createElement('div');
            fd.className = 'cc-attach-file';
            const icon = document.createElement('span');
            icon.className = 'cc-attach-file-icon';
            icon.textContent = '📄';
            const name = document.createElement('span');
            name.className = 'cc-attach-file-name';
            name.textContent = entry.file.name;
            name.title = entry.file.name;
            fd.appendChild(icon);
            fd.appendChild(name);
            item.appendChild(fd);
        }
        const rm = document.createElement('button');
        rm.className = 'cc-attach-remove';
        rm.textContent = '✕';
        rm.title = '移除';
        rm.onclick = () => removeFile(i);
        item.appendChild(rm);
        attachPreview.appendChild(item);
    });
}

/** 从 pendingFiles 构建 images 数组（base64），用于 WS 发送 */
async function buildImages() {
    const images = [];
    for (const entry of pendingFiles) {
        const dataURL = entry.dataURL || await readAsDataURL(entry.file);
        // dataURL 格式: data:mime;base64,xxxx
        const commaIdx = dataURL.indexOf(',');
        const meta = dataURL.substring(0, commaIdx); // data:image/png;base64
        const base64 = dataURL.substring(commaIdx + 1);
        const mimeMatch = meta.match(/^data:([^;]+)/);
        const mimeType = mimeMatch ? mimeMatch[1] : 'application/octet-stream';
        images.push({data: base64, mimeType, name: entry.file.name});
    }
    return images;
}

if (domOk && btnSend && inputEl) {
    btnSend.addEventListener('click', async () => {
        try {
            const t = inputEl.value.trim();
            if (!t && pendingFiles.length === 0) return;
            chat.endStream();
            let images = [];
            /* 图片用 dataURL 预览；非图片文件在气泡里显示文件名 */
            const imageURLs = pendingFiles
                .filter((e) => e.dataURL)
                .map((e) => e.dataURL);
            const fileLabels = pendingFiles
                .filter((e) => !e.dataURL)
                .map((e) => `📎 ${e.file.name}`);
            const displayText = [t, ...fileLabels].filter(Boolean).join('\n');
            if (pendingFiles.length > 0) {
                images = await buildImages();
            }
            chat.enqueueUserMessage(displayText, imageURLs);
            session.sendChat(t, images);
            inputEl.value = '';
            clearFiles();
            inputEl.classList.remove('cc-textarea--expanded');
            autoGrow(inputEl);
            syncSendState();
        } catch (e) {
            alert(e.message || String(e));
        }
    });

    btnCancel.addEventListener('click', () => session.cancel());

    if (btnRestartAgent) {
        btnRestartAgent.addEventListener('click', () => {
            try {
                session.restartAgent();
            } catch (e) {
                alert(e.message || String(e));
            }
        });
    }

    inputEl.addEventListener('input', () => {
        if (!inputEl.value.includes('\n')) {
            inputEl.classList.remove('cc-textarea--expanded');
        }
        autoGrow(inputEl);
        syncSendState();
    });

    inputEl.addEventListener('keydown', (e) => {
        if (e.key !== 'Enter') return;
        /* Ctrl/⌘+Enter：换行并允许出现滚动条；其余 Enter：发送 */
        if (e.ctrlKey || e.metaKey) {
            e.preventDefault();
            const start = inputEl.selectionStart ?? inputEl.value.length;
            const end = inputEl.selectionEnd ?? start;
            const v = inputEl.value;
            inputEl.value = v.slice(0, start) + '\n' + v.slice(end);
            const pos = start + 1;
            inputEl.selectionStart = pos;
            inputEl.selectionEnd = pos;
            inputEl.classList.add('cc-textarea--expanded');
            autoGrow(inputEl);
            syncSendState();
            return;
        }
        e.preventDefault();
        if (!btnSend.disabled) btnSend.click();
    });

    /* 粘贴图片 */
    inputEl.addEventListener('paste', (e) => {
        const items = e.clipboardData?.items;
        if (!items) return;
        const files = [];
        for (const item of items) {
            if (item.kind === 'file') {
                const f = item.getAsFile();
                if (f) files.push(f);
            }
        }
        if (files.length > 0) {
            e.preventDefault();
            addFiles(files).catch((err) => console.warn('acp-bridge: 粘贴添加文件失败', err));
        }
    });

    /* 拖拽文件 */
    const inputBox = inputEl.closest('.cc-input-box');
    if (inputBox) {
        inputBox.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'copy';
        });
        inputBox.addEventListener('drop', (e) => {
            e.preventDefault();
            if (e.dataTransfer?.files?.length) addFiles(Array.from(e.dataTransfer.files)).catch((err) => console.warn('acp-bridge: 拖拽添加文件失败', err));
        });
    }
}

/* 附件按钮 */
if (btnAttach && fileInput) {
    btnAttach.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', () => {
        if (fileInput.files?.length) {
            addFiles(Array.from(fileInput.files)).catch((err) => console.warn('acp-bridge: 文件选择添加失败', err));
            fileInput.value = '';
        }
    });
}

/* ── 图片预览 Lightbox ── */
const imgLightbox = document.getElementById('imgLightbox');
const lightboxImg = document.getElementById('lightboxImg');
const lightboxClose = document.getElementById('lightboxClose');

function openLightbox(src) {
    if (!imgLightbox || !lightboxImg) return;
    lightboxImg.src = src;
    imgLightbox.hidden = false;
}

function closeLightbox() {
    if (!imgLightbox) return;
    imgLightbox.hidden = true;
    if (lightboxImg) lightboxImg.src = '';
}

if (imgLightbox) {
    imgLightbox.querySelector('.cc-lightbox-backdrop')?.addEventListener('click', closeLightbox);
    if (lightboxClose) lightboxClose.addEventListener('click', closeLightbox);
}

/* ── Mermaid 图表放大 ── */
const mermaidLightbox = document.getElementById('mermaidLightbox');
const mermaidLightboxInner = document.getElementById('mermaidLightboxInner');
const mermaidLightboxClose = document.getElementById('mermaidLightboxClose');
/** 避免快速连点两次异步渲染交错 */
let mermaidLightboxSeq = 0;

/**
 * @param {HTMLElement} source
 */
function cloneMermaidForLightbox(source) {
    const wrap = source.cloneNode(true);
    wrap.classList.remove('cc-mermaid--interactive');
    wrap.removeAttribute('tabindex');
    wrap.removeAttribute('role');
    wrap.removeAttribute('aria-label');

    /*
     * Mermaid SVG 内部 <style> 使用 #id 选择器引用节点。
     * 直接克隆会导致页面上出现重复 id，浏览器 CSS 只命中第一个（页面内的原始 SVG），
     * 克隆图因此丢失样式而显示空白。
     * 解决方案：给克隆 SVG 内所有 id 加唯一后缀，并同步更新 <style> 和属性中的引用。
     */
    const suffix = '_lb' + Date.now().toString(36);
    const svg = wrap.querySelector('svg');
    if (svg) {
        /* 收集所有带 id 的元素 */
        const idEls = svg.querySelectorAll('[id]');
        const idMap = new Map();
        for (const el of idEls) {
            const oldId = el.id;
            if (!oldId) continue;
            const newId = oldId + suffix;
            idMap.set(oldId, newId);
            el.id = newId;
        }
        /* 更新 <style> 标签中的 #id 引用 */
        for (const style of svg.querySelectorAll('style')) {
            let css = style.textContent || '';
            for (const [oldId, newId] of idMap) {
                /* 替换 #oldId（后跟非字母数字下划线横线的字符或字符串结尾） */
                css = css.replaceAll('#' + oldId, '#' + newId);
            }
            style.textContent = css;
        }
        /* 更新 url(#id) 引用（fill、clip-path、marker-end 等属性） */
        const urlRefAttrs = ['fill', 'stroke', 'clip-path', 'marker-start', 'marker-mid', 'marker-end', 'filter', 'mask'];
        for (const el of svg.querySelectorAll('*')) {
            for (const attr of urlRefAttrs) {
                const val = el.getAttribute(attr);
                if (!val || !val.includes('url(#')) continue;
                let newVal = val;
                for (const [oldId, newId] of idMap) {
                    newVal = newVal.replaceAll('url(#' + oldId + ')', 'url(#' + newId + ')');
                }
                if (newVal !== val) el.setAttribute(attr, newVal);
            }
            /* href / xlink:href 引用 */
            for (const hrefAttr of ['href', 'xlink:href']) {
                const hval = el.getAttribute(hrefAttr);
                if (!hval || !hval.startsWith('#')) continue;
                const refId = hval.slice(1);
                if (idMap.has(refId)) {
                    el.setAttribute(hrefAttr, '#' + idMap.get(refId));
                }
            }
        }
    }
    return wrap;
}

async function openMermaidLightbox(source) {
    if (!mermaidLightbox || !mermaidLightboxInner || !source) return;
    const seq = ++mermaidLightboxSeq;
    mermaidLightboxInner.textContent = '正在渲染图表…';
    mermaidLightbox.hidden = false;
    let ok = false;
    try {
        ok = await renderMermaidInLightboxMount(mermaidLightboxInner, source);
    } catch (e) {
        console.warn('acp-bridge: mermaid lightbox render failed, falling back to clone', e);
    }
    if (seq !== mermaidLightboxSeq) return;
    if (!ok) {
        mermaidLightboxInner.textContent = '';
        mermaidLightboxInner.appendChild(cloneMermaidForLightbox(source));
    }
}

function closeMermaidLightbox() {
    if (!mermaidLightbox || !mermaidLightboxInner) return;
    mermaidLightbox.hidden = true;
    mermaidLightboxInner.textContent = '';
}

if (mermaidLightbox) {
    mermaidLightbox.querySelector('.cc-mermaid-lightbox-backdrop')?.addEventListener('click', closeMermaidLightbox);
    if (mermaidLightboxClose) mermaidLightboxClose.addEventListener('click', closeMermaidLightbox);
}

document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    if (qqDrawer && !qqDrawer.hidden) {
        closeQQDrawer();
        return;
    }
    if (mermaidLightbox && !mermaidLightbox.hidden) {
        closeMermaidLightbox();
        return;
    }
    if (imgLightbox && !imgLightbox.hidden) closeLightbox();
});

/* 事件委托：用户图片预览 · Mermaid 放大 */
if (logEl) {
    logEl.addEventListener('click', (e) => {
        const mer = e.target.closest('.cc-mermaid--interactive');
        if (mer) {
            openMermaidLightbox(mer);
            return;
        }
        const img = e.target.closest('.cc-user-img');
        if (img && img.src) openLightbox(img.src);
    });
    logEl.addEventListener('keydown', (e) => {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        const mer = e.target.closest('.cc-mermaid--interactive');
        if (!mer) return;
        e.preventDefault();
        openMermaidLightbox(mer);
    });
}

function autoGrow(el) {
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
}

bindSystemThemeListener();

window.addEventListener('pagehide', () => {
    try {
        chat?.flushPersist();
    } catch (_) {
    }
});
window.addEventListener('beforeunload', () => {
    try {
        chat?.flushPersist();
    } catch (_) {
    }
});

document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
        try {
            chat?.flushPersist();
        } catch (_) {
        }
    }
});

if (domOk && session) {
    setConnected(false);
    syncSendState();
    try {
        session.connect();
    } catch (e) {
        console.error(e);
        if (statusEl) {
            statusEl.textContent = '脚本异常，无法连接：' + (e && e.message ? e.message : String(e));
            setHeaderConnLive(statusEl, false);
        }
    }
}