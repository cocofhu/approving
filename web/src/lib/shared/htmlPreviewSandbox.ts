/**
 * Safe sandbox for srcdoc preview iframes — scripts + forms only.
 *
 * Deliberately omits allow-same-origin: when combined with allow-scripts on a
 * same-origin frame (including srcdoc), the embedded document can remove its
 * own sandbox and reach the parent DOM/storage — a sandbox escape. Keep this
 * locked to the token list below.
 *
 * Clarify demoHtml / visual page.html and other srcdoc content must not depend
 * on Web Storage (localStorage/sessionStorage) or cookie-backed same-origin
 * state; need a full SPA or persistence → app_preview (noVNC).
 */
export const SANDBOX_ATTR = 'allow-scripts allow-forms'

/**
 * Generate a page-unique instance id for HtmlPreview inline resize messaging.
 * Uses crypto.randomUUID when available (Secure Context); otherwise falls back to
 * a timestamp + random suffix (aligned with session.js degradation pattern).
 * @internal HtmlPreview call chain only — not part of the public API surface.
 */
export function createInstanceId(): string {
  return typeof crypto?.randomUUID === 'function'
    ? crypto.randomUUID()
    : `hp-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`
}

export const INLINE_FALLBACK_HEIGHT = 120

/** Ignore resize postMessage when |Δh| is below this threshold (steady-state anti-jitter). */
export const RESIZE_HEIGHT_EPSILON = 6

/** Debounce subsequent ResizeObserver reports inside the iframe (ms). First report is immediate. */
export const RESIZE_DEBOUNCE_MS = 75

/**
 * Shared cap for fillPreview content-fit preview shells (CSS max-height +
 * HtmlPreview clamp). Short content stays content-sized; only overflow caps.
 */
export const CONTENT_FIT_PREVIEW_MAX_VH = 60

/**
 * Approx height of HtmlPreview default-mode device toolbar
 * (`border-b` + `py-2` control row). Used when measuring is unavailable.
 */
export const HTML_PREVIEW_DEFAULT_TOOLBAR_PX = 37

/**
 * Approx height of GateApproval reviewing-upstream strip
 * (`py-1.5` + `text-[11px]` row). Matches visual AFTER `calc(60vh - 28px)`.
 */
export const CONTENT_FIT_REVIEWING_STRIP_PX = 28

/**
 * Pixel height for a content-fit vh cap against a viewport.
 * Defaults to {@link CONTENT_FIT_PREVIEW_MAX_VH}.
 */
export function contentFitPreviewCapPx(
  viewportHeight = typeof window !== 'undefined' ? window.innerHeight : 0,
  vh = CONTENT_FIT_PREVIEW_MAX_VH,
): number {
  return Math.round(viewportHeight * (vh / 100))
}

export const RESIZE_MESSAGE_TYPE = 'html-preview-resize'

/** Parent → iframe: toggle DOM inspect / pick mode. */
export const INSPECT_COMMAND_TYPE = 'html-preview-inspect-cmd'

/** iframe → parent: element pick with CSS selector + element screenshot. */
export const INSPECT_MESSAGE_TYPE = 'html-preview-inspect'

/** iframe → parent: user canceled inspect (Esc) inside opaque-origin sandbox. */
export const INSPECT_CANCELED_TYPE = 'html-preview-inspect-canceled'

/** Timeout before falling back to fixed inline height when no resize message arrives. */
export const RESIZE_TIMEOUT_MS = 2500

export interface ResizeMessage {
  type: typeof RESIZE_MESSAGE_TYPE
  id: string
  height: number
}

export interface InspectCommandMessage {
  type: typeof INSPECT_COMMAND_TYPE
  id: string
  enabled: boolean
}

export interface InspectPickMessage {
  type: typeof INSPECT_MESSAGE_TYPE
  id: string
  selector: string
  tagName: string
  imageDataUrl: string
  /** Element rect inside the opaque iframe viewport at pick time (for parent pin overlay). */
  bounds?: { left: number; top: number; width: number; height: number }
  /** Truncated visible text at pick time (optional). */
  currentText?: string
}

export interface InspectCanceledMessage {
  type: typeof INSPECT_CANCELED_TYPE
  id: string
}

function escapeForSingleQuotedJs(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/'/g, "\\'")
}

function buildResizeScript(instanceId: string): string {
  const safeId = escapeForSingleQuotedJs(instanceId)
  const debounceMs = RESIZE_DEBOUNCE_MS
  return `<script>(function(){
var instanceId='${safeId}';
var msgType='${RESIZE_MESSAGE_TYPE}';
var debounceMs=${debounceMs};
var debounceTimer=null;
var hasReported=false;
function sendHeight(){
  var docEl=document.documentElement;
  var body=document.body;
  var h=Math.max(docEl?docEl.scrollHeight:0,body?body.scrollHeight:0,0);
  parent.postMessage({type:msgType,id:instanceId,height:h},'*');
  hasReported=true;
}
function report(){
  if(!hasReported){sendHeight();return;}
  if(debounceTimer)clearTimeout(debounceTimer);
  debounceTimer=setTimeout(function(){debounceTimer=null;sendHeight();},debounceMs);
}
if(typeof ResizeObserver!=='undefined'){
  var ro=new ResizeObserver(function(){report();});
  if(document.body)ro.observe(document.body);
  if(document.documentElement)ro.observe(document.documentElement);
}
if(document.readyState==='loading'){
  document.addEventListener('DOMContentLoaded',report);
}else{
  report();
}
window.addEventListener('load',report);
})();<\/script>`
}

/**
 * Inspect script: CSS selector path aligned with server/internal/browser/rod.go
 * pickScript (id / tag:nth-of-type), plus same-document element screenshot via
 * SVG foreignObject → canvas. Parent toggles mode with INSPECT_COMMAND_TYPE.
 */
function buildInspectScript(instanceId: string): string {
  const safeId = escapeForSingleQuotedJs(instanceId)
  return `<script>(function(){
var instanceId='${safeId}';
var cmdType='${INSPECT_COMMAND_TYPE}';
var pickType='${INSPECT_MESSAGE_TYPE}';
var cancelType='${INSPECT_CANCELED_TYPE}';
var enabled=false;
var hoverEl=null;
var styleEl=null;
function escId(id){
  if(typeof CSS!=='undefined'&&CSS.escape)return CSS.escape(id);
  return String(id).replace(/([ !"#$%&'()*+,./:;<=>?@[\\\\\\]^\`{|}~])/g,'\\\\$1');
}
function seg(e){
  if(e.id)return '#'+escId(e.id);
  var s=e.tagName.toLowerCase();
  var p=e.parentElement;
  if(!p)return s;
  var same=Array.prototype.filter.call(p.children,function(c){return c.tagName===e.tagName;});
  if(same.length>1)s+=':nth-of-type('+(same.indexOf(e)+1)+')';
  return s;
}
function path(e){
  var parts=[];
  while(e&&e.nodeType===1&&e.tagName.toLowerCase()!=='html'){
    var s=seg(e);
    parts.unshift(s);
    if(s.charAt(0)==='#')break;
    e=e.parentElement;
  }
  return parts.join(' > ');
}
function ensureStyle(){
  if(styleEl)return;
  styleEl=document.createElement('style');
  styleEl.textContent='.__hp-inspect-hover{outline:2px solid #3b82f6!important;outline-offset:1px!important;cursor:crosshair!important;}html.__hp-inspecting,html.__hp-inspecting *{cursor:crosshair!important;}';
  (document.head||document.documentElement).appendChild(styleEl);
}
function clearHover(){
  if(hoverEl){hoverEl.classList.remove('__hp-inspect-hover');hoverEl=null;}
}
function setEnabled(on){
  enabled=!!on;
  ensureStyle();
  clearHover();
  var root=document.documentElement;
  if(root){
    if(enabled)root.classList.add('__hp-inspecting');
    else root.classList.remove('__hp-inspecting');
  }
}
function inlineClone(el){
  var clone=el.cloneNode(true);
  function walk(src,dst){
    if(src.nodeType!==1||dst.nodeType!==1)return;
    try{
      var cs=window.getComputedStyle(src);
      var parts=[];
      for(var i=0;i<cs.length;i++){
        var prop=cs[i];
        parts.push(prop+':'+cs.getPropertyValue(prop));
      }
      dst.setAttribute('style',parts.join(';'));
    }catch(e){}
    var sc=src.children,dc=dst.children;
    for(var j=0;j<sc.length&&j<dc.length;j++)walk(sc[j],dc[j]);
  }
  walk(el,clone);
  return clone;
}
function captureElement(el){
  return new Promise(function(resolve){
    try{
      var rect=el.getBoundingClientRect();
      var w=Math.max(1,Math.ceil(rect.width));
      var h=Math.max(1,Math.ceil(rect.height));
      var maxSide=2048;
      var scale=Math.min(2,window.devicePixelRatio||1,maxSide/Math.max(w,h,1));
      var cw=Math.max(1,Math.ceil(w*scale));
      var ch=Math.max(1,Math.ceil(h*scale));
      var clone=inlineClone(el);
      var wrapper=document.createElement('div');
      wrapper.setAttribute('xmlns','http://www.w3.org/1999/xhtml');
      wrapper.style.cssText='width:'+w+'px;height:'+h+'px;margin:0;padding:0;box-sizing:border-box;overflow:hidden;background:#fff;';
      wrapper.appendChild(clone);
      var serialized=new XMLSerializer().serializeToString(wrapper);
      var svg='<svg xmlns="http://www.w3.org/2000/svg" width="'+cw+'" height="'+ch+'">'+
        '<foreignObject width="100%" height="100%" transform="scale('+scale+')">'+serialized+'</foreignObject></svg>';
      var url='data:image/svg+xml;charset=utf-8,'+encodeURIComponent(svg);
      var img=new Image();
      img.onload=function(){
        try{
          var canvas=document.createElement('canvas');
          canvas.width=cw;canvas.height=ch;
          var ctx=canvas.getContext('2d');
          if(!ctx){resolve('');return;}
          ctx.fillStyle='#fff';
          ctx.fillRect(0,0,cw,ch);
          ctx.drawImage(img,0,0);
          resolve(canvas.toDataURL('image/png'));
        }catch(e){resolve('');}
      };
      img.onerror=function(){resolve('');};
      img.src=url;
    }catch(e){resolve('');}
  });
}
function onMove(ev){
  if(!enabled)return;
  var t=ev.target;
  if(!t||t===hoverEl||t===styleEl)return;
  if(t.nodeType!==1)return;
  clearHover();
  hoverEl=t;
  hoverEl.classList.add('__hp-inspect-hover');
}
function onClick(ev){
  if(!enabled)return;
  ev.preventDefault();
  ev.stopPropagation();
  var t=ev.target;
  if(!t||t.nodeType!==1)return;
  var selector=path(t);
  var tagName=t.tagName.toLowerCase();
  var rect=t.getBoundingClientRect();
  var bounds={left:rect.left,top:rect.top,width:rect.width,height:rect.height};
  var currentText='';
  try{
    currentText=String(t.innerText||t.textContent||'').replace(/\\s+/g,' ').trim().slice(0,240);
  }catch(e){}
  clearHover();
  // One-shot: leave inspect after pick (parent also clears button state).
  setEnabled(false);
  captureElement(t).then(function(imageDataUrl){
    parent.postMessage({
      type:pickType,
      id:instanceId,
      selector:selector,
      tagName:tagName,
      imageDataUrl:imageDataUrl||'',
      bounds:bounds,
      currentText:currentText
    },'*');
  });
}
function onKeydown(ev){
  if(!enabled)return;
  if(ev.key!=='Escape'&&ev.key!=='Esc')return;
  ev.preventDefault();
  ev.stopPropagation();
  setEnabled(false);
  parent.postMessage({type:cancelType,id:instanceId},'*');
}
window.addEventListener('message',function(ev){
  var data=ev.data;
  if(!data||typeof data!=='object')return;
  if(data.type!==cmdType||data.id!==instanceId)return;
  setEnabled(!!data.enabled);
});
document.addEventListener('mousemove',onMove,true);
document.addEventListener('click',onClick,true);
document.addEventListener('keydown',onKeydown,true);
})();<\/script>`
}

/** Insert an HTML snippet before </body>, after <body>, or as a fallback wrap. */
function injectIntoHtml(html: string, snippet: string): string {
  if (!html) return snippet

  const bodyCloseIdx = html.toLowerCase().indexOf('</body>')
  if (bodyCloseIdx !== -1) {
    return html.slice(0, bodyCloseIdx) + snippet + html.slice(bodyCloseIdx)
  }

  const bodyOpenMatch = html.match(/<body\b[^>]*>/i)
  if (bodyOpenMatch?.index !== undefined) {
    const insertIdx = bodyOpenMatch.index + bodyOpenMatch[0].length
    return html.slice(0, insertIdx) + snippet + html.slice(insertIdx)
  }

  const htmlOpenMatch = html.match(/<html\b[^>]*>/i)
  if (htmlOpenMatch?.index !== undefined) {
    const insertIdx = htmlOpenMatch.index + htmlOpenMatch[0].length
    return html.slice(0, insertIdx) + `<body>${snippet}</body>` + html.slice(insertIdx)
  }

  return snippet + html
}

/** Inject height-reporting script into HTML for inline iframe srcdoc. */
export function injectInlineResizeScript(html: string, instanceId: string): string {
  return injectIntoHtml(html, buildResizeScript(instanceId))
}

/** Inject DOM inspect + element screenshot script for opaque-origin srcdoc. */
export function injectInlineInspectScript(html: string, instanceId: string): string {
  return injectIntoHtml(html, buildInspectScript(instanceId))
}

/** Inject resize and/or inspect scripts into srcdoc HTML. */
export function injectPreviewScripts(
  html: string,
  instanceId: string,
  opts: { resize?: boolean; inspect?: boolean } = {},
): string {
  let out = html || ''
  if (opts.resize) out = injectInlineResizeScript(out, instanceId)
  if (opts.inspect) out = injectInlineInspectScript(out, instanceId)
  return out
}

export function isValidResizeMessage(data: unknown): data is ResizeMessage {
  if (!data || typeof data !== 'object') return false
  const msg = data as Record<string, unknown>
  if (msg.type !== RESIZE_MESSAGE_TYPE) return false
  if (typeof msg.id !== 'string' || !msg.id) return false
  if (typeof msg.height !== 'number' || !Number.isFinite(msg.height) || msg.height < 0) return false
  return true
}

export function parseResizeMessage(data: unknown): ResizeMessage | null {
  if (!isValidResizeMessage(data)) return null
  return {
    type: RESIZE_MESSAGE_TYPE,
    id: data.id,
    height: Math.max(data.height, INLINE_FALLBACK_HEIGHT),
  }
}

export function isValidInspectPickMessage(data: unknown): data is InspectPickMessage {
  if (!data || typeof data !== 'object') return false
  const msg = data as Record<string, unknown>
  if (msg.type !== INSPECT_MESSAGE_TYPE) return false
  if (typeof msg.id !== 'string' || !msg.id) return false
  if (typeof msg.selector !== 'string' || !msg.selector.trim()) return false
  if (typeof msg.tagName !== 'string' || !msg.tagName) return false
  if (typeof msg.imageDataUrl !== 'string') return false
  if (msg.bounds !== undefined) {
    if (!msg.bounds || typeof msg.bounds !== 'object') return false
    const b = msg.bounds as Record<string, unknown>
    for (const k of ['left', 'top', 'width', 'height'] as const) {
      if (typeof b[k] !== 'number' || !Number.isFinite(b[k])) return false
    }
  }
  if (msg.currentText !== undefined && typeof msg.currentText !== 'string') return false
  return true
}

export function parseInspectPickMessage(data: unknown): InspectPickMessage | null {
  if (!isValidInspectPickMessage(data)) return null
  const out: InspectPickMessage = {
    type: INSPECT_MESSAGE_TYPE,
    id: data.id,
    selector: data.selector.trim(),
    tagName: data.tagName,
    imageDataUrl: data.imageDataUrl,
  }
  if (data.bounds) {
    out.bounds = {
      left: data.bounds.left,
      top: data.bounds.top,
      width: data.bounds.width,
      height: data.bounds.height,
    }
  }
  if (typeof data.currentText === 'string' && data.currentText.trim()) {
    out.currentText = data.currentText.trim()
  }
  return out
}

export function isValidInspectCanceledMessage(data: unknown): data is InspectCanceledMessage {
  if (!data || typeof data !== 'object') return false
  const msg = data as Record<string, unknown>
  if (msg.type !== INSPECT_CANCELED_TYPE) return false
  if (typeof msg.id !== 'string' || !msg.id) return false
  return true
}

/** Build parent→iframe inspect command (caller posts to iframe contentWindow). */
export function buildInspectCommand(instanceId: string, enabled: boolean): InspectCommandMessage {
  return { type: INSPECT_COMMAND_TYPE, id: instanceId, enabled }
}

/** Split a data URL into ClarifyImage-shaped { data, mimeType }; empty on failure. */
export function dataUrlToImageParts(dataUrl: string): { data: string; mimeType: string } | null {
  const m = /^data:([^;,]+);base64,(.+)$/i.exec(dataUrl || '')
  if (!m) return null
  const mimeType = m[1].trim() || 'image/png'
  const data = m[2].trim()
  if (!data) return null
  return { data, mimeType }
}
