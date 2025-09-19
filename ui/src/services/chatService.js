// Chat service helper used by forge SettingProvider.
// Contains submitMessage implementation extracted from App.jsx to keep
// App clean and focused on composition.

import {endpoints} from '../endpoint';
import {FormRenderer} from 'forge';
import MCPForm from '../components/MCPForm.jsx';
import MCPInteraction from '../components/MCPInteraction.jsx';
import PolicyApproval from '../components/PolicyApproval.jsx';
import {poll} from './utils/apiUtils';
import {classifyMessage, normalizeMessages, isSimpleTextSchema} from './messageNormalizer';

import ExecutionBubble from '../components/chat/ExecutionBubble.jsx';
import HTMLTableBubble from '../components/chat/HTMLTableBubble.jsx';
import {ensureConversation, newConversation} from './conversationService';
import SummaryNote from '../components/chat/SummaryNote.jsx';
import {setStage} from '../utils/stageBus.js';
import {setComposerBusy} from '../utils/composerBus.js';

// -------------------------------
// Window lifecycle helpers
// -------------------------------

// Utility: Safe date → ISO string to avoid invalid time values
const toISOSafe = (v) => {
    if (!v) return new Date().toISOString();
    try {
        const d = new Date(v);
        if (!isNaN(d.getTime())) return d.toISOString();
    } catch (_) { /* ignore */
    }
    return new Date().toISOString();
};

// Track which conversations have completed an initial fetch across transient
// DS resets within the same window lifecycle to avoid duplicate initial loads.
const initialFetchDoneByConv = new Set();

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
        const convCtx = context.Context('conversations');
        const handlers = convCtx?.handlers?.dataSource;
        const start = Date.now();
        const deadline = start + 1000;
        const timer = setInterval(() => {
            let convID = '';
            try {
                convID = handlers?.peekFormData?.()?.id || convCtx?.signals?.input?.peek?.()?.id || convCtx?.signals?.input?.peek?.()?.filter?.id;
            } catch (_) {
            }
            if (convID) {
                clearInterval(timer);
                try {
                    handlers?.setFormData?.({values: {id: convID}});
                } catch (_) {
                }
                // Avoid DS-driven fetch to prevent UI blink. Kick off polling immediately.
                try {
                    // Wrap setError to always store string
                    const msgCtx = context.Context('messages');
                    const ds = msgCtx?.handlers?.dataSource;
                    if (ds && typeof ds.setError === 'function' && !ds._setErrorWrapped) {
                        const origSetError = ds.setError.bind(ds);
                        ds.setError = (err) => origSetError(String(err?.message || err));
                        ds._setErrorWrapped = true;
                    }
                    // Set DS input.parameters and trigger fetch=true for initial load.
                    const inSig = msgCtx?.signals?.input;
                    if (inSig) {
                        const cur = (typeof inSig.peek === 'function') ? (inSig.peek() || {}) : (inSig.value || {});
                        const params = {...(cur.parameters || {}), convID, since: ''};
                        const next = {...cur, parameters: params, fetch: true};
                        if (typeof inSig.set === 'function') inSig.set(next); else inSig.value = next;
                        console.log('[chat][signals] set messages.input (initial fetch)', next);
                    }
                } catch (_) {
                }
                try {
                    dsTick({context});
                } catch (_) {
                }
                try {
                    installMessagesDebugHooks(context);
                } catch (_) {
                }
            } else if (Date.now() > deadline) {
                clearInterval(timer);
            }
        }, 60);
    } catch (_) { /* ignore */
    }

    // 4) Start DS-driven refresh loop (no external poller logic).
    try {
        if (context.resources?.chatTimer) {
            clearInterval(context.resources.chatTimer);
        }
        context.resources = context.resources || {};
        context.resources.chatTimer = setInterval(() => dsTick({context}), 1000);
    } catch (_) { /* ignore */
    }
}

// DS-driven refresh: computes since and invokes DS getCollection with input parameters
async function dsTick({context}) {
    try {
        const convCtx = context.Context('conversations');
        const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id;
        if (!convID) {
            const msgContext = context.Context('messages');
            msgContext.signals.collection.value = []
            return;
        }
        const messagesCtx = context.Context('messages');
        if (!messagesCtx) {
            return;
        }
        const ctrl = messagesCtx.signals?.control;
        if (ctrl?.peek?.()?.loading) {
            return;
        }
        const coll = Array.isArray(messagesCtx.signals?.collection?.value) ? messagesCtx.signals.collection.value : [];
        let since = '';
        for (let i = coll.length - 1; i >= 0; i--) {
            if (coll[i]?.turnId) {
                since = coll[i].turnId;
                break;
            }
        }



        if (!since && coll.length) {
            since = coll[coll.length - 1]?.id || '';
        }
        // Throttle requests but do not skip when 'since' is unchanged – we still want
        // to pick up updates within the same turn (model/tool call progress).
        const nowTs = Date.now();
        const minIntervalMs = context.resources?.messagesPollThrottleMs || 1000;
        const lastReqTs = context.resources?.lastDsReqTs || 0;
        if ((nowTs - lastReqTs) < minIntervalMs) {
            return;
        }
        context.resources = context.resources || {};
        context.resources.lastDsReqTs = nowTs;

        // Perform a silent poll via connector to avoid toggling DS loading and flicker
        try {
            const api = messagesCtx.connector;
            console.time(`[chat][poll][silent] since=${since}`);
            const json = await api.get({inputParameters: {convID, since}});
            const conv = json && (json.data ?? json.Data ?? json.conversation ?? json.Conversation ?? json);
            const convStage = conv?.stage || conv?.Stage;
            if (convStage) {
                setStage({phase: String(convStage)});
            }



            const transcript = Array.isArray(conv?.transcript) ? conv.transcript
                : Array.isArray(conv?.Transcript) ? conv.Transcript : [];
            const rows = mapTranscriptToRowsWithExecutions(transcript);
            if (rows.length) {
                receiveMessages(messagesCtx, rows, since);
            }
            // Update noop/backoff signals
            let newestTurnId = '';
            for (let i = transcript.length - 1; i >= 0; i--) {
                const t = transcript[i];
                newestTurnId = (t?.id || t?.Id || newestTurnId);
                if (newestTurnId) break;
            }
            if (newestTurnId && newestTurnId === since) {
                context.resources.messagesNoopPolls = Math.min((context.resources.messagesNoopPolls || 0) + 1, 10);
            } else {
                context.resources.messagesNoopPolls = 0;
            }
            if (newestTurnId) {
                context.resources.messagesLastTurnId = newestTurnId;
            }
            console.timeEnd(`[chat][poll][silent] since=${since}`);
        } catch (e) {
            console.warn('dsTick poll error', e);
        }
    } catch (e) {
        console.warn('dsTick error', e);
    }
}

// --------------------------- Transcript → rows helpers ------------------------------

function buildThinkingStepFromModelCall(mc) {
    if (!mc) return null;
    return {
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
    };
}

function buildToolStepFromToolCall(tc) {
    if (!tc) return null;
    return {
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
    };
}

function computeElapsed(step) {
    const started = step.startedAt ? new Date(step.startedAt) : null;
    const ended = step.endedAt ? new Date(step.endedAt) : null;
    let elapsed = '';
    if (started && ended && !isNaN(started) && !isNaN(ended)) {
        elapsed = (((ended - started) / 1000).toFixed(2)) + 's';
    }
    return elapsed;
}

function mapTranscriptToRowsWithExecutions(transcript = []) {
    const rows = [];
    for (const turn of transcript) {
        const turnId = turn?.id || turn?.Id;
        const messages = Array.isArray(turn?.message) ? turn.message
            : Array.isArray(turn?.Message) ? turn.Message : [];

        // 1) Build all execution steps in this turn (model/tool/interim)
        const steps = [];
        for (const m of messages) {
            const isInterim = !!(m?.interim ?? m?.Interim);
            if (isInterim) {
                const created = m?.createdAt || m?.CreatedAt;
                const interim = {
                    id: (m.id || m.Id || '') + '/interim',
                    name: 'assistant',
                    reason: 'interim',
                    success: true,
                    startedAt: created,
                    endedAt: created
                };
                steps.push({...interim, elapsed: computeElapsed(interim)});
            }
            const mc = m?.modelCall || m?.ModelCall;
            const tc = m?.toolCall || m?.ToolCall;
            const s1 = buildThinkingStepFromModelCall(mc);
            const s2 = buildToolStepFromToolCall(tc);
            if (s1) steps.push({...s1, elapsed: computeElapsed(s1)});
            if (s2) steps.push({...s2, elapsed: computeElapsed(s2)});
        }

        // Sort steps by timestamp (prefer startedAt, fallback endedAt)
        steps.sort((a, b) => {
            const ta = a?.startedAt || a?.endedAt || '';
            const tb = b?.startedAt || b?.endedAt || '';
            const da = ta ? new Date(ta).getTime() : 0;
            const db = tb ? new Date(tb).getTime() : 0;
            return da - db;
        });

        // 2) Build visible chat rows (user/assistant non-interim, skip call-only entries)
        const turnRows = [];
        for (const m of messages) {
            const roleLower = String(m.role || m.Role || '').toLowerCase();
            const isInterim = !!(m?.interim ?? m?.Interim);
            const hasCall = !!(m?.toolCall || m?.ToolCall || m?.modelCall || m?.ModelCall);
            if (isInterim) continue;
            if (hasCall) continue; // call content is represented in steps
            if (roleLower !== 'user' && roleLower !== 'assistant') continue;

            const id = m.id || m.Id;
            const createdAt = toISOSafe(m.createdAt || m.CreatedAt);
            const turnIdRef = m.turnId || m.TurnId || turnId;

            // Row usage derived from model call only when attached to this row later; leave null here.
            turnRows.push({
                id,
                conversationId: m.conversationId || m.ConversationId,
                role: roleLower,
                content: m.content || m.Content || '',
                createdAt,
                toolName: m.toolName || m.ToolName,
                turnId: turnIdRef,
                parentId: turnIdRef,
                executions: [],
                usage: null,
                elicitation: m.elicitation || m.Elicitation,
            });
        }

        // 3) Attach steps to a single carrier message in the same turn (prefer first user)
        if (steps.length && turnRows.length) {
            let carrierIdx = turnRows.findIndex(r => r.role === 'user');
            if (carrierIdx < 0) carrierIdx = 0;
            // Compute usage from the thinking step model if available
            let usage = null;
            // No token fields on step; usage computed elsewhere in poll path; keep null here.
            turnRows[carrierIdx] = {...turnRows[carrierIdx], executions: [{steps}], usage};
        }

        for (const r of turnRows) rows.push(r);
    }
    return rows;
}

// ---------------------------------------------------------------------------
// Debug helpers – instrument Forge DataSource signals & connector calls
// ---------------------------------------------------------------------------
function installMessagesDebugHooks(context) {
    const messagesCtx = context?.Context?.('messages');
    if (!messagesCtx || messagesCtx._debugInstalled) return;
    messagesCtx._debugInstalled = true;

    const collSig = messagesCtx?.signals?.collection;
    const ctrlSig = messagesCtx?.signals?.control;
    // Poll for changes in collection length/loading flag to detect external mutations
    let lastLen = Array.isArray(collSig?.value) ? collSig.value.length : 0;
    let lastLoading = !!ctrlSig?.peek?.()?.loading;
    const tick = () => {
        try {
            const curr = Array.isArray(collSig?.value) ? collSig.value : [];
            const len = curr.length;
            const ctrlVal = ctrlSig?.peek?.() || {};
            let loading = !!ctrlVal.loading;
            const errVal = ctrlVal.error;
            if (errVal && (typeof errVal === 'object')) {
                // Coerce Error object to string so Chat error banner can render safely
                const coerced = String(errVal.message || errVal.toString?.() || '');
                ctrlSig.value = {...ctrlVal, error: coerced};
            }
            // Once we have any messages, suppress spinner on background polls.
            if (len > 0) {
                context.resources = context.resources || {};
                if (!context.resources.suppressMessagesLoading) {
                    context.resources.suppressMessagesLoading = true;
                }
            }
            if (len !== lastLen || loading !== lastLoading) {
                console.log('[chat][signals] messages', {len, loading, ts: Date.now()});
                lastLen = len;
                lastLoading = loading;
            }
        } catch (_) {
        }
    };
    const t = setInterval(tick, 120);
    context.resources = context.resources || {};
    context.resources.messagesDebugTimer = t;

    // Wrap connector GET/POST to log DF activity
    const conn = messagesCtx.connector || {};
    const origGet = conn.get?.bind(conn);
    const origPost = conn.post?.bind(conn);
    if (origGet) {
        conn.get = async (opts) => {
            console.log('[chat][connector][GET] messages', opts);
            const res = await origGet(opts);
            console.log('[chat][connector][GET][done] messages', {status: res?.status, keys: Object.keys(res || {})});
            return res;
        };
    }
    if (origPost) {
        conn.post = async (opts) => {
            console.log('[chat][connector][POST] messages', opts);
            const res = await origPost(opts);
            console.log('[chat][connector][POST][done] messages', {
                status: res?.status,
                dataKeys: Object.keys(res?.data || {})
            });
            return res;
        };
    }

    // Suppress loading spinner flicker for background polling after initial load.
    try {
        const ds = messagesCtx?.handlers?.dataSource;
        if (ds && typeof ds.setLoading === 'function' && !ds._setLoadingWrapped) {
            const origSetLoading = ds.setLoading.bind(ds);
            ds.setLoading = (flag) => {
                // After initial data arrives, avoid toggling loading=true; only allow clearing to false.
                const suppress = !!(context?.resources?.suppressMessagesLoading);
                if (suppress) {
                    if (!flag) {
                        return origSetLoading(false);
                    }
                    // ignore true to prevent spinner flicker
                    return;
                }
                return origSetLoading(flag);
            };
            ds._setLoadingWrapped = true;
        }
    } catch (_) {
    }
}

function selectFolder(props) {
    const {context, selected} = props;
    console.log('selectFolder 1', context)

    context.handlers.dialog.commit()
    console.log('selectFolder', props)
}

// --------------------------- Debug helpers ------------------------------

export function debugHistoryOpen({context}) {
    try {
        const convCtx = context.Context('history') || context.Context('conversations');
        const selected = convCtx?.handlers?.dataSource?.peekSelection?.()?.selected || {};
        const id = selected?.id;
        console.log('[chat][history] open click at', Date.now(), 'convId:', id, 'row:', selected);
    } catch (e) {
        console.log('[chat][history] open click log error', e);
    }
}

export function debugHistorySelection({context}) {
    try {
        const convCtx = context.Context('history') || context.Context('conversations');
        const sel = convCtx?.handlers?.dataSource?.peekSelection?.();
        console.log('[chat][history] selection', Date.now(), sel);
    } catch (e) {
        console.log('[chat][history] selection log error', e);
    }
}

export function debugMessagesLoaded({context, response}) {
    try {
        const data = response?.data || response;
        const transcript = Array.isArray(data?.Transcript) ? data.Transcript : (Array.isArray(data?.transcript) ? data.transcript : []);
        const turns = transcript.length;
        let messages = 0;
        for (const t of transcript) {
            const list = Array.isArray(t?.Message) ? t.Message : (Array.isArray(t?.message) ? t.message : []);
            messages += list.length;
        }
        console.log('[chat][messagesDS] onSuccess at', Date.now(), 'turns:', turns, 'messages:', messages);
    } catch (e) {
        console.log('[chat][messagesDS] onSuccess log error', e);
    }
}

export function debugMessagesError({context, error}) {
    try {
        const msgCtx = context.Context('messages');
        const ctrl = msgCtx?.signals?.control;
        if (ctrl) {
            const prev = (typeof ctrl.peek === 'function') ? (ctrl.peek() || {}) : (ctrl.value || {});
            const coerced = String(error?.message || error);
            ctrl.value = {...prev, error: coerced, loading: false};
        }
        console.log('[chat][messagesDS] onError at', Date.now(), error);
    } catch (e) {
        // ignore
    }
}

// Ensure DS-driven refresh does not shrink the displayed collection by
// restoring the last known full snapshot captured by the polling path.
export function hydrateMessagesCollection({context}) {
    try {
        const messagesCtx = context.Context('messages');
        const collSig = messagesCtx?.signals?.collection;
        const snap = messagesCtx && messagesCtx._snapshot;
        if (!collSig || !Array.isArray(snap) || snap.length === 0) return;
        const curr = Array.isArray(collSig.value) ? collSig.value : [];
        if (curr.length < snap.length) {
            collSig.value = [...snap];
            try {
                console.log('[chat] hydrateMessagesCollection: restored snapshot', {
                    before: curr.length,
                    after: snap.length
                });
            } catch (_) {
            }
        }
    } catch (e) {
        // ignore
    }
}

// onFetch handler for messages DS: return [] so DS does not assign non-array
// payloads to the collection. Merging happens in onSuccess (mergeFromResponse).
// Transform transcript turns (collection) into flat message rows for Chat UI.
export function onFetchMessages(props) {
    try {
        const {collection, context} = props || {};
        const transcript = Array.isArray(collection) ? collection : [];
        const built = mapTranscriptToRowsWithExecutions(transcript);

        // Append-only merge with existing DS rows
        let prev = [];
        try {
            const msgCtx = context?.Context?.('messages');
            prev = Array.isArray(msgCtx?.signals?.collection?.peek?.()) ? msgCtx.signals.collection.peek() : [];
        } catch (_) {
        }
        const seen = new Set((prev || []).map(r => r && r.id).filter(Boolean));
        const merged = [...prev];
        for (const r of built) {
            if (!r || !r.id || seen.has(r.id)) continue;
            seen.add(r.id);
            merged.push(r);
        }
        return merged;
    } catch (_) {
        return [];
    }
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


// Saves settings from the Settings dialog into the conversations form (in-memory)
export function saveSettings(args) {
    const {context} = args
    const metaContext = context.Context('meta');

    const source = metaContext.handlers.dataSource.peekFormData();
    const {agent, model, tools} = source
    const convContext = context.Context('conversations');
    const convDataSource = convContext?.handlers?.dataSource;
    const current = convDataSource.peekFormData()
    convDataSource.setFormData?.({values: {...current, agent, model, tools}});
}

// Applies meta.agentTools mapping to the conversations.tools field when agent changes
export function selectAgent(args) {
    const {context, selected} = args
    const form = context.handlers.dataSource.peekFormData()
    const selectedTools = form.agentInfo[selected]?.tools || []
    const selectedModel = form.agentInfo[selected]?.model || '';
    context.handlers.dataSource.setFormField({item: {id: 'tool'}, value: selectedTools});
    context.handlers.dataSource.setFormField({item: {id: 'model'}, value: selectedModel});
}


// Applies meta.agentTools mapping to the conversations.tools field when agent changes
export function selectModel(args) {
    const {context, selected} = args
    context.handlers.dataSource.setFormField({item: {id: 'model'}, value: selected});
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

    const messagesContext = context.Context('messages');
    const messagesAPI = messagesContext.connector;

    try {
        const convID = await ensureConversation({context});
        if (!convID) {
            return;
        }

        // Mark composer busy via dedicated signal (decoupled from DS loading)
        try {
            setComposerBusy(true);
        } catch (_) {
        }

        const body = {
            content: message.content,
        }
        // Collect Forge-uploaded attachments from message
        const msgAtts = Array.isArray(message?.attachments) ? message.attachments : [];
        if (msgAtts.length > 0) {
            body.attachments = msgAtts.map(a => ({
                name: a?.name,
                size: a?.size,
                stagingFolder: a?.stagingFolder || a?.folder,
                uri: a?.uri,
                mime: a?.mime || a?.type,
            })).filter(x => x && x.uri);
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

        // Ask DS to refresh from backend so DataSource stays the single source of truth
        // Trigger immediate messages refresh: reset since cursor and fetch
        try {
            const msgCtx = context.Context('messages');
            const inSig = msgCtx?.signals?.input;
            if (inSig) {
                const cur = (typeof inSig.peek === 'function') ? (inSig.peek() || {}) : (inSig.value || {});
                const params = {...(cur.parameters || {}), convID, since: ''};
                const next = {...cur, parameters: params, fetch: true};
                if (typeof inSig.set === 'function') inSig.set(next); else inSig.value = next;
            } else {
                await msgCtx?.handlers?.dataSource?.getCollection?.();
            }
        } catch (_) {
        }

    } catch (error) {
        console.error('submitMessage error:', error);
        messagesContext?.handlers?.dataSource?.setError(error);
    } finally {
        try {
            setComposerBusy(false);
        } catch (_) {
        }
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
    const {context} = props || {};
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
            inputParameters: {id: convID},
        });

        // Optimistic stage update; backend will publish final stage via polling.
        setStage({phase: 'aborted'});

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
 * Merges incoming messages with the current collection
 * @param {Object} messagesContext - Messages context
 * @param {Array} incoming - Incoming messages to merge
 */
function mergeMessages(messagesContext, incoming) {
    try {
        console.log('[chat] mergeMessages:incoming', Array.isArray(incoming) ? incoming.length : 0);
    } catch (_) {
    }
    const collSig = messagesContext.signals?.collection;
    if (!collSig || !Array.isArray(incoming) || !incoming.length) {
        return;
    }

    const current = Array.isArray(collSig.value) ? [...collSig.value] : [];
    let changed = false;

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
            const updated = {...msg};
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
            if (!deepEqualShallow(prev, updated)) {
                current[idx] = updated;
                changed = true;
            }
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
            changed = true;
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

    const before = Array.isArray(collSig.value) ? collSig.value.length : 0;
    const next = dedupById(current);
    // Only publish when content actually changed (length or items) to reduce re-renders/blink
    const publish = changed || before !== next.length || !sameIds(collSig.value, next);
    if (publish) {
        collSig.value = next;
        try {
            messagesContext._snapshot = [...next];
        } catch (_) {
        }
        try {
            console.log('[chat] mergeMessages:applied', {before, after: next.length});
        } catch (_) {
        }
        // Do not mutate the messages DataSource form here; changing form values
        // can reset the chat composer input and cause focus flicker.
    }
}

// Compares two arrays of messages by id positionally
function sameIds(a, b) {
    const aa = Array.isArray(a) ? a : [];
    const bb = Array.isArray(b) ? b : [];
    if (aa.length !== bb.length) return false;
    for (let i = 0; i < aa.length; i++) {
        if ((aa[i]?.id) !== (bb[i]?.id)) return false;
    }
    return true;
}

// Lightweight deep equality for message rows; compares primitives and arrays/objects recursively.
function deepEqualShallow(a, b) {
    if (a === b) return true;
    if (!a || !b) return false;
    if (typeof a !== 'object' || typeof b !== 'object') return a === b;
    // Arrays
    if (Array.isArray(a) || Array.isArray(b)) {
        if (!Array.isArray(a) || !Array.isArray(b)) return false;
        if (a.length !== b.length) return false;
        for (let i = 0; i < a.length; i++) {
            if (!deepEqualShallow(a[i], b[i])) return false;
        }
        return true;
    }
    // Objects
    const ak = Object.keys(a).sort();
    const bk = Object.keys(b).sort();
    if (ak.length !== bk.length) return false;
    for (let i = 0; i < ak.length; i++) {
        const k = ak[i];
        if (k !== bk[i]) return false;
        if (!deepEqualShallow(a[k], b[k])) return false;
    }
    return true;
}


// --------------------------- Public helper ------------------------------

// receiveMessages merges incoming messages into the Forge messages DataSource
// and injects synthetic form-renderer placeholders when necessary. Intended
// for generic polling logic (open chat follow-up fetches).
export function receiveMessages(messagesContext, data, sinceId = '') {
    if (!Array.isArray(data) || data.length === 0) return;
    try {
        const convId = data?.[0]?.conversationId;
        console.log('[chat] receiveMessages', {count: data.length, sinceId, convId});
    } catch (_) {
    }
    // Prefer merging fully – including the sinceId boundary – so that enriched
    // rows (with executions/usage) update the existing optimistic/plain row.
    mergeMessages(messagesContext, data);
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
                if (mc && mc.name) {
                    model = mc.name;
                    break;
                }
            }
        }
        const agg = perModelMap.get(model) || {
            model,
            inputTokens: 0,
            outputTokens: 0,
            embeddingTokens: 0,
            cachedTokens: 0
        };
        agg.inputTokens += (u.promptTokens || 0);
        agg.outputTokens += (u.completionTokens || 0);
        perModelMap.set(model, agg);
    }

    const perModel = Array.from(perModelMap.values());
    const totalTokens = inputTokens + outputTokens + embeddingTokens + cachedTokens;
    return {conversationId, inputTokens, outputTokens, embeddingTokens, cachedTokens, totalTokens, perModel};
}


/**
 * Chat service for handling chat interactions
 */
export const chatService = {
    submitMessage,
    upload,
    onInit,
    onDestroy,
    onMetaLoaded,
    onFetchMeta,

    saveSettings,
    debugHistoryOpen,
    debugHistorySelection,
    debugMessagesLoaded,
    debugMessagesError,
    // selectAgent no longer needed in new chat window; keep for compatibility where used
    selectAgent,
    selectModel,
    newConversation,
    classifyMessage,
    normalizeMessages,
    selectFolder,
    receiveMessages,
    // DS event handlers
    onFetchMessages,
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


// Called when meta DS fetch completes; ensures conversations form has defaults when empty
function onMetaLoaded(args) {
    const {context, response, collection} = args
    if (!collection || collection?.length === 0) return;
    const data = collection[0]

    const convCtx = context.Context('conversations');
    const ds = convCtx?.handlers?.dataSource;
    if (!ds) return;
    const current = ds.peekFormData?.() || {};
    const {defaults} = data
    ds.setFormData?.({
        values: {
            ...current,
            agent: defaults.agent || '',
            model: defaults.model || '',
            agentInfo: data.agentInfo || {},
        }
    });

}

// Prevent DS from trying to assign object payload to a collection; return [] so
// the collection path is left untouched and onSuccess can map to the form.
function onFetchMeta(args) {
    const {collection = []} = args;
    console.log('[settings] onFetchMeta', collection);

    const updated = collection.map(data => {

        const agentInfo = data.agentInfo || {};

        console.log('agentInfo', agentInfo)


        const agentsRaw = Array.isArray(data?.agents)
            ? data.agents
            : (data?.agentInfo ? Object.keys(data.agentInfo) : []);

        const modelsRaw = Array.isArray(data?.models)
            ? data.models
            : (defaults?.model ? [defaults.model] : []);

        const toolsRaw = Array.isArray(data?.tools) ? data.tools : [];

        return {
            ...data,
            agentOptions: agentsRaw.map(v => ({
                value: String(v).toLowerCase(),
                label: String(v)
            })),
            agent: data.defaults.agent,

            modelOptions: modelsRaw.map(v => ({
                value: String(v),
                label: String(v)
            })),
            model: data.defaults.model,

            toolOptions: toolsRaw.map(v => ({
                value: String(v),
                label: String(v)
            })),
            tool: agentInfo[data.defaults.agent]?.tools
        };
    });
    return updated;
}

//
