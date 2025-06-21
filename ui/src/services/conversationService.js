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
        const resp = await conversationAPI.post({});
        const data = resp?.data || {};
        convID = data?.id;
        
        if (!convID) {
            console.error('Failed to obtain conversation id');
            return null;
        }

        const input = conversationContext.signals?.input || {};
        const collection = conversationContext.signals?.collection || {};
        collection.value = [{ ...data }];
        input.value = { ...input.peek(), filter: { id: convID } };
        conversionHandlers.setFormData({ ...data });
        conversionHandlers.setSelected({ selected: { ...data }, rowIndex: 0 });
    }
    
    return convID;
}

/**
 * Creates a new conversation
 * @param {Object} options - Options object
 * @param {Object} options.context - Application context
 * @returns {Promise<string>} - The new conversation ID
 */
export async function newConversation({ context }) {
    try {
        const convID = await ensureConversation({ context });
        if (!convID) {
            console.error('newConversation: backend did not return id');
            return null;
        }
        return convID;
    } catch (error) {
        console.error('newConversation error:', error);
        return null;
    }
}