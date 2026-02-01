// Conversation management service

import {saveSettings} from "./chatService.js";
import { getLogger } from 'forge/utils/logger';

const log = getLogger('agently');

const activeConversationStorageKey = 'agently.activeConversationID';
let activeConversationID = '';

export function setActiveConversationID(value) {
    const next = String(value || '').trim();
    activeConversationID = next;
    try {
        if (next) localStorage.setItem(activeConversationStorageKey, next);
        else localStorage.removeItem(activeConversationStorageKey);
    } catch (_) {}
}

export function getActiveConversationID() {
    if (activeConversationID) return activeConversationID;
    try {
        const v = String(localStorage.getItem(activeConversationStorageKey) || '').trim();
        if (v) activeConversationID = v;
    } catch (_) {}
    return activeConversationID;
}

/**
 * Ensures a conversation exists, creating a new one if necessary
 * @param {Object} options - Options object
 * @param {Object} options.context - Application context
 * @returns {Promise<string>} - The conversation ID
 */
export async function ensureConversation({ context }) {
    const conversationContext = context.Context('conversations');
    const conversationAPI = conversationContext.connector;
    const conversionHandlers = conversationContext.handlers.dataSource;

    // Ensure we have a conversation id
    let convID = conversionHandlers.getSelection()?.selected?.id;
    if (convID) {
        setActiveConversationID(convID);
    }
    
    if (!convID) {
        // include current overrides (model, agent, tools) when present
        const dsForm = conversationContext.handlers?.dataSource?.peekFormData?.() || {};
        const sigForm = (conversationContext.signals?.form?.peek?.() || conversationContext.signals?.form?.value || {});
        // Prefer signals form (newest UI selections) over DS form (may be stale).
        const currentForm = { ...dsForm, ...sigForm };
        const {model = '', agent = '', tools: toolsRaw0 = '', tool: toolRaw = '', autoSelectTools = false, visibility = ''} = currentForm;
        const toolsRaw = toolsRaw0 || toolRaw || '';

        const body = {};
        if (model)  body.model  = model;
        if (agent)  body.agent  = agent;
        if (visibility) body.visibility = visibility;

        // Tools may come as array from treeMultiSelect or as comma separated string.
        // When auto tool selection is enabled, omit explicit tool selection so the backend can route.
        if (!autoSelectTools) {
            if (Array.isArray(toolsRaw) && toolsRaw.length) {
                body.tools = toolsRaw.join(',');
            } else if (typeof toolsRaw === 'string' && toolsRaw.trim() !== '') {
                body.tools = toolsRaw.trim();
            }
        }

        const resp = await conversationAPI.post({ body });
        const data = (resp && typeof resp === 'object' && 'data' in resp) ? resp.data : resp;
        convID = data?.id;
        
        if (!convID) {
            log.error('Failed to obtain conversation id');
            return null;
        }
        setActiveConversationID(convID);

        try {
            const inputSig = conversationContext.signals?.input;
            const collSig  = conversationContext.signals?.collection;
            if (Array.isArray(collSig?.value)) {
                collSig.value = [{ ...data }];
            } else if (collSig) {
                collSig.value = [{ ...data }];
            }
            if (inputSig) {
                const cur = (typeof inputSig.peek === 'function') ? (inputSig.peek() || {}) : (inputSig.value || {});
                inputSig.value = { ...cur, filter: { id: convID } };
            }
        } catch(_) {}
        try { conversionHandlers.setFormData({ values: { ...data } }); } catch(_) {}
        try { conversionHandlers.setSelected({ selected: { ...data }, rowIndex: 0 }); } catch(_) {}
    }
    
    return convID;
}


export async function newConversation({context}) {
    const msgCtx = context.Context('messages');
    msgCtx.handlers.dataSource.handleAddNew()
    msgCtx.handlers.dataSource.setCollection([])

    const convContext = context.Context('conversations');
    convContext.handlers.dataSource.setSelection({args: {rowIndex: -1}})
    setActiveConversationID('');
    saveSettings({context})

}
