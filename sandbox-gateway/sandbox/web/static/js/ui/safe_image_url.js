/**
 * Protocol allowlist for user-supplied image URLs assigned to img.src
 * (CodeQL #1 XSS / #3 open redirect).
 *
 * Allowed: data:image/* (optionally ;base64) and absolute http(s) URLs.
 */

/**
 * @param {unknown} url
 * @returns {boolean}
 */
export function isSafeImageURL(url) {
    if (typeof url !== 'string') return false;
    const t = url.trim();
    if (!t) return false;
    if (/^data:image\/[a-zA-Z0-9.+-]+(;base64)?,/i.test(t)) {
        return true;
    }
    try {
        const u = new URL(t);
        return u.protocol === 'http:' || u.protocol === 'https:';
    } catch {
        return false;
    }
}
