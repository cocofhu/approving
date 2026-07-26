/**
 * Protocol allowlist for user-supplied image URLs assigned to img.src
 * (CodeQL #1 XSS / #3 open redirect).
 *
 * Returns a canonical safe string (data:image whitelist or URL.href for http(s)),
 * or empty string when the input must not be assigned to img.src.
 */

const ALLOWED_DATA_MIME = new Set([
    'image/png',
    'image/jpeg',
    'image/jpg',
    'image/gif',
    'image/webp',
]);

/**
 * @param {unknown} url
 * @returns {string} Safe URL for img.src, or "" if rejected.
 */
export function sanitizeImageURL(url) {
    if (typeof url !== 'string') return '';
    const t = url.trim();
    if (!t) return '';

    if (/^data:/i.test(t)) {
        const m = /^data:(image\/([a-z0-9.+-]+))(;base64)?,/i.exec(t);
        if (!m) return '';
        let mime = m[1].toLowerCase();
        if (!ALLOWED_DATA_MIME.has(mime)) return '';
        if (mime === 'image/jpg') {
            return t.replace(/^data:image\/jpg/i, 'data:image/jpeg');
        }
        return t;
    }

    try {
        const u = new URL(t);
        if (u.protocol !== 'http:' && u.protocol !== 'https:') return '';
        return u.href;
    } catch {
        return '';
    }
}

/**
 * @deprecated Prefer sanitizeImageURL; kept for any residual boolean checks.
 * @param {unknown} url
 * @returns {boolean}
 */
export function isSafeImageURL(url) {
    return sanitizeImageURL(url) !== '';
}
