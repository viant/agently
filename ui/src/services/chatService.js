// Chat service helper used by forge SettingProvider.
// Contains submitMessage implementation extracted from App.jsx to keep
// App clean and focused on composition.

import {endpoints} from '../endpoint';
import {FormRenderer} from 'forge';
import MCPForm from '../components/MCPForm.jsx';
import MCPInteraction from '../components/MCPInteraction.jsx';
import PolicyApproval from '../components/PolicyApproval.jsx';
import {poll, fetchJSON} from './utils/apiUtils';
import {classifyMessage, normalizeMessages, isSimpleTextSchema} from './messageNormalizer';

import ExecutionBubble from '../components/chat/ExecutionBubble.jsx';
import HTMLTableBubble from '../components/chat/HTMLTableBubble.jsx';
import {ensureConversation, newConversation} from './conversationService';
import SummaryNote from '../components/chat/SummaryNote.jsx';
import {setStage} from '../utils/stageBus.js';

// -------------------------------
// Window lifecycle helpers
// -------------------------------

/**
 * Called by Forge when the Chat window becomes visible (onInit).
 * Performs the following steps:
 *   1. Fetches default agent & model via fetchMetaDefaults (backend).
 *   2. Pre-selects every tool that matches the default agent patterns so the
 *      Tools field starts populated.
 *   3. Starts a 1-second interval ticker whose id is stored on the window
 *      context so it can be cleared later in onDestroy.
 */
export async function onInit({context}) {


    try {
        const convContext = context.Context('conversations');
        const agentContext = context.Context('agents');
        const toolsContext = context.Context('tools');
        const metaContext = context.Context('meta')


        const defaults = await fetchMetaDefaults({context});
        const agentResp = await agentContext.connector.get({})
        const agents = agentResp.data || [];

        const toolResp = await toolsContext.connector.get({})
        const allTools = toolResp.data || [];
        const values = {...defaults, agents: {}}

        for (const agent of agents) {
            const agentTools = agent.tool || []
            const patterns = agentTools
                .map(extractPattern)
                .filter(Boolean)
                .map(canonPattern);
            const matchedTools = filterToolsByPatterns(allTools, patterns)
            values.agents[agent.id] = matchedTools.map(t => t.name)

            metaContext.handlers.dataSource.setFormData({values: values})
        }
        const matchedToolNames = values.agents[defaults.agent]
        convContext.handlers.dataSource.setFormField({item: {id: 'tools'}, value: matchedToolNames})
    } catch (err) {
        console.error('chatService.onInit error:', err);
    }

    try {
        // 3) Start 1-second polling loop. Replace any previous one.
        if (context.resources.chatTimer) {
            clearInterval(context.resources.chatTimer);
        }
        context.resources['chatTimerState'] = {busy: false}
        context.resources['chatTimer'] = setInterval(() => {
            startPolling({context});
        }, 1000);
    } catch
        (err) {
        console.error('chatService.onOpen error:', err);
    }
}

function selectFolder(props) {
    const {context, selected} = props;
    console.log('selectFolder 1', context)

    context.handlers.dialog.commit()
    console.log('selectFolder', props)
}

function startPolling({context}) {
    if (!context || typeof context.Context !== 'function') {
        console.warn('chatService.startPolling: invalid context');
        return;
    }
    const tick = async () => {
        if (context.resources.chatTimerState) return;
        try {
            context.resources.chatTimerState = true;

            const convCtx = context.Context('conversations');
            const convID = convCtx?.handlers?.dataSource.peekFormData?.()?.id;
            if (!convID) {
                return; // no active conversation – nothing to do
            }

            const messagesCtx = context.Context('messages');
            if (!messagesCtx) {
                return;
            }

            const collSig = messagesCtx.signals?.collection;
            const current = Array.isArray(collSig?.value) ? collSig.value : [];
            const lastID = current.length ? current[current.length - 1].id : '';

            const base = endpoints.appAPI.baseURL + `/conversations/${convID}/messages`;
            const url = lastID
                ? `${base}?since=${encodeURIComponent(lastID)}`
                : `${base}`;

            const json = await fetchJSON(url);

            // Broadcast stage (even when no new messages) so StatusBar updates smoothly.
            if (json && json.stage) {
                setStage(json.stage);
            }

            if (json && json.status === 'ok' && Array.isArray(json.data) && json.data.length) {
                receiveMessages(messagesCtx, json.data, lastID);
            }

            // Determine id of newest assistant message after merge.
            const messages = messagesCtx.signals?.collection?.value || [];
            let newestAssistantID = '';
            for (let i = messages.length - 1; i >= 0; i--) {
                if (messages[i].role === 'assistant') {
                    newestAssistantID = messages[i].id;
                    break;
                }
            }

            // Refresh usage panel by computing from messages (avoid extra HTTP call).
            try {
                const usageCtx = context.Context('usage');
                const summary = computeUsageFromMessages(messages, convID);
                // Set as a one-row collection so derived usagePerModel (selectors.data: perModel)
                // can project the perModel list.
                usageCtx?.handlers?.dataSource?.setCollection?.({ rows: [summary] });
            } catch (err) {
                console.error('usage compute error:', err);
            }

        } catch (err) {
            console.error('chatService.startPolling tick error:', err);
        }
    };
    tick().then().finally(() => context.resources.chatTimerState = false)
}

// Map v2 transcript DAO rows to legacy message shape expected by the UI merge
function mapTranscriptToMessages(rows = []) {
    return rows.map(v => ({
        id: v.id,
        conversationId: v.conversationId,
        role: v.role,
        content: v.content,
        createdAt: v.createdAt || new Date().toISOString(),
        toolName: v.toolName,
    }));
}


/**
 * Stops the polling loop created in onOpen. Bound to window onDestroy.
 */
export function onDestroy({context}) {
    if (context?.resources?.chatTimer) {
        clearInterval(context.resources.chatTimer);
        delete context.resources.chatTimer;
    }

    // Clear global stage so other windows do not show stale data.
    setStage(null);
}


function matchAgentTools(context, patterns) {
    // grab the full tool objects, not just names
    const allTools = getAllTools(context);
    // filter by name patterns, but keep the full object
    const matchedTools = filterToolsByPatterns(allTools, patterns);
    return matchedTools;
}

/**
 * Builds dynamic options for the tools treeMultiSelect widget.
 * @param {Object} options - Forge handler options
 * @param {Object} options.context - SettingProvider context instance
 * @returns {{options: Array<{id: string, label: string, value: string, tooltip: string}>}}
 */
export function buildToolOptions({context}) {
    const agentName = getSelectedAgent(context);
    if (!agentName) {
        return {options: []};
    }

    const patterns = getAgentPatterns(context, agentName);
    if (!patterns.length) {
        return {options: []};
    }

    const matchedTools = matchAgentTools(context, patterns);
    return {options: formatOptions(matchedTools)};
}

// --- Helpers ---

function getSelectedAgent(context) {
    const conv = context.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
    return conv.agent || '';
}

function getAgentPatterns(context, agentName) {
    const agentContext = context.Context('agents');
    let agents = agentContext?.handlers?.dataSource?.peekCollection?.() || [];

    const agent = agents.find(a => a?.name === agentName || a?.id === agentName);

    if (!agent?.tool || !Array.isArray(agent.tool)) {
        return [];
    }
    return agent.tool
        .map(extractPattern)
        .filter(Boolean)
        .map(canonPattern);
}

function extractPattern(item) {
    if (typeof item === 'string') return item;
    return item.pattern || item.definition?.name || '';
}

function canonPattern(pattern) {
    return pattern.replace(/\//g, '_');
}

// Return the full tool objects
function getAllTools(context) {
    return (context.Context?.('tools')?.handlers?.dataSource?.peekCollection?.() || [])
        .filter(t => t && t.name)  // ensure valid
        .map(t => ({name: t.name, description: t.description || '', id: t.id}));
}

// Filter the tools by your name-based patterns
function filterToolsByPatterns(tools, patterns) {
    const matches = tools.filter(tool =>
        patterns.some(pat => pat === '*' || tool.name.startsWith(pat))
    );
    // dedupe by name and sort
    const unique = Array.from(new Map(matches.map(t => [t.name, t])).values());
    return unique.sort((a, b) => a.name.localeCompare(b.name));
}

// Build the final options, including tooltip
function formatOptions(tools) {
    return tools.map(tool => ({
        id: tool.name,
        label: tool.name,
        value: tool.name,
        tooltip: tool.description
    }));
}


export async function selectAgent(props) {
    const {context} = props;
    const metaContext = context.Context('meta')
    const form = metaContext.handlers.dataSource.peekFormData()
    const convContext = context.Context('conversations')
    const agentId = props.selected
    const tools = form.agents[agentId]
    convContext.handlers.dataSource.setFormField({item: {id: 'tools'}, value: tools})
    return true
}


/**
 * Submits a user message to the chat
 * @param {Object} options - Options object
 * @param {Object} options.context - Application context
 * @param {Object} options.message - Message to submit
 * @returns {Promise<void>}
 */
export async function submitMessage(props) {
    const {context, message, parameters} = props;
    console.log('submitMessage', props)

    // Reference to DataSource controlling the chat collection – used to toggle
    // the global loading lock that enables / disables the Composer's Send button in the UI.
    const messagesContext = context.Context('messages');
    const messagesAPI = messagesContext.connector;
    const messageHandlers = messagesContext?.handlers?.dataSource;

    // Engage global lock (button disabled)
    messageHandlers?.setLoading(true);
    try {
        const convID = await ensureConversation({context});
        if (!convID) {
            return;
        }

        const body = {
            content: message.content,
        }

        if (parameters && parameters.model) {
            body.model = parameters.model;
        }
        if (parameters && parameters.agent) {
            body.agent = parameters.agent;
        }

        // Post user message
        const postResp = await messagesAPI.post({
            inputParameters: {convID},
            body: body
        });

        const messageId = postResp?.data?.id;
        if (!messageId) {
            console.error('Message accepted but no id returned', postResp);
            return;
        }

        // Optimistic UI update
        updateCollectionWithUserMessage(messagesContext, messageId, message.content);

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
 * Aborts the currently running assistant turn by calling backend terminate
 * endpoint. Wired to chat.onAbort event so the Forge chat component shows the
 * Abort/Stop button while streaming.
 * @param {Object} props - Forge handler props
 * @param {Object} props.context - SettingProvider context
 */
export async function abortConversation(props) {
    const { context } = props || {};
    if (!context || typeof context.Context !== 'function') {
        console.warn('chatService.abortConversation: invalid context');
        return false;
    }

    try {
        const convCtx = context.Context('conversations');
        const convAPI = convCtx?.connector;
        const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id ||
                       convCtx?.handlers?.dataSource?.getSelection?.()?.selected?.id;

        if (!convID) {
            console.warn('chatService.abortConversation – no active conversation');
            return false;
        }

        await convAPI.post({
            uri: `/v1/api/conversations/${encodeURIComponent(convID)}/terminate`,
            inputParameters: { id: convID },
        });

        // Optimistic stage update; backend will publish final stage via polling.
        setStage({ phase: 'aborted' });

        return true;
    } catch (err) {
        console.error('chatService.abortConversation error:', err);
        // Show error in UI if possible.
        const convCtx = context.Context('conversations');
        convCtx?.handlers?.setError?.(err);
        return false;
    }
}

/**
 * Fetches default agent/model from backend metadata endpoint and pre-fills the
 * conversations form data so that the Settings dialog shows current default
 * selections.
 */
export async function fetchMetaDefaults({context}) {


    const metaContext = context.Context('meta')
    const metaAPI = metaContext.connector;
    try {
        const resp = await metaAPI.get({})
        if (!resp) return;
        const {agent = '', model = ''} = resp;
        const convCtx = context.Context('conversations');
        if (!convCtx?.handlers?.dataSource) return;

        const existing = convCtx.handlers.dataSource.peekFormData?.() || {};
        const values = {...existing, agent, model};
        convCtx.handlers.dataSource.setFormData({values: values});
        return values
    } catch (err) {
        console.error('fetchMetaDefaults error', err);
    }
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
    // If this id already exists (e.g., server responded faster than optimistic add),
    // do not append a duplicate; update fields in-place if needed.
    const idx = curr.findIndex(m => m && m.id === messageId);
    if (idx >= 0) {
        const existing = { ...curr[idx] };
        if (!existing.content) existing.content = content;
        if (!existing.createdAt) existing.createdAt = new Date().toISOString();
        const next = [...curr];
        next[idx] = existing;
        collSig.value = next;
        return;
    }
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
        // Messages flagged as "summarized" have been condensed into a single
        // summary entry. Remove any prior copy from the transcript and skip
        // adding/merging.
        if (msg.status === 'summarized') {
            const idx = current.findIndex((m) => m.id === msg.id);
            if (idx >= 0) {
                current.splice(idx, 1); // delete in-place
            }
            return; // done
        }
        const idx = current.findIndex((m) => m.id === msg.id);
        if (idx >= 0) {
            const prev = current[idx] || {};
            // Prefer latest network payload wholesale when id matches.
            const updated = { ...msg };
            // Preserve createdAt if network omitted it
            if (!updated.createdAt) {
                updated.createdAt = prev.createdAt || new Date().toISOString();
            }
            // Ensure immutable refs for arrays
            if (Array.isArray(updated.executions)) {
                updated.executions = [...updated.executions];
            }
            if (Array.isArray(updated.execution)) {
                updated.execution = [...updated.execution];
            }
            current[idx] = updated;
        } else {
            const addedBase = Array.isArray(msg.execution)
                ? {...msg, execution: [...msg.execution]}
                : {...msg};

            // Force new ref for executions array when present
            if (Array.isArray(addedBase.executions)) {
                addedBase.executions = [...addedBase.executions];
            }

            if (!addedBase.createdAt) {
                addedBase.createdAt = new Date().toISOString();
            }
            current.push(addedBase);
        }
    });

    // Final safety net: ensure uniqueness by id (last-write wins), preserve order
    const dedupById = (list) => {
        const seen = new Set();
        const out = [];
        for (let i = list.length - 1; i >= 0; i--) {
            const it = list[i];
            const id = it && it.id;
            if (!id) continue;
            if (seen.has(id)) continue;
            seen.add(id);
            out.unshift(it);
        }
        return out;
    };

    collSig.value = dedupById(current);

    // Keep the DataSource form in sync with the newest assistant chunk
    messagesContext?.handlers?.dataSource?.setFormData?.({
        values: {...current[current.length - 1]}
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
            // Skip trivial single-field text elicitations – they are rendered
            // as normal bubbles and therefore do not need a synthetic form.
            if (isSimpleTextSchema(msg.elicitation.requestedSchema)) return;

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

// --------------------------- Public helper ------------------------------

// receiveMessages merges incoming messages into the Forge messages DataSource
// and injects synthetic form-renderer placeholders when necessary. Intended
// for generic polling logic (open chat follow-up fetches).
export function receiveMessages(messagesContext, data, sinceId = '') {
    if (!Array.isArray(data) || data.length === 0) return;
    // Prefer merging fully – including the sinceId boundary – so that enriched
    // rows (with executions/usage) update the existing optimistic/plain row.
    mergeMessages(messagesContext, data);
    injectFormMessages(messagesContext, data);
}

// --------------------------- Usage computation ------------------------------

function computeUsageFromMessages(messages = [], conversationId = '') {
    let inputTokens = 0, outputTokens = 0, embeddingTokens = 0, cachedTokens = 0;
    const perModelMap = new Map();

    for (const m of messages) {
        const u = m?.usage;
        if (!u) continue;
        inputTokens += (u.promptTokens || 0);
        outputTokens += (u.completionTokens || 0);
        // No embedding/cached per message – keep at zero

        // Determine model name from executions thinking step, fallback to 'unknown'
        let model = 'unknown';
        if (Array.isArray(m.executions)) {
            for (const ex of m.executions) {
                const steps = ex?.steps || [];
                const mc = steps.find(s => (s?.reason === 'thinking' && s?.name));
                if (mc && mc.name) { model = mc.name; break; }
            }
        }
        const agg = perModelMap.get(model) || { model, inputTokens: 0, outputTokens: 0, embeddingTokens: 0, cachedTokens: 0 };
        agg.inputTokens += (u.promptTokens || 0);
        agg.outputTokens += (u.completionTokens || 0);
        perModelMap.set(model, agg);
    }

    const perModel = Array.from(perModelMap.values());
    const totalTokens = inputTokens + outputTokens + embeddingTokens + cachedTokens;
    return { conversationId, inputTokens, outputTokens, embeddingTokens, cachedTokens, totalTokens, perModel };
}


/**
 * Chat service for handling chat interactions
 */
export const chatService = {
    submitMessage,
    upload,
    onInit,
    onDestroy,
    selectAgent,
    fetchMetaDefaults,
    newConversation,
    classifyMessage,
    normalizeMessages,
    selectFolder,
    buildToolOptions,
    receiveMessages,
    renderers: {
        execution: ExecutionBubble,
        form: FormRenderer,
        mcpelicitation: MCPForm,
        mcpuserinteraction: MCPInteraction,
        policyapproval: PolicyApproval,
        htmltable: HTMLTableBubble,
        summary: SummaryNote,
    },

};


// --------- internal state ---------

// Maps window context objects → setInterval id so we can safely start / stop
// per-window background polling without leaking intervals when the window is
// closed or remounted.
const pollingRegistry = new WeakMap();
