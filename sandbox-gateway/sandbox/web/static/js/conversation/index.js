/**
 * 会话扩展统一入口：新 session/update 类型请 registerSessionUpdateHandler，或改 builtin_session_handlers。
 */
export {CardType} from './card_types.js';
export {dispatchSessionUpdate, registerSessionUpdateHandler} from './session_update_dispatch.js';
