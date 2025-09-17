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

// -------------------------------
// Window lifecycle helpers
// -------------------------------

// Utility: Safe date → ISO string to avoid invalid time values
const toISOSafe = (v) => {
    if (!v) return new Date().toISOString();
    try {
        const d = new Date(v);
        if (!isNaN(d.getTime())) return d.toISOString();
    } catch (_) { /* ignore */ }
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
    try { console.log('[chat] onInit:start', Date.now()); } catch(_) {}
    // 1) Pre-fill conversation form defaults
    let defaults = {};
    try { defaults = await fetchMetaDefaults({ context }) || {}; } catch(_) {}


    // 2) Use aggregated meta to preselect tools for the default agent
    try {
        const convCtx = context.Context('conversations');
        const metaCtx = context.Context('meta');
        const meta = await ensureMeta(context);
        const agentTools = meta?.agentTools || {};
        // persist mapping on meta form
        try { metaCtx?.handlers?.dataSource?.setFormData?.({ values: { ...(meta || {}), agentTools } }); } catch(_) {}
        const matchedToolNames = agentTools?.[defaults.agent] || [];
        if (matchedToolNames && convCtx?.handlers?.dataSource?.setFormField) {
            convCtx.handlers.dataSource.setFormField({ item: { id: 'tools' }, value: matchedToolNames });
        }
    } catch(_) {}

    // 3) One-shot: wait briefly for conversations.id then trigger messages fetch
    try {
        const convCtx = context.Context('conversations');
        const handlers = convCtx?.handlers?.dataSource;
        const start = Date.now();
        const deadline = start + 1000;
        const timer = setInterval(() => {
            let convID = '';
            try {
                convID = handlers?.peekFormData?.()?.id || convCtx?.signals?.input?.peek?.()?.id || convCtx?.signals?.input?.peek?.()?.filter?.id;
            } catch(_) {}
            if (convID) {
                clearInterval(timer);
                try { handlers?.setFormData?.({ values: { id: convID } }); } catch(_) {}
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
                        const params = { ...(cur.parameters || {}), convID, since: '' };
                        const next = { ...cur, parameters: params, fetch: true };
                        if (typeof inSig.set === 'function') inSig.set(next); else inSig.value = next;
                        console.log('[chat][signals] set messages.input (initial fetch)', next);
                    }
                } catch(_) {}
                try { dsTick({ context }); } catch(_) {}
                try { installMessagesDebugHooks(context); } catch(_) {}
            } else if (Date.now() > deadline) {
                clearInterval(timer);
            }
        }, 60);
    } catch (_) { /* ignore */ }

    // 4) Start DS-driven refresh loop (no external poller logic).
    try {
        if (context.resources?.chatTimer) {
            clearInterval(context.resources.chatTimer);
        }
        context.resources = context.resources || {};
        context.resources.chatTimer = setInterval(() => dsTick({ context }), 1000);
    } catch (_) { /* ignore */ }
}

// DS-driven refresh: computes since and invokes DS getCollection with input parameters
async function dsTick({ context }) {
    try {
        const convCtx = context.Context('conversations');
        const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id;
        if (!convID) { try { console.log('[chat][dsTick] skip: no convID'); } catch(_) {} ; return; }
        const messagesCtx = context.Context('messages');
        if (!messagesCtx) { try { console.log('[chat][dsTick] skip: no messagesCtx'); } catch(_) {} ; return; }
        const ctrl = messagesCtx.signals?.control;
        if (ctrl?.peek?.()?.loading) { try { console.log('[chat][dsTick] skip: DS loading'); } catch(_) {} ; return; }
        const coll = Array.isArray(messagesCtx.signals?.collection?.value) ? messagesCtx.signals.collection.value : [];
        let since = '';
        for (let i = coll.length - 1; i >= 0; i--) { if (coll[i]?.turnId) { since = coll[i].turnId; break; } }
        if (!since && coll.length) { since = coll[coll.length - 1]?.id || ''; }
        // Gate fetch to avoid toggling loading and input flicker.
        const reqSig = `${convID}|${since}`;
        const initialFetched = !!context.resources?.dsInitialMessagesFetched;
        if (Array.isArray(coll) && coll.length > 0) {
            context.resources = context.resources || {};
            context.resources.dsInitialMessagesFetched = true;
        }
        if (context.resources?.lastDsReqSig === reqSig && initialFetched) {
            try { console.log('[chat][dsTick] skip: unchanged since', { since }); } catch(_) {}
            return;
        }
        context.resources = context.resources || {};
        context.resources.lastDsReqSig = reqSig;

        // Update DS input and mark fetch=true so DS effect triggers doFetchRecords
        const inSig = messagesCtx.signals?.input;
        if (inSig) {
            const cur = (typeof inSig.peek === 'function') ? (inSig.peek() || {}) : (inSig.value || {});
            const params = { ...(cur.parameters || {}), convID, since };
            const next = { ...cur, parameters: params, fetch: true };
            if (typeof inSig.set === 'function') inSig.set(next); else inSig.value = next;
            try { console.log('[chat][signals] update messages.input (fetch)', next); } catch(_) {}
        }
        // Trigger fetch via signals; DataSource.useSignalEffect will handle network call
        // when input.fetch == true and dependencies are satisfied.
    } catch (e) {
        console.warn('dsTick error', e);
    }
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
                ctrlSig.value = { ...ctrlVal, error: coerced };
            }
            if (len !== lastLen || loading !== lastLoading) {
                console.log('[chat][signals] messages', { len, loading, ts: Date.now() });
                lastLen = len;
                lastLoading = loading;
            }
        } catch(_) {}
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
            console.log('[chat][connector][GET][done] messages', { status: res?.status, keys: Object.keys(res || {}) });
            return res;
        };
    }
    if (origPost) {
        conn.post = async (opts) => {
            console.log('[chat][connector][POST] messages', opts);
            const res = await origPost(opts);
            console.log('[chat][connector][POST][done] messages', { status: res?.status, dataKeys: Object.keys(res?.data || {}) });
            return res;
        };
    }
}

function selectFolder(props) {
    const {context, selected} = props;
    console.log('selectFolder 1', context)

    context.handlers.dialog.commit()
    console.log('selectFolder', props)
}

// --------------------------- Debug helpers ------------------------------

export function debugHistoryOpen({ context }) {
    try {
        const convCtx = context.Context('history') || context.Context('conversations');
        const selected = convCtx?.handlers?.dataSource?.peekSelection?.()?.selected || {};
        const id = selected?.id;
        console.log('[chat][history] open click at', Date.now(), 'convId:', id, 'row:', selected);
    } catch (e) {
        console.log('[chat][history] open click log error', e);
    }
}

export function debugHistorySelection({ context }) {
    try {
        const convCtx = context.Context('history') || context.Context('conversations');
        const sel = convCtx?.handlers?.dataSource?.peekSelection?.();
        console.log('[chat][history] selection', Date.now(), sel);
    } catch (e) {
        console.log('[chat][history] selection log error', e);
    }
}

export function debugMessagesLoaded({ context, response }) {
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

export function debugMessagesError({ context, error }) {
    try {
        const msgCtx = context.Context('messages');
        const ctrl = msgCtx?.signals?.control;
        if (ctrl) {
            const prev = (typeof ctrl.peek === 'function') ? (ctrl.peek() || {}) : (ctrl.value || {});
            const coerced = String(error?.message || error);
            ctrl.value = { ...prev, error: coerced, loading: false };
        }
        console.log('[chat][messagesDS] onError at', Date.now(), error);
    } catch (e) {
        // ignore
    }
}

// Ensure DS-driven refresh does not shrink the displayed collection by
// restoring the last known full snapshot captured by the polling path.
export function hydrateMessagesCollection({ context }) {
    try {
        const messagesCtx = context.Context('messages');
        const collSig = messagesCtx?.signals?.collection;
        const snap = messagesCtx && messagesCtx._snapshot;
        if (!collSig || !Array.isArray(snap) || snap.length === 0) return;
        const curr = Array.isArray(collSig.value) ? collSig.value : [];
        if (curr.length < snap.length) {
            collSig.value = [...snap];
            try { console.log('[chat] hydrateMessagesCollection: restored snapshot', { before: curr.length, after: snap.length }); } catch(_) {}
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
        const { collection, context } = props || {};
        const transcript = Array.isArray(collection) ? collection : [];

        // Flatten turns to rows
        const toISOSafe = (v) => {
            if (!v) return new Date().toISOString();
            try { const d = new Date(v); return isNaN(d.getTime()) ? new Date().toISOString() : d.toISOString(); } catch(_) { return new Date().toISOString(); }
        };
        const rows = [];
        for (const turn of transcript) {
            const turnId = turn?.id || turn?.Id;
            const msgs = Array.isArray(turn?.message) ? turn.message : (Array.isArray(turn?.Message) ? turn.Message : []);
            for (const m of msgs) {
                const interim = m?.interim ?? m?.Interim;
                const role = String(m?.role || m?.Role || '').toLowerCase();
                if (interim) continue;
                if (role !== 'user' && role !== 'assistant') continue;
                rows.push({
                    id: m.id || m.Id,
                    conversationId: m.conversationId || m.ConversationId,
                    role,
                    content: m.content || m.Content || '',
                    createdAt: toISOSafe(m.createdAt || m.CreatedAt),
                    toolName: m.toolName || m.ToolName,
                    turnId: m.turnId || m.TurnId || turnId,
                    parentId: m.turnId || m.TurnId || turnId,
                    executions: [],
                });
            }
        }

        // Append-only merge with existing DS rows
        let prev = [];
        try {
            const msgCtx = context?.Context?.('messages');
            prev = Array.isArray(msgCtx?.signals?.collection?.peek?.()) ? msgCtx.signals.collection.peek() : [];
        } catch(_) {}

        const seen = new Set((prev || []).map(r => r && r.id).filter(Boolean));
        const merged = [...prev];
        for (const r of rows) {
            if (!r || !r.id || seen.has(r.id)) continue;
            seen.add(r.id);
            merged.push(r);
        }
        return merged;
    } catch (e) {
        console.warn('onFetchMessages error', e);
        return [];
    }
}

// onSuccess handler: merges response conversation transcript rows into DS collection.
export function mergeFromResponse({ context, response }) {
    try {
        const conv = response?.data || response?.Data || response?.conversation || response?.Conversation || response;
        if (!conv) return;
        const transcript = Array.isArray(conv?.transcript) ? conv.transcript
                          : Array.isArray(conv?.Transcript) ? conv.Transcript : [];
        try { console.log('[chat][mergeFromResponse] transcript size', transcript.length); } catch(_) {}
        const rows = [];
        for (const turn of transcript) {
            const turnId = turn?.id || turn?.Id;
            const messages = Array.isArray(turn?.message) ? turn.message
                             : Array.isArray(turn?.Message) ? turn.Message : [];
            const usage = turn?.usage || turn?.Usage;
            const turnExecutions = [];
            for (const m of messages) {
                const interim = m?.interim ?? m?.Interim;
                const roleLower = String(m.role || m.Role || '').toLowerCase();
                const hasCall = !!(m?.toolCall || m?.ToolCall || m?.modelCall || m?.ModelCall);
                if (interim) {
                    const created = m?.createdAt || m?.CreatedAt;
                    turnExecutions.push({ id: (m.id || m.Id || '') + '/interim', name: 'assistant', reason: 'interim', success: true, startedAt: created, endedAt: created });
                    continue;
                }
                if (hasCall) continue;
                if (roleLower !== 'user' && roleLower !== 'assistant') continue;
                const id = m.id || m.Id;
                const createdAt = toISOSafe(m.createdAt || m.CreatedAt);
                const turnIdRef = m.turnId || m.TurnId || turnId;
                rows.push({ id, conversationId: m.conversationId || m.ConversationId, role: roleLower, content: m.content || m.Content || '', createdAt, toolName: m.toolName || m.ToolName, turnId: turnIdRef, parentId: turnIdRef, executions: [], usage, elicitation: m.elicitation || m.Elicitation });
            }
            // Attach turn executions to a single carrier row
            if (turnExecutions.length && rows.length) {
                const idx = rows.findIndex(r => r.turnId === turnId && r.role === 'user');
                const cidx = idx >= 0 ? idx : rows.findIndex(r => r.turnId === turnId);
                if (cidx >= 0) {
                    rows[cidx] = { ...rows[cidx], executions: turnExecutions };
                }
            }
        }
        const messagesCtx = context.Context('messages');
        receiveMessages(messagesCtx, rows);
    } catch (e) {
        console.warn('mergeFromResponse error', e);
    }
}

// Deprecated – DataSource now owns fetching; retained for reference but unused
function startPolling({context}) {
    if (!context || typeof context.Context !== 'function') {
        console.warn('chatService.startPolling: invalid context');
        return;
    }
    const tick = async () => {
        const t0 = Date.now();
        const seq = (context.resources.pollSeq = (context.resources.pollSeq || 0) + 1);
        if (context.resources.chatTimerState) return;
        try {
            context.resources.chatTimerState = true;

            // Skip polling during initial grace window to avoid duplicate calls
            const now = Date.now();
            const graceUntil = context.resources?.messagesGraceUntil || 0;
            if (now < graceUntil) {
                try { console.log('[chat][poll]', seq, 'skip grace', { now, graceUntil, remainMs: (graceUntil - now) }); } catch(_) {}
                return;
            }

            const convCtx = context.Context('conversations');
            let convID = context.resources?.convID;
            if (!convID) {
                convID = convCtx?.handlers?.dataSource.peekFormData?.()?.id;
                if (convID) context.resources.convID = convID;
            }
            if (!convID) {
                try { console.log('[chat][poll]', seq, 'skip no convID'); } catch(_) {}
                return; // no active conversation – nothing to do
            }

            const messagesCtx = context.Context('messages');
            if (!messagesCtx) {
                try { console.log('[chat][poll]', seq, 'skip no messagesCtx'); } catch(_) {}
                return;
            }

            const collSig = messagesCtx.signals?.collection;
            const ctrlSig = messagesCtx.signals?.control;
            // If the DataSource is already fetching (auto-fetch on input change), skip this tick
            if (ctrlSig?.peek?.()?.loading) {
                try { console.log('[chat][poll]', seq, 'skip DS loading'); } catch(_) {}
                return;
            }
            let current = Array.isArray(collSig?.value) ? collSig.value : [];
            try { console.log('[chat][poll]', seq, 'collLen', current.length); } catch(_) {}
            // Prevent UI blink if DS altered collection (cleared or shrunk) by restoring last snapshot
            try {
                if (context.resources?.messagesDidInitialFetch) {
                    const snap = messagesCtx && messagesCtx._snapshot;
                    if (Array.isArray(snap) && snap.length > 0 && (!current || current.length < snap.length)) {
                        collSig.value = [...snap];
                        current = collSig.value;
                        console.log('[chat][poll]', seq, 'restored snapshot to prevent blink', { size: current.length });
                    }
                }
            } catch(_) {}
            // Prefer last turnId if present; fallback to last message id
            let lastID = '';
            for (let i = current.length - 1; i >= 0; i--) {
                if (current[i]?.turnId) { lastID = current[i].turnId; break; }
            }
            if (!lastID && current.length) {
                lastID = current[current.length - 1].id;
            }
            // Fallback to stored last seen turnId to survive transient collection clears
            if (!lastID) {
                const stored = context.resources?.messagesLastTurnId || '';
                if (stored) lastID = stored;
            }

            // If no messages in UI yet, perform initial load (full transcript)
            const messagesAPI = messagesCtx.connector;
            if (!lastID && !context.resources?.messagesDidInitialFetch && !initialFetchDoneByConv.has(convID)) {
                const nowTick0 = Date.now();
                const lastTs0 = context.resources?.messagesLastFetchTs || 0;
                const throttleMs0 = context.resources?.messagesPollThrottleMs || 900;
                if ((nowTick0 - lastTs0) >= throttleMs0 && !context.resources?.messagesFetchInFlight) {
                    context.resources.messagesFetchInFlight = true;
                    context.resources.messagesLastFetchTs = nowTick0;
                    try { console.log('[chat][initial]', seq, 'DS GET', {convID, since: ''}); console.time(`[chat][initial] seq=${seq}`); } catch(_) {}
                    const json0 = await messagesAPI.get({ inputParameters: { convID, since: '' } });
                    const conv0 = json0 && (json0.data ?? json0.Data ?? json0.conversation ?? json0.Conversation ?? json0);
                    const convStage0 = conv0?.stage || conv0?.Stage;
                    if (convStage0) {
                        setStage({ phase: String(convStage0) });
                    }
                    const transcript0 = Array.isArray(conv0?.transcript) ? conv0.transcript
                                      : Array.isArray(conv0?.Transcript) ? conv0.Transcript : [];
                    try { console.log('[chat][initial] transcript size', transcript0.length); } catch(_) {}
                    const rows0 = [];
                    // reuse existing turn → rows mapping logic by mimicking transcript var
                    for (const turn of transcript0) {
                        const turnId = turn?.id || turn?.Id;
                        const messages = Array.isArray(turn?.message) ? turn.message
                                         : Array.isArray(turn?.Message) ? turn.Message : [];
                        // Minimal mapping: push rows without executions; let subsequent tick compute executions
                        for (const m of messages) {
                            const roleLower = String(m.role || m.Role || '').toLowerCase();
                            if (roleLower !== 'user' && roleLower !== 'assistant') continue;
                            if (m?.interim || m?.Interim) continue;
                            rows0.push({
                                id: m.id || m.Id,
                                conversationId: m.conversationId || m.ConversationId,
                                role: roleLower,
                                content: m.content || m.Content || '',
                                createdAt: toISOSafe(m.createdAt || m.CreatedAt),
                                toolName: m.toolName || m.ToolName,
                                turnId: m.turnId || m.TurnId || turnId,
                                parentId: m.turnId || m.TurnId || turnId,
                                executions: [],
                            });
                        }
                    }
                    if (rows0.length) {
                        receiveMessages(messagesCtx, rows0, '');
                        try { console.log('[chat][initial]', seq, 'rows', rows0.length, 'turns', transcript0.length); } catch(_) {}
                    }
                    // Track newest turnId from initial transcript
                    let newestTurnId0 = '';
                    for (let i = transcript0.length - 1; i >= 0; i--) {
                        const t = transcript0[i];
                        newestTurnId0 = (t?.id || t?.Id || newestTurnId0);
                        if (newestTurnId0) break;
                    }
                    context.resources.messagesLastTurnId = newestTurnId0 || context.resources.messagesLastTurnId || '';
                    context.resources.messagesDidInitialFetch = true;
                    initialFetchDoneByConv.add(convID);
                    try { console.timeEnd(`[chat][initial] seq=${seq}`); } catch(_) {}
                }
                return;
            }
            // If we have already done an initial fetch but still no lastID (e.g., transient clears), skip this tick
            if (!lastID) {
                try { console.log('[chat][poll]', seq, 'skip no lastID (post-initial)'); } catch(_) {}
                return;
            }

            // Throttle and de-dupe in-flight calls
            const nowTick = Date.now();
            const lastTs = context.resources?.messagesLastFetchTs || 0;
            const baseThrottle = context.resources?.messagesPollThrottleMs || 900;
            const noopCount = context.resources?.messagesNoopPolls || 0;
            const throttleMs = Math.min(5000, baseThrottle + (noopCount * 600));
            if ((nowTick - lastTs) < throttleMs) {
                try { console.log('[chat] poll:skip throttle'); } catch(_) {}
                return;
            }
            if (context.resources?.messagesFetchInFlight) {
                try { console.log('[chat] poll:skip inflight'); } catch(_) {}
                return;
            }

            // Fetch rich conversation view (v2) via backend proxy on v1 path
            const url = { convID, since: lastID };
            context.resources.messagesFetchInFlight = true;
            context.resources.messagesLastFetchTs = nowTick;
            try { console.log('[chat][poll]', seq, 'DS GET', url); console.time(`[chat][poll] seq=${seq}`); } catch(_) {}
            const json = await messagesAPI.get({ inputParameters: url });

            // New format: data is a Conversation object with transcript and stage (Go fields are Capitalized)
            const conv = json && (json.data ?? json.Data ?? json.conversation ?? json.Conversation ?? json);
            const convStage = conv?.stage || conv?.Stage;
            if (convStage) {
                setStage({ phase: String(convStage) });
            }
            const transcript = Array.isArray(conv?.transcript) ? conv.transcript
                              : Array.isArray(conv?.Transcript) ? conv.Transcript : [];
            const rows = [];


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
                    const createdAt = toISOSafe(m.createdAt || m.CreatedAt);
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
            // Adjust noop backoff: if newest turnId didn't change, increase; otherwise reset
            let newestTurnId = '';
            for (let i = transcript.length - 1; i >= 0; i--) {
                const t = transcript[i];
                newestTurnId = (t?.id || t?.Id || newestTurnId);
                if (newestTurnId) break;
            }
            if (newestTurnId && newestTurnId === lastID) {
                context.resources.messagesNoopPolls = Math.min((context.resources.messagesNoopPolls || 0) + 1, 10);
            } else {
                context.resources.messagesNoopPolls = 0;
            }
            // Persist newest turnId for resilience across DS resets
            if (newestTurnId) {
                context.resources.messagesLastTurnId = newestTurnId;
            }
            try {
                console.log('[chat][poll]', seq, 'rows', rows.length, 'turns', transcript.length, 'noopPolls', context.resources.messagesNoopPolls, 'newestTurnId', newestTurnId, 'lastID', lastID);
                console.timeEnd(`[chat][poll] seq=${seq}`);
            } catch(_) {}

            // Determine id of newest assistant message after merge (kept for future features)
            // const messages = messagesCtx.signals?.collection?.value || [];
            // let newestAssistantID = '';
            // for (let i = messages.length - 1; i >= 0; i--) {
            //     if (messages[i].role === 'assistant') {
            //         newestAssistantID = messages[i].id;
            //         break;
            //     }
            // }

        } catch (err) {
            console.error('chatService.startPolling tick error:', err);
        }
    };
    tick().then(() => {}).finally(() => {
        context.resources.chatTimerState = false;
        context.resources.messagesFetchInFlight = false;
        try { console.log('[chat][poll] end', { elapsedMs: Date.now() - t0 }); } catch(_) {}
        try {
            const now2 = Date.now();
            const desired = (now2 > (context.resources.stabilizationUntil || 0)) ? 1000 : 250;
            if ((context.resources.pollIntervalMs || 0) !== desired) {
                clearInterval(context.resources.chatTimer);
                context.resources.pollIntervalMs = desired;
                context.resources.chatTimer = setInterval(() => startPolling({ context }), desired);
                console.log('[chat][poll] interval adjusted to', desired, 'ms');
            }
        } catch(_) {}
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

/**
 * Builds options for a simple select from aggregated meta list (agents/models/tools).
 * args[0] should be one of: 'agents' | 'models' | 'tools'.
 */
export async function buildOptionsFromMeta({ context, args }) {
    const listName = Array.isArray(args) && args[0] ? String(args[0]) : '';
    if (!listName) return { options: [] };
    const meta = await ensureMeta(context);
    const list = Array.isArray(meta?.[listName]) ? meta[listName] : [];
    const options = list.map(v => ({ id: String(v), label: String(v), value: String(v) }));
    return { options };
}

/**
 * Builds tool options from aggregated meta tools list.
 */
export async function buildToolOptionsFromMeta({ context }) {
    return buildOptionsFromMeta({ context, args: ['tools'] });
}

// Applies meta.agentTools mapping to the conversations.tools field when agent changes
export async function applyAgentToolsFromMeta({ context, selected }) {
    try {
        const meta = await ensureMeta(context);
        const agentTools = meta?.agentTools || {};
        const convCtx = context.Context('conversations');
        const toolNames = agentTools?.[selected] || [];
        if (convCtx?.handlers?.dataSource?.setFormField) {
            convCtx.handlers.dataSource.setFormField({ item: { id: 'tools' }, value: toolNames });
        }
        return true;
    } catch (e) {
        console.warn('applyAgentToolsFromMeta error', e);
        return false;
    }
}

async function ensureMeta(context) {
    // Prefer cached result on window context.
    if (context?.resources?.metaPayload) {
        return context.resources.metaPayload;
    }
    const metaContext = context?.Context?.('meta');
    const metaAPI = metaContext?.connector;
    try {
        const resp = await metaAPI?.get?.({});
        const data = (resp && typeof resp === 'object' && 'data' in resp) ? resp.data : resp;
        if (data) {
            // Cache on context and try to persist in DS form for other handlers.
            context.resources = context.resources || {};
            context.resources.metaPayload = data;
            try { metaContext?.handlers?.dataSource?.setFormData?.({ values: data }); } catch(_) {}
            return data;
        }
    } catch (err) {
        console.error('ensureMeta error', err);
    }
    return {};
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

        // Ask DS to refresh from backend so DataSource stays the single source of truth
        try { await messageHandlers?.getCollection?.(); } catch(_) {}

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
        const data = (resp && typeof resp === 'object' && 'data' in resp) ? resp.data : resp;
        if (!data) return;
        const defaults = data?.defaults || {};
        const agent = defaults.agent || '';
        const model = defaults.model || '';
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
    try { console.log('[chat] mergeMessages:incoming', Array.isArray(incoming) ? incoming.length : 0); } catch(_) {}
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
        try { messagesContext._snapshot = [...next]; } catch(_) {}
        try { console.log('[chat] mergeMessages:applied', { before, after: next.length }); } catch(_) {}
        // Keep the DataSource form in sync with the newest assistant chunk
        const last = next[next.length - 1] || {};
        messagesContext?.handlers?.dataSource?.setFormData?.({ values: { ...last } });
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
        console.log('[chat] receiveMessages', { count: data.length, sinceId, convId });
    } catch(_) {}
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
    debugHistoryOpen,
    debugHistorySelection,
    debugMessagesLoaded,
    debugMessagesError,
    // selectAgent no longer needed in new chat window; keep for compatibility where used
    selectAgent,
    fetchMetaDefaults,
    newConversation,
    classifyMessage,
    normalizeMessages,
    selectFolder,
    buildToolOptions,
    buildOptionsFromMeta,
    buildToolOptionsFromMeta,
    applyAgentToolsFromMeta,
    receiveMessages,
    // DS event handlers
    onFetchMessages,
    mergeFromResponse,
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
