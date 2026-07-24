/** 卡片类型常量（供 data-card-type / 扩展用）。 */

/** @readonly */
export const CardType = Object.freeze({
    User: 'user',
    Assistant: 'assistant',
    ToolCall: 'tool_call',
    ToolResult: 'tool_result',
    Confirm: 'confirm',
    Notice: 'notice',
});
