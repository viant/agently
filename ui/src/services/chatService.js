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
        // Grace period to avoid duplicate initial fetch (DataSource auto-fetch vs poller)
        context.resources['messagesGraceUntil'] = Date.now() + 2000;
        context.resources['messagesPollThrottleMs'] = 900;
        context.resources['messagesLastFetchTs'] = 0;
        context.resources['messagesFetchInFlight'] = false;
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

            // Skip polling during initial grace window to avoid duplicate calls
            const now = Date.now();
            const graceUntil = context.resources?.messagesGraceUntil || 0;
            if (now < graceUntil) {
                return;
            }

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
            const ctrlSig = messagesCtx.signals?.control;
            // If the DataSource is already fetching (auto-fetch on input change), skip this tick
            if (ctrlSig?.peek?.()?.loading) {
                return;
            }
            const current = Array.isArray(collSig?.value) ? collSig.value : [];
            // Prefer last turnId if present; fallback to last message id
            let lastID = '';
            for (let i = current.length - 1; i >= 0; i--) {
                if (current[i]?.turnId) { lastID = current[i].turnId; break; }
            }
            if (!lastID && current.length) {
                lastID = current[current.length - 1].id;
            }

            // Skip first poll if no messages yet – initial DataSource fetch handles it
            if (!lastID) {
                return;
            }

            // Throttle and de-dupe in-flight calls
            const nowTick = Date.now();
            const lastTs = context.resources?.messagesLastFetchTs || 0;
            const throttleMs = context.resources?.messagesPollThrottleMs || 900;
            if ((nowTick - lastTs) < throttleMs) {
                return;
            }
            if (context.resources?.messagesFetchInFlight) {
                return;
            }

            // Fetch rich conversation view (v2) via backend proxy on v1 path
            const baseRoot = (endpoints.appAPI.baseURL || (typeof window !== 'undefined' ? window.location.origin + '/v1/api' : ''))
                .replace(/\/+$/,'');
            const base = `${baseRoot}/conversations/${encodeURIComponent(convID)}/messages`;
            const url = `${base}?since=${encodeURIComponent(lastID)}`;
            context.resources.messagesFetchInFlight = true;
            context.resources.messagesLastFetchTs = nowTick;
            const json = await fetchJSON(url);

            // New format: data is a Conversation object with transcript and stage (Go fields are Capitalized)
            const conv = json && json.status === 'ok' ? json.data : null;
            const convStage = conv?.stage || conv?.Stage;
            if (convStage) {
                setStage({ phase: String(convStage) });
            }
            const transcript = Array.isArray(conv?.transcript) ? conv.transcript
                              : Array.isArray(conv?.Transcript) ? conv.Transcript : [];
            const rows = [];

            // Safe date → ISO helper to avoid Invalid time values in UI
            const toISO = (v) => {
                if (!v) return new Date().toISOString();
                try {
                    const d = new Date(v);
                    if (!isNaN(d.getTime())) return d.toISOString();
                } catch (_) { /* ignore */ }
                return new Date().toISOString();
            };

            for (const turn of transcript) {
                const turnId = turn?.id || turn?.Id;
                const messages = Array.isArray(turn?.message) ? turn.message
                                 : Array.isArray(turn?.Message) ? turn.Message : [];

                // Aggregate all events in this turn into a single executions list
                const turnSteps = [];
                for (const m of messages) {
                    // interim events
                    const interim = m?.interim ?? m?.Interim;
                    const created = m?.createdAt || m?.CreatedAt;
                    if (interim) {
                        turnSteps.push({
                            id: (m.id || m.Id || '') + '/interim',
                            name: 'assistant',
                            reason: 'interim',
                            success: true,
                            startedAt: created,
                            endedAt: created,
                        });
                    }
                    // Do not include user/assistant non-interim messages in execution steps
                    // tool/model calls
                    const toolCall = m?.toolCall || m?.ToolCall;
                    const modelCall = m?.modelCall || m?.ModelCall;
                    if (toolCall) {
                        const tc = toolCall;
                        turnSteps.push({
                            id: tc.opId || tc.OpId,
                            name: tc.toolName || tc.ToolName,
                            reason: 'tool_call',
                            success: String((tc.status || tc.Status || '')).toLowerCase() === 'completed',
                            error: tc.errorMessage || tc.ErrorMessage || '',
                            startedAt: tc.startedAt || tc.StartedAt,
                            endedAt: tc.completedAt || tc.CompletedAt,
                            requestPayloadId: tc.requestPayloadId || tc.RequestPayloadId,
                            responsePayloadId: tc.responsePayloadId || tc.ResponsePayloadId,
                            providerRequestPayloadId: null,
                            providerResponsePayloadId: null,
                        });
                    }
                    if (modelCall) {
                        const mc = modelCall;
                        turnSteps.push({
                            id: mc.messageId || mc.MessageId,
                            name: mc.model || mc.Model,
                            reason: 'thinking',
                            success: String((mc.status || mc.Status || '')).toLowerCase() === 'completed',
                            error: mc.errorMessage || mc.ErrorMessage || '',
                            startedAt: mc.startedAt || mc.StartedAt,
                            endedAt: mc.completedAt || mc.CompletedAt,
                            requestPayloadId: mc.requestPayloadId || mc.RequestPayloadId,
                            responsePayloadId: mc.responsePayloadId || mc.ResponsePayloadId,
                            streamPayloadId: mc.streamPayloadId || mc.StreamPayloadId,
                            providerRequestPayloadId: mc.providerRequestPayloadId || mc.ProviderRequestPayloadId,
                            providerResponsePayloadId: mc.providerResponsePayloadId || mc.ProviderResponsePayloadId,
                        });
                    }
                }

                // Normalize/compute elapsed for each step and drop invalid dates
                const normalizedSteps = turnSteps.map(s => {
                    const started = s.startedAt ? new Date(s.startedAt) : null;
                    const ended = s.endedAt ? new Date(s.endedAt) : null;
                    let elapsed = '';
                    if (started && ended && !isNaN(started) && !isNaN(ended)) {
                        elapsed = (((ended - started) / 1000).toFixed(2)) + 's';
                    }
                    return { ...s, elapsed };
                });
                const turnExecutions = normalizedSteps.length ? [{ steps: normalizedSteps }] : [];

                // Build per-message rows first; attach executions only to one carrier message
                const turnRows = [];
                for (const m of messages) {
                    const modelCall = m?.modelCall || m?.ModelCall;
                    const toolCall  = m?.toolCall  || m?.ToolCall;
                    const hasCall   = !!(modelCall || toolCall);
                    const isInterim = !!(m?.interim ?? m?.Interim);
                    const roleLower = String(m.role || m.Role || '').toLowerCase();
                    let usage = null;
                    const pt = (modelCall && (modelCall.promptTokens || modelCall.PromptTokens)) || 0;
                    const ct = (modelCall && (modelCall.completionTokens || modelCall.CompletionTokens)) || 0;
                    const tt = (modelCall && (modelCall.totalTokens || modelCall.TotalTokens));
                    if (pt || ct || tt) {
                        usage = {
                            promptTokens: Number(pt || 0),
                            completionTokens: Number(ct || 0),
                            totalTokens: Number(tt != null ? tt : (Number(pt || 0) + Number(ct || 0))),
                        };
                    }
                    const id = m.id || m.Id;
                    const createdAt = toISO(m.createdAt || m.CreatedAt);
                    const turnIdRef = m.turnId || m.TurnId || turnId;

                    // Only keep user/assistant roles, skip interim entries
                    if (isInterim) continue;
                    if (roleLower !== 'user' && roleLower !== 'assistant') continue;
                    // Keep assistant elicitation prompts so they render as forms

                    // If message carries tool/model call, do not render a separate bubble
                    if (hasCall) continue;

                    turnRows.push({
                        id,
                        conversationId: m.conversationId || m.ConversationId,
                        role: roleLower,
                        content: m.content || m.Content || '',
                        createdAt,
                        toolName: m.toolName || m.ToolName,
                        turnId: turnIdRef,
                        parentId: turnIdRef,
                        executions: [], // attach later to a single row
                        usage,
                        elicitation: m.elicitation || m.Elicitation,
                    });
                }

                // Choose a single carrier message for executions: prefer first user, else first message
                let carrierIdx = -1;
                for (let i = 0; i < turnRows.length; i++) {
                    if (turnRows[i].role === 'user') { carrierIdx = i; break; }
                }
                if (carrierIdx === -1 && turnRows.length) carrierIdx = 0;
                if (carrierIdx >= 0) {
                    turnRows[carrierIdx] = { ...turnRows[carrierIdx], executions: turnExecutions };
                }

                // Append to global rows
                for (const r of turnRows) rows.push(r);
            }
            if (rows.length) {
                receiveMessages(messagesCtx, rows, lastID);
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
    tick().then(() => {}).finally(() => {
        context.resources.chatTimerState = false;
        context.resources.messagesFetchInFlight = false;
    });
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
    // No-op: we render assistant elicitations directly via FormRenderer now.
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
