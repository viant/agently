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

// Utility: Safe date → ISO string to avoid invalid time values
const toISOSafe = (v) => {
    if (!v) return new Date().toISOString();
    try {
        const d = new Date(v);
        if (!isNaN(d.getTime())) return d.toISOString();
    } catch (_) { /* ignore */ }
    return new Date().toISOString();
};

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
    // Defer all defaults/agents/tools fetching to when the Settings dialog opens.
    try { console.log('[chat] onInit:start', Date.now()); } catch(_) {}

    // When opened from history, ensure conversations form has id immediately
    try {
        const convCtx = context.Context('conversations');
        const handlers = convCtx?.handlers?.dataSource;
        const form = handlers?.peekFormData?.() || {};
        // Input may carry id directly or wrapped under filter.id
        const inputSig = convCtx?.signals?.input;
        let convID = undefined;
        try { convID = inputSig?.peek?.()?.filter?.id ?? inputSig?.peek?.()?.id; } catch (_) {}
        if (convID && !form.id) {
            handlers.setFormData({ values: { id: convID } });
        }
        // Cache convID to survive brief form clears during mount
        if (convID) {
            context.resources.convID = convID;
        } else if (form.id) {
            context.resources.convID = form.id;
        }
        try { console.log('[chat] onInit:convId', convID, 'form.id', handlers?.peekFormData?.()?.id); } catch(_) {}
        // Kick immediate poll tick if we have convID; otherwise wait briefly for it
        if (handlers?.peekFormData?.()?.id) {
            try { console.log('[chat] onInit:immediate tick (have id)'); } catch(_) {}
            try { context.resources.messagesGraceUntil = Date.now() - 1; } catch(_) {}
            // If we have preloaded rows for this conv, seed the messages DS now
            try {
                const convID = handlers.peekFormData().id;
                const preloaded = context.resources?.preloadedMessages?.[convID];
                if (preloaded && Array.isArray(preloaded) && preloaded.length) {
                    const messagesCtx = context.Context('messages');
                    messagesCtx?.handlers?.dataSource?.setCollection?.({ rows: preloaded });
                    console.log('[chat] onInit:seeded from preloaded', preloaded.length);
                }
            } catch(_) {}
            try { startPolling({ context }); } catch(_) {}
        } else {
            const start = Date.now();
            const waitMs = 1200;
            const timer = setInterval(() => {
                const formNow = handlers?.peekFormData?.() || {};
                const got = formNow.id;
                if (got) {
                    clearInterval(timer);
                    try { console.log('[chat] onInit:convId acquired', got, 'after', Date.now() - start, 'ms'); } catch(_) {}
                    try { context.resources.messagesGraceUntil = Date.now() - 1; } catch(_) {}
                    try {
                        const preloaded = context.resources?.preloadedMessages?.[got];
                        if (preloaded && Array.isArray(preloaded) && preloaded.length) {
                            const messagesCtx = context.Context('messages');
                            messagesCtx?.handlers?.dataSource?.setCollection?.({ rows: preloaded });
                            console.log('[chat] onInit:seeded from preloaded', preloaded.length);
                        }
                    } catch(_) {}
                    try { startPolling({ context }); } catch(_) {}
                } else if ((Date.now() - start) > waitMs) {
                    clearInterval(timer);
                    try { console.log('[chat] onInit:convId not found after', waitMs, 'ms'); } catch(_) {}
                }
            }, 60);
        }
    } catch (e) {
        // ignore – not critical
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
        context.resources['messagesNoopPolls'] = 0;
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
        console.log('[chat][messagesDS] onError at', Date.now(), error);
    } catch (e) {
        // ignore
    }
}

function startPolling({context}) {
    if (!context || typeof context.Context !== 'function') {
        console.warn('chatService.startPolling: invalid context');
        return;
    }
    const tick = async () => {
        const t0 = Date.now();
        if (context.resources.chatTimerState) return;
        try {
            context.resources.chatTimerState = true;

            // Skip polling during initial grace window to avoid duplicate calls
            const now = Date.now();
            const graceUntil = context.resources?.messagesGraceUntil || 0;
            if (now < graceUntil) {
                try { console.log('[chat] poll:skip grace', { now, graceUntil }); } catch(_) {}
                return;
            }

            const convCtx = context.Context('conversations');
            let convID = context.resources?.convID;
            if (!convID) {
                convID = convCtx?.handlers?.dataSource.peekFormData?.()?.id;
                if (convID) context.resources.convID = convID;
            }
            if (!convID) {
                try { console.log('[chat] poll:skip no convID'); } catch(_) {}
                return; // no active conversation – nothing to do
            }

            const messagesCtx = context.Context('messages');
            if (!messagesCtx) {
                try { console.log('[chat] poll:skip no messagesCtx'); } catch(_) {}
                return;
            }

            const collSig = messagesCtx.signals?.collection;
            const ctrlSig = messagesCtx.signals?.control;
            // If the DataSource is already fetching (auto-fetch on input change), skip this tick
            if (ctrlSig?.peek?.()?.loading) {
                try { console.log('[chat] poll:skip DS loading'); } catch(_) {}
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

            // If no messages in UI yet, perform initial load (full transcript)
            const baseRoot = (endpoints.appAPI.baseURL || (typeof window !== 'undefined' ? window.location.origin + '/v1/api' : ''))
                .replace(/\/+$/,'');
            const base = `${baseRoot}/conversations/${encodeURIComponent(convID)}/messages`;
            if (!lastID) {
                const nowTick0 = Date.now();
                const lastTs0 = context.resources?.messagesLastFetchTs || 0;
                const throttleMs0 = context.resources?.messagesPollThrottleMs || 900;
                if ((nowTick0 - lastTs0) >= throttleMs0 && !context.resources?.messagesFetchInFlight) {
                    context.resources.messagesFetchInFlight = true;
                    context.resources.messagesLastFetchTs = nowTick0;
                    try { console.log('[chat] initial:GET', base); console.time('[chat] initial fetch'); } catch(_) {}
                    const json0 = await fetchJSON(base);
                    const conv0 = json0 && json0.status === 'ok' ? json0.data : null;
                    const convStage0 = conv0?.stage || conv0?.Stage;
                    if (convStage0) {
                        setStage({ phase: String(convStage0) });
                    }
                    const transcript0 = Array.isArray(conv0?.transcript) ? conv0.transcript
                                      : Array.isArray(conv0?.Transcript) ? conv0.Transcript : [];
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
                        try { console.log('[chat] initial:rows', rows0.length); } catch(_) {}
                    }
                    try { console.timeEnd('[chat] initial fetch'); } catch(_) {}
                }
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
            const url = `${base}?since=${encodeURIComponent(lastID)}`;
            context.resources.messagesFetchInFlight = true;
            context.resources.messagesLastFetchTs = nowTick;
            try { console.log('[chat] poll:GET', url); console.time('[chat] poll fetch'); } catch(_) {}
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
            try { console.log('[chat] poll:rows', rows.length, 'turns', transcript.length, 'noopPolls', context.resources.messagesNoopPolls); console.timeEnd('[chat] poll fetch'); } catch(_) {}

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
        try { console.log('[chat] poll:end', { elapsedMs: Date.now() - t0 }); } catch(_) {}
    });
}

// Prefetch conversation transcript to seed the chat window before it opens.
export async function preloadConversation({ context, row }) {
    try {
        const convID = row?.id || row?.Id || context?.Context('history')?.handlers?.dataSource?.peekSelection?.()?.selected?.id;
        if (!convID) return false;
        const baseRoot = (endpoints.appAPI.baseURL || (typeof window !== 'undefined' ? window.location.origin + '/v1/api' : ''))
            .replace(/\/+$/,'');
        const base = `${baseRoot}/conversations/${encodeURIComponent(convID)}/messages`;
        console.time('[chat] preload fetch');
        const json = await fetchJSON(base);
        console.timeEnd('[chat] preload fetch');
        const conv = json && json.status === 'ok' ? json.data : null;
        const transcript = Array.isArray(conv?.transcript) ? conv.transcript
                          : Array.isArray(conv?.Transcript) ? conv.Transcript : [];

        const toISOSafe = (v) => {
            if (!v) return new Date().toISOString();
            try { const d = new Date(v); if (!isNaN(d.getTime())) return d.toISOString(); } catch(_) {}
            return new Date().toISOString();
        };

        const rows = [];
        for (const turn of transcript) {
            const turnId = turn?.id || turn?.Id;
            const messages = Array.isArray(turn?.message) ? turn.message
                             : Array.isArray(turn?.Message) ? turn.Message : [];
            // Aggregate steps (thinking/tool/interim) as in polling path
            const steps = [];
            for (const m of messages) {
                const interim = m?.interim ?? m?.Interim;
                const created = m?.createdAt || m?.CreatedAt;
                if (interim) {
                    steps.push({ id: (m.id || m.Id || '') + '/interim', name: 'assistant', reason: 'interim', success: true, startedAt: created, endedAt: created });
                }
                const tc = m?.toolCall || m?.ToolCall;
                const mc = m?.modelCall || m?.ModelCall;
                if (tc) {
                    steps.push({ id: tc.opId || tc.OpId, name: tc.toolName || tc.ToolName, reason: 'tool_call', success: String((tc.status || tc.Status || '')).toLowerCase() === 'completed', startedAt: tc.startedAt || tc.StartedAt, endedAt: tc.completedAt || tc.CompletedAt });
                }
                if (mc) {
                    steps.push({ id: mc.messageId || mc.MessageId, name: mc.model || mc.Model, reason: 'thinking', success: String((mc.status || mc.Status || '')).toLowerCase() === 'completed', startedAt: mc.startedAt || mc.StartedAt, endedAt: mc.completedAt || mc.CompletedAt });
                }
            }
            const normalizedSteps = steps.map(s => {
                const started = s.startedAt ? new Date(s.startedAt) : null;
                const ended = s.endedAt ? new Date(s.endedAt) : null;
                let elapsed = '';
                if (started && ended && !isNaN(started) && !isNaN(ended)) {
                    elapsed = (((ended - started) / 1000).toFixed(2)) + 's';
                }
                return { ...s, elapsed };
            });
            const turnExecutions = normalizedSteps.length ? [{ steps: normalizedSteps }] : [];
            const turnRows = [];
            for (const m of messages) {
                const roleLower = String(m.role || m.Role || '').toLowerCase();
                if (m?.interim || m?.Interim) continue;
                if (roleLower !== 'user' && roleLower !== 'assistant') continue;
                const mc = m?.modelCall || m?.ModelCall;
                const tc = m?.toolCall || m?.ToolCall;
                if (mc || tc) continue;
                const id = m.id || m.Id;
                const createdAt = toISOSafe(m.createdAt || m.CreatedAt);
                const turnIdRef = m.turnId || m.TurnId || turnId;
                turnRows.push({ id, conversationId: m.conversationId || m.ConversationId, role: roleLower, content: m.content || m.Content || '', createdAt, toolName: m.toolName || m.ToolName, turnId: turnIdRef, parentId: turnIdRef, executions: [] });
            }
            // Attach executions to first user or first message
            let carrierIdx = -1;
            for (let i = 0; i < turnRows.length; i++) if (turnRows[i].role === 'user') { carrierIdx = i; break; }
            if (carrierIdx === -1 && turnRows.length) carrierIdx = 0;
            if (carrierIdx >= 0) turnRows[carrierIdx] = { ...turnRows[carrierIdx], executions: turnExecutions };
            for (const r of turnRows) rows.push(r);
        }
        if (!context.resources.preloadedMessages) context.resources.preloadedMessages = {};
        context.resources.preloadedMessages[convID] = rows;
        console.log('[chat] preload:cached rows', rows.length, 'for', convID);
        return true;
    } catch (e) {
        console.error('preloadConversation error', e);
        return false;
    }
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
    try { console.log('[chat] mergeMessages:incoming', Array.isArray(incoming) ? incoming.length : 0); } catch(_) {}
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

    const before = Array.isArray(collSig.value) ? collSig.value.length : 0;
    const next = dedupById(current);
    collSig.value = next;
    try { console.log('[chat] mergeMessages:applied', { before, after: next.length }); } catch(_) {}

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
    try {
        const convId = data?.[0]?.conversationId;
        console.log('[chat] receiveMessages', { count: data.length, sinceId, convId });
    } catch(_) {}
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
    debugHistoryOpen,
    debugHistorySelection,
    debugMessagesLoaded,
    debugMessagesError,
    selectAgent,
    fetchMetaDefaults,
    newConversation,
    classifyMessage,
    normalizeMessages,
    selectFolder,
    buildToolOptions,
    receiveMessages,
    preloadConversation,
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
