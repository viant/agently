// Chat service helper used by forge SettingProvider.
// Contains submitMessage implementation extracted from App.jsx to keep
// App clean and focused on composition.

import { endpoints } from '../endpoint';
import { FormRenderer } from 'forge';
import MCPForm from '../components/MCPForm.jsx';
import MCPInteraction from '../components/MCPInteraction.jsx';
import PolicyApproval from '../components/PolicyApproval.jsx';
import { poll, fetchJSON } from './utils/apiUtils';
import { classifyMessage, normalizeMessages } from './messageNormalizer';
import { ensureConversation, newConversation } from './conversationService';


/**
 * Submits a user message to the chat
 * @param {Object} options - Options object
 * @param {Object} options.context - Application context
 * @param {Object} options.message - Message to submit
 * @returns {Promise<void>}
 */
export async function submitMessage({ context, message }) {
    // Reference to DataSource controlling the chat collection – used to toggle
    // the global loading lock that enables / disables the Composer's Send button in the UI.
    const messagesContext = context.Context('messages');
    const messagesAPI = messagesContext.connector;
    const messageHandlers = messagesContext?.handlers?.dataSource;

    // Engage global lock (button disabled)
    messageHandlers?.setLoading(true);

    try {
        const convID = await ensureConversation({ context });
        if (!convID) {
            return;
        }

        // Post user message
        const postResp = await messagesAPI.post({
            inputParameters: { convID },
            body: { content: message.content },
        });

        const messageId = postResp?.data?.id;
        if (!messageId) {
            console.error('Message accepted but no id returned', postResp);
            return;
        }

        // Optimistic UI update
        updateCollectionWithUserMessage(messagesContext, messageId, message.content);

        // Polling
        const msgURL = endpoints.appAPI.baseURL + `/conversations/${convID}/messages?parentId=${messageId}`;
        await pollForMessages(messagesContext, msgURL);
    } catch (error) {
        console.error('submitMessage error:', error);
        messageHandlers?.setError(error);
    } finally {
        // Release global lock (button enabled)
        messageHandlers?.setLoading(false);
    }
}

/**
 * Dummy upload placeholder to keep original API shape
 */
export async function upload() {
    // No implementation needed
}

/**
 * Updates the collection signal with a new user message
 * @param {Object} messagesContext - Messages context
 * @param {string} messageId - ID of the new message
 * @param {string} content - Content of the message
 */
function updateCollectionWithUserMessage(messagesContext, messageId, content) {
    const collSig = messagesContext.signals?.collection;
    if (!collSig) return;

    const curr = Array.isArray(collSig.value) ? collSig.value : [];
    collSig.value = [
        ...curr,
        {
            id: messageId,
            parentId: messageId,
            role: 'user',
            content: content,
            createdAt: new Date().toISOString(),
        },
    ];
}

/**
 * Merges incoming messages with the current collection
 * @param {Object} messagesContext - Messages context
 * @param {Array} incoming - Incoming messages to merge
 */
function mergeMessages(messagesContext, incoming) {
    const collSig = messagesContext.signals?.collection;
    if (!collSig || !Array.isArray(incoming) || !incoming.length) {
        return;
    }

    const current = Array.isArray(collSig.value) ? [...collSig.value] : [];

    incoming.forEach((msg) => {
        const idx = current.findIndex((m) => m.id === msg.id);
        if (idx >= 0) {
            const updated = { ...current[idx], ...msg };
            if (Array.isArray(updated.execution)) {
                updated.execution = [...updated.execution]; // new ref to force tables
            }
            if (!updated.createdAt) {
                updated.createdAt = new Date().toISOString();
            }
            current[idx] = updated;
        } else {
            const addedBase = Array.isArray(msg.execution)
                ? { ...msg, execution: [...msg.execution] }
                : { ...msg };

            if (!addedBase.createdAt) {
                addedBase.createdAt = new Date().toISOString();
            }
            current.push(addedBase);
        }
    });

    // Publish via signal with new array ref
    collSig.value = [...current];

    // Keep the DataSource form in sync with the newest assistant chunk
    messagesContext?.handlers?.dataSource?.setFormData?.({ 
        values: { ...current[current.length - 1] } 
    });
}

/**
 * Injects form messages for elicitation responses
 * @param {Object} messagesContext - Messages context
 * @param {Array} data - Message data
 */
function injectFormMessages(messagesContext, data) {
    try {
        const collSig = messagesContext.signals?.collection;
        if (!collSig || !Array.isArray(data)) return;

        const current = Array.isArray(collSig.value) ? [...collSig.value] : [];

        data.forEach((msg) => {
            if (msg.role !== 'assistant' || !msg.elicitation) return;

            const formMsgId = `${msg.id}/form`;

            // Skip when the synthetic form already exists
            const exists = current.some((m) => m.id === formMsgId);
            if (exists) return;

            const synthetic = {
                id: formMsgId,
                parentId: msg.id,
                role: 'assistant',
                isForm: true,                // flag for custom renderer
                formSpec: msg.elicitation,   // pass full elicitation struct
                createdAt: new Date().toISOString(),
            };

            current.push(synthetic);
        });

        // Update signal when we added synthetic entries
        if (current.length !== collSig.value.length) {
            collSig.value = [...current];
        }
    } catch (error) {
        console.error('injectFormMessages error:', error);
    }
}

/**
 * Polls for new messages
 * @param {Object} messagesContext - Messages context
 * @param {string} msgURL - URL to poll for messages
 * @returns {Promise<void>}
 */
async function pollForMessages(messagesContext, msgURL) {
    let assistantArrived = false;
    let stillCount = 0;
    let lastSig = '';

    await poll(
        () => fetchJSON(msgURL),
        (json) => {
            if (!json || json.status !== 'ok' || !Array.isArray(json.data)) {
                return false;
            }

            mergeMessages(messagesContext, json.data);
            injectFormMessages(messagesContext, json.data);

            // Detect stability
            const sig = JSON.stringify(json.data);
            if (sig === lastSig) {
                stillCount += 1;
            } else {
                stillCount = 0;
                lastSig = sig;
            }

            // Detect first assistant reply
            if (!assistantArrived) {
                const hasAssistant = json.data.some((m) => m.role === 'assistant');
                if (hasAssistant) {
                    assistantArrived = true;
                }
            }

            // Stop polling when we already saw the assistant message and the
            // payload stayed unchanged for at least one additional tick
            const ASSISTANT_STABILITY_TICKS = 1;
            return assistantArrived && stillCount >= ASSISTANT_STABILITY_TICKS;
        },
        // Allow up to ~15 minutes of polling (900 attempts × 1s interval)
        { maxAttempts: 900 }
    );
}



/**
 * Chat service for handling chat interactions
 */
export const chatService = {
    submitMessage,
    upload,
    newConversation,
    classifyMessage,
    normalizeMessages,
    renderers: {
        form: FormRenderer,
        mcpelicitation: MCPForm,
        mcpuserinteraction: MCPInteraction,
        policyapproval: PolicyApproval,
    }
};