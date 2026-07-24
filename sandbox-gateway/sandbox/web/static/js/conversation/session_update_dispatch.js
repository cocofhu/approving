/**
 * 全局 session/update 路由器（单例）。扩展请使用 registerSessionUpdateHandler。
 */

import {installBuiltinSessionHandlers} from './builtin_session_handlers.js';
import {createSessionUpdateRouter} from './session_update_router.js';

const router = createSessionUpdateRouter();
installBuiltinSessionHandlers(router);

/**
 * @param {import('./session_update_router.js').SessionUpdateContext} ctx
 */
export function dispatchSessionUpdate(ctx) {
    router.dispatch(ctx);
}

/**
 * @param {import('./session_update_router.js').SessionUpdateHandler} handler
 */
export function registerSessionUpdateHandler(handler) {
    router.register(handler);
}
