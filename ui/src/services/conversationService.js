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


export async function newConversation({context}) {
    const conversations = context.Context('conversations');
    conversations.handlers.dataSource.setSelection({args: {rowIndex: -1}})
}