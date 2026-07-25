/**
 * Docs/site locale: approving-locale (zh-CN | en).
 * Priority: localStorage > navigator (zh* → zh-CN, en* → en, else zh-CN).
 * Home entry (/ and /en/) may redirect once; deep links never auto-rewrite.
 */
(() => {
  const STORAGE_KEY = "approving-locale";

  function getSaved() {
    try {
      const v = localStorage.getItem(STORAGE_KEY);
      if (v === "zh-CN" || v === "en") return v;
    } catch {
      /* private mode / opaque origin */
    }
    return null;
  }

  function setSaved(loc) {
    if (loc !== "zh-CN" && loc !== "en") return;
    try {
      localStorage.setItem(STORAGE_KEY, loc);
    } catch {
      /* ignore */
    }
  }

  function fromNavigator() {
    const lang = (navigator.language || "zh-CN").toLowerCase();
    if (lang.startsWith("zh")) return "zh-CN";
    if (lang.startsWith("en")) return "en";
    return "zh-CN";
  }

  function pageLocale() {
    const lang = (document.documentElement.lang || "zh-CN").toLowerCase();
    if (lang === "en" || lang.startsWith("en-")) return "en";
    return "zh-CN";
  }

  function wireSwitcher() {
    document.querySelectorAll("[data-locale-set]").forEach((el) => {
      el.addEventListener("click", () => {
        const loc = el.getAttribute("data-locale-set");
        if (loc === "zh-CN" || loc === "en") setSaved(loc);
      });
    });
  }

  /**
   * Home entry matrix (at most one redirect):
   * - saved en on / → /en/
   * - saved zh-CN on /en/ → /
   * - no saved: en* on / → /en/; zh* or other stay on /
   * - no saved on /en/: stay (do not bounce zh* away)
   *
   * Use string-literal destinations only (not DOM-derived data-base).
   * Avoids CodeQL "DOM text reinterpreted as HTML" on location.replace;
   * custom-domain docs are served at site root (BASE=/).
   */
  function maybeRedirectHome() {
    if (document.documentElement.getAttribute("data-home-entry") !== "1") return;

    const page = pageLocale();
    const saved = getSaved();

    if (saved === "en" && page === "zh-CN") {
      location.replace("/en/");
      return;
    }
    if (saved === "zh-CN" && page === "en") {
      location.replace("/");
      return;
    }
    if (!saved && page === "zh-CN" && fromNavigator() === "en") {
      location.replace("/en/");
    }
  }

  // Script loads in <head>; bind switcher after [data-locale-set] exists in body.
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", wireSwitcher);
  } else {
    wireSwitcher();
  }
  maybeRedirectHome();
})();
