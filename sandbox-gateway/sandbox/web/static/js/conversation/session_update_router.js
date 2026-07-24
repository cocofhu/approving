/** session/update 可注册分发（扩展新 kind 时 register 即可）。 */

/**
 * @typedef {{ id: string, priority: number, match: (ctx: object) => boolean, handle: (ctx: object) => void }} SessionUpdateHandler
 * @typedef {{ chatView: object, kind: string, flat: object, merged: object }} SessionUpdateContext
 */

/**
 * @returns {{ register: (h: SessionUpdateHandler) => void, dispatch: (ctx: SessionUpdateContext) => void }}
 */
export function createSessionUpdateRouter() {
    /** @type {SessionUpdateHandler[]} */
    const handlers = [];
    /** @type {SessionUpdateHandler|null} */
    let fallback = null;

    return {
        /**
         * @param {SessionUpdateHandler} h
         */
        register(h) {
            if (h.id === 'fallback') {
                fallback = h;
                return;
            }
            handlers.push(h);
            handlers.sort((a, b) => b.priority - a.priority);
        },
        /**
         * @param {SessionUpdateContext} ctx
         */
        dispatch(ctx) {
            for (const h of handlers) {
                if (h.match(ctx)) {
                    h.handle(ctx);
                    return;
                }
            }
            if (fallback) fallback.handle(ctx);
        },
    };
}
