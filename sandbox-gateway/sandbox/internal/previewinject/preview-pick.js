/* Approving IP-direct preview cooperative script. Runs in the app origin. */
(function () {
  var READY = 'direct-preview-ready';
  var URL_MSG = 'direct-preview-url';
  var PICKED = 'direct-preview-picked';
  var CANCELED = 'direct-preview-canceled';
  var INSPECT = 'direct-preview-inspect';
  var NAV = 'direct-preview-nav';
  var PING = 'direct-preview-ping';
  var enabled = false;
  var hoverEl = null;
  var styleEl = null;

  function post(msg) {
    // Always '*' : HTTPS Approving embedding http://IP:port often strips
    // document.referrer (strict-origin-when-cross-origin), and a wrong
    // targetOrigin fails silently with no exception.
    try {
      parent.postMessage(msg, '*');
    } catch (e) {}
  }

  function currentUrl() {
    try {
      return location.href;
    } catch (e) {
      return '';
    }
  }

  function postUrl(type) {
    var u = currentUrl();
    if (!u) return;
    post({ type: type || URL_MSG, url: u });
  }

  function escId(id) {
    if (typeof CSS !== 'undefined' && CSS.escape) return CSS.escape(id);
    return String(id).replace(/([ !"#$%&'()*+,./:;<=>?@[\\\]^`{|}~])/g, '\\$1');
  }

  function seg(e) {
    if (e.id) return '#' + escId(e.id);
    var s = e.tagName.toLowerCase();
    var p = e.parentElement;
    if (!p) return s;
    var same = Array.prototype.filter.call(p.children, function (c) {
      return c.tagName === e.tagName;
    });
    if (same.length > 1) s += ':nth-of-type(' + (same.indexOf(e) + 1) + ')';
    return s;
  }

  function path(e) {
    var parts = [];
    while (e && e.nodeType === 1 && e.tagName.toLowerCase() !== 'html') {
      var s = seg(e);
      parts.unshift(s);
      if (s.charAt(0) === '#') break;
      e = e.parentElement;
    }
    return parts.join(' > ');
  }

  function ensureStyle() {
    if (styleEl) return;
    styleEl = document.createElement('style');
    styleEl.textContent =
      '.__hp-inspect-hover{outline:2px solid #3b82f6!important;outline-offset:1px!important;cursor:crosshair!important;}' +
      'html.__hp-inspecting,html.__hp-inspecting *{cursor:crosshair!important;}';
    (document.head || document.documentElement).appendChild(styleEl);
  }

  function clearHover() {
    if (hoverEl) {
      hoverEl.classList.remove('__hp-inspect-hover');
      hoverEl = null;
    }
  }

  function setEnabled(on) {
    enabled = !!on;
    ensureStyle();
    clearHover();
    var root = document.documentElement;
    if (root) {
      if (enabled) root.classList.add('__hp-inspecting');
      else root.classList.remove('__hp-inspecting');
    }
  }

  function onMove(ev) {
    if (!enabled) return;
    var t = ev.target;
    if (!t || t === hoverEl || t === styleEl) return;
    if (t.nodeType !== 1) return;
    clearHover();
    hoverEl = t;
    hoverEl.classList.add('__hp-inspect-hover');
  }

  function onClick(ev) {
    if (!enabled) return;
    ev.preventDefault();
    ev.stopPropagation();
    var t = ev.target;
    if (!t || t.nodeType !== 1) return;
    var selector = path(t);
    var tagName = t.tagName.toLowerCase();
    var html = '';
    try {
      html = t.outerHTML || '';
    } catch (e) {}
    clearHover();
    setEnabled(false);
    post({
      type: PICKED,
      selector: selector,
      tagName: tagName,
      outerHTML: html,
      url: currentUrl(),
    });
  }

  function onKeydown(ev) {
    if (!enabled) return;
    if (ev.key !== 'Escape' && ev.key !== 'Esc') return;
    ev.preventDefault();
    ev.stopPropagation();
    setEnabled(false);
    post({ type: CANCELED });
  }

  window.addEventListener('message', function (ev) {
    var data = ev.data;
    if (!data || typeof data !== 'object') return;
    if (data.type === PING) {
      postUrl(READY);
      return;
    }
    if (data.type === INSPECT) {
      setEnabled(!!data.on);
      return;
    }
    if (data.type === NAV) {
      var action = data.action;
      try {
        if (action === 'back') history.back();
        else if (action === 'forward') history.forward();
        else if (action === 'reload') location.reload();
      } catch (e) {}
    }
  });

  document.addEventListener('mousemove', onMove, true);
  document.addEventListener('click', onClick, true);
  document.addEventListener('keydown', onKeydown, true);
  window.addEventListener('popstate', function () {
    postUrl();
  });
  window.addEventListener('hashchange', function () {
    postUrl();
  });

  try {
    var push = history.pushState;
    history.pushState = function () {
      var r = push.apply(this, arguments);
      postUrl();
      return r;
    };
    var replace = history.replaceState;
    history.replaceState = function () {
      var r = replace.apply(this, arguments);
      postUrl();
      return r;
    };
  } catch (e) {}

  postUrl(READY);
  // Parent may arm its wait on iframe "load", which fires after this script.
  // Re-announce so a late listener still clears the missing-script tip.
  if (document.readyState === 'complete') {
    postUrl(READY);
  } else {
    window.addEventListener('load', function () {
      postUrl(READY);
    });
  }
})();
