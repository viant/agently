// Conversation management service

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
    
    if (!convID) {
        // include current overrides (model, agent, tools) when present
        const currentForm = conversationContext.handlers?.dataSource?.peekFormData?.() || {};
        const {model = '', agent = '', tools: toolsRaw = ''} = currentForm;

        const body = {};
        if (model)  body.model  = model;
        if (agent)  body.agent  = agent;

        // Tools may come as array from treeMultiSelect or as comma separated string.
        if (Array.isArray(toolsRaw) && toolsRaw.length) {
            body.tools = toolsRaw.join(',');
        } else if (typeof toolsRaw === 'string' && toolsRaw.trim() !== '') {
            body.tools = toolsRaw.trim();
        }

        const resp = await conversationAPI.post({ body });
        const data = (resp && typeof resp === 'object' && 'data' in resp) ? resp.data : resp;
        convID = data?.id;
        
        if (!convID) {
            console.error('Failed to obtain conversation id');
            return null;
        }

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
    const conversations = context.Context('conversations');
    conversations.handlers.dataSource.setSelection({args: {rowIndex: -1}})
}
