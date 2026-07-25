/**
 * Docs/site locale: approving-locale (zh-CN | en).
 * Priority: localStorage > navigator (zh*→zh-CN, en*→en, else zh-CN).
 * Home entry (/ and /en/) may redirect once; deep links never auto-rewrite.
 */
(() => {
  const STORAGE_KEY = "approving-locale";

  function readBase() {
    const raw = document.documentElement.getAttribute("data-base");
    if (raw == null || raw === "") return "";
    return raw.endsWith("/") ? raw.slice(0, -1) : raw;
  }

  function withBase(path) {
    const base = readBase();
    const cleaned = path.startsWith("/") ? path : `/${path}`;
    return `${base}${cleaned}`;
  }

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
   * - no saved: en* on / → /en/; zh*/other stay on /
   * - no saved on /en/: stay (do not bounce zh* away)
   */
  function maybeRedirectHome() {
    if (document.documentElement.getAttribute("data-home-entry") !== "1") return;

    const page = pageLocale();
    const saved = getSaved();
    let dest = null;

    if (saved === "en" && page === "zh-CN") dest = withBase("/en/");
    else if (saved === "zh-CN" && page === "en") dest = withBase("/");
    else if (!saved && page === "zh-CN" && fromNavigator() === "en") dest = withBase("/en/");

    if (dest) location.replace(dest);
  }

  wireSwitcher();
  maybeRedirectHome();
})();
