// Chat service helper used by forge SettingProvider.
// Contains submitMessage implementation extracted from App.jsx to keep
// App clean and focused on composition.

import {endpoints} from '../endpoint';
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
import {FormRenderer} from 'forge';
import ElicitionForm from '../components/ElicitionForm.jsx';
import MCPInteraction from '../components/MCPInteraction.jsx';
import PolicyApproval from '../components/PolicyApproval.jsx';
import {poll} from './utils/apiUtils';
import {classifyMessage, normalizeMessages, isSimpleTextSchema} from './messageNormalizer';

import ExecutionBubble from '../components/chat/ExecutionBubble.jsx';
import ToolFeed from '../components/chat/ToolFeed.jsx';
import HTMLTableBubble from '../components/chat/HTMLTableBubble.jsx';
import {ensureConversation, newConversation} from './conversationService';
import SummaryNote from '../components/chat/SummaryNote.jsx';
import {setStage} from '../utils/stageBus.js';
import {setComposerBusy} from '../utils/composerBus.js';
import { setExecutionDetailsEnabled, setToolFeedEnabled, getExecutionDetailsEnabled, getToolFeedEnabled } from '../utils/execFeedBus.js';

// Module-level stash for uploads to avoid relying on mutable message object
let pendingUploads = [];

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
        // Prevent DS loading from disabling composer or showing Abort during initial fetch
        context.resources = context.resources || {};
        context.resources.suppressMessagesLoading = true;
        try {
            const msgCtx = context.Context('messages');
            const ctrlSig = msgCtx?.signals?.control;
            if (ctrlSig) {
                const prev = (typeof ctrlSig.peek === 'function') ? (ctrlSig.peek() || {}) : (ctrlSig.value || {});
                ctrlSig.value = {...prev, loading: false};
                // eslint-disable-next-line no-console
                console.debug('[chat][init] suppressMessagesLoading=true; control.loading=false');
            }
            // Also ensure conversations form has running=false initially
            try {
                const convCtx = context.Context('conversations');
                convCtx?.handlers?.dataSource?.setFormField?.({ item: { id: 'running' }, value: false });
                console.debug('[chat][init] conversations.running=false');
            } catch(_) {}
        } catch(_) {}

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
                        log.debug('[chat][signals] set messages.input (initial fetch)', next);
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
            log.warn('dsTick poll error', e);
        }
    } catch (e) {
        log.warn('dsTick error', e);
    }
}

// --------------------------- Transcript → rows helpers ------------------------------

function buildThinkingStepFromModelCall(mc) {
    if (!mc) return null;
    const status = String((mc.status || mc.Status || '')).toLowerCase();
    return {
        id: mc.messageId || mc.MessageId,
        name: mc.model || mc.Model,
        provider: mc.provider || mc.Provider,
        model: mc.model || mc.Model,
        finishReason: mc.finishReason || mc.FinishReason,
        errorCode: mc.errorCode || mc.ErrorCode,
        reason: 'thinking',
        success: status === 'completed',
        statusText: status,
        error: mc.errorMessage || mc.ErrorMessage || '',
        startedAt: mc.startedAt || mc.StartedAt,
        endedAt: mc.completedAt || mc.CompletedAt,
        promptTokens: mc.promptTokens ?? mc.PromptTokens,
        promptCachedTokens: mc.promptCachedTokens ?? mc.PromptCachedTokens,
        promptAudioTokens: mc.promptAudioTokens ?? mc.PromptAudioTokens,
        completionTokens: mc.completionTokens ?? mc.CompletionTokens,
        completionReasoningTokens: mc.completionReasoningTokens ?? mc.CompletionReasoningTokens,
        completionAudioTokens: mc.completionAudioTokens ?? mc.CompletionAudioTokens,
        totalTokens: mc.totalTokens ?? mc.TotalTokens,
        requestPayloadId: mc.requestPayloadId || mc.RequestPayloadId,
        responsePayloadId: mc.responsePayloadId || mc.ResponsePayloadId,
        streamPayloadId: mc.streamPayloadId || mc.StreamPayloadId,
        providerRequestPayloadId: mc.providerRequestPayloadId || mc.ProviderRequestPayloadId,
        providerResponsePayloadId: mc.providerResponsePayloadId || mc.ProviderResponsePayloadId,
    };
}

function buildToolStepFromToolCall(tc) {
    if (!tc) return null;
    const status = String((tc.status || tc.Status || '')).toLowerCase();
    return {
        id: tc.opId || tc.OpId,
        name: tc.toolName || tc.ToolName,
        toolName: tc.toolName || tc.ToolName,
        reason: 'tool_call',
        success: status === 'completed',
        statusText: status,
        error: tc.errorMessage || tc.ErrorMessage || '',
        errorCode: tc.errorCode || tc.ErrorCode,
        attempt: typeof (tc.attempt ?? tc.Attempt) === 'number' ? (tc.attempt ?? tc.Attempt) : undefined,
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
    // Determine the very last message id to decide whether to suppress the inline
    // elicitation step (last message should open the form dialog instead of a step).
    let globalLastMsgId = '';
    let globalLastTurnId = '';
    try {
        const lastTurn = transcript && transcript.length ? (transcript[transcript.length - 1]) : null;
        const lastTurnMsgs = lastTurn ? (Array.isArray(lastTurn?.message) ? lastTurn.message : (Array.isArray(lastTurn?.Message) ? lastTurn.Message : [])) : [];
        if (lastTurnMsgs.length) {
            const lm = lastTurnMsgs[lastTurnMsgs.length - 1];
            globalLastMsgId = lm?.id || lm?.Id || '';
        }
        globalLastTurnId = lastTurn?.id || lastTurn?.Id || '';
    } catch(_) {}
    // Track most recent elicitation step across turns to catch user replies
    let recentElicitationStep = null;
    for (const turn of transcript) {
        const turnId = turn?.id || turn?.Id;
        const isLastTurn = !!globalLastTurnId && (turnId === globalLastTurnId);
        const turnStatus = String(turn?.status || turn?.Status || '').toLowerCase();
        const turnError = (turn?.errorMessage || turn?.ErrorMessage || '') + '';
        const turnCreatedAt = toISOSafe(turn?.createdAt || turn?.CreatedAt);
        const turnUpdatedAt = toISOSafe(turn?.updatedAt || turn?.UpdatedAt || turn?.completedAt || turn?.CompletedAt || turn?.createdAt || turn?.CreatedAt);
        const turnElapsedSecRaw = (turn?.elapsedInSec ?? turn?.ElapsedInSec);
        const turnElapsedSec = (typeof turnElapsedSecRaw === 'number' && isFinite(turnElapsedSecRaw) && turnElapsedSecRaw >= 0) ? Math.floor(turnElapsedSecRaw) : undefined;
        const messages = Array.isArray(turn?.message) ? turn.message
            : Array.isArray(turn?.Message) ? turn.Message : [];

        // Gather elicitation inline user bodies in this turn for reliable suppression
        const elicitationUserBodies = new Set();
        try {
            for (const m of messages) {
                const body = m?.UserElicitationData?.InlineBody || m?.userElicitationData?.InlineBody;
                if (typeof body === 'string' && body.trim()) {
                    elicitationUserBodies.add(body.trim());
                }
            }
        } catch(_) {}

        // 1) Build all execution steps in this turn (model/tool/interim)
        const steps = [];
        let lastElicitationStep = null;
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
            // Elicitation step – include except when it is the last message and still pending
            let s3 = null;
            const roleLower2 = String(m.role || m.Role || '').toLowerCase();
            const typeLower2 = String(m.type || m.Type || '').toLowerCase();
            const status2 = String(m.status || m.Status || '').toLowerCase();
            const payloadId = m.elicitationPayloadId || m.ElicitationPayloadId || m.payloadId || m.PayloadId;
            if (roleLower2 === 'assistant' || roleLower2 === 'tool') {
                // parse elicitation content to confirm presence and capture schema/message
                let elic = null;
                try {
                    const maybe = typeof (m.content || m.Content) === 'string' ? JSON.parse(m.content || m.Content || '') : (m.content || m.Content);
                    if (maybe && typeof maybe === 'object' && (maybe.requestedSchema || maybe.elicitationId)) {
                        elic = maybe;
                    }
                } catch(_) {}
                if (elic) {
                    const created = m?.createdAt || m?.CreatedAt;
                    const updated = m?.updatedAt || m?.UpdatedAt || created;
                    const isLast = (m.id || m.Id) === globalLastMsgId;
                    // For assistant: include unless it is last AND pending; for tool: include always (step timeline)
                    const includeNow = (roleLower2 === 'assistant') ? (!isLast || (status2 && status2 !== 'pending')) : true;
                    if (includeNow) {
                        s3 = {
                            id: (m.id || m.Id || '') + '/elicitation',
                            name: 'elicitation',
                            reason: 'elicitation',
                            successBool: status2 === 'accepted',
                            statusText: status2 || 'pending',
                            originRole: roleLower2,
                            startedAt: created,
                            endedAt: updated,
                            responsePayloadId: payloadId,
                            elicitationPayloadId: payloadId,
                            elicitation: {
                                message: elic.message || elic.prompt,
                                requestedSchema: elic.requestedSchema,
                                url: elic.url,
                                callbackURL: elic.callbackURL || (m.callbackURL || m.CallbackURL),
                                elicitationId: elic.elicitationId || elic.ElicitationId,
                            },
                        };
                        // Best-effort inline user data carried by message
                        try {
                            const udataRaw = m?.UserElicitationData?.InlineBody || m?.userElicitationData?.InlineBody;
                            if (udataRaw) {
                                try { s3.userData = JSON.parse(udataRaw); } catch(_) { s3.userData = udataRaw; }
                            }
                        } catch(_) {}
                        lastElicitationStep = s3;
                        recentElicitationStep = s3;
                    }
                }
            }
            if (s1) steps.push({...s1, elapsed: computeElapsed(s1)});
            if (s2) steps.push({...s2, elapsed: computeElapsed(s2)});
            if (s3) steps.push({...s3, elapsed: computeElapsed(s3)});
        }

        // Sort steps by timestamp (prefer startedAt, fallback endedAt)
        steps.sort((a, b) => {
            const ta = a?.startedAt || a?.endedAt || '';
            const tb = b?.startedAt || b?.endedAt || '';
            const da = ta ? new Date(ta).getTime() : 0;
            const db = tb ? new Date(tb).getTime() : 0;
            return da - db;
        });

        // 2) Build visible chat rows:
        //    - user/assistant messages (non-interim, skip call-only entries)
        //    - plus control elicitations (assistant or tool) so the form/modal can render
        const turnRows = [];
        for (const m of messages) {
            const roleLower = String(m.role || m.Role || '').toLowerCase();
            const isInterim = !!(m?.interim ?? m?.Interim);
            const hasCall = !!(m?.toolCall || m?.ToolCall || m?.modelCall || m?.ModelCall);
            let suppressBubble = false;
            // Detect and attach a user reply to the most recent elicitation step within this turn
            if (roleLower === 'user') {
                try {
                    const txt = m?.content || m?.Content || '';
                    const maybe = typeof txt === 'string' && txt.trim().startsWith('{') ? JSON.parse(txt) : null;
                    const target = lastElicitationStep || recentElicitationStep;
                    if (maybe && target && !target.userData) {
                        target.userData = maybe;
                        try { target.replyMessageId = m.id || m.Id; } catch(_) {}
                        suppressBubble = true;
                    }
                    // Also suppress when user content equals any elicitation inline body in this turn
                    if (!suppressBubble && typeof txt === 'string' && elicitationUserBodies.has(txt.trim())) {
                        suppressBubble = true;
                    }
                } catch(_) {}
            }
            if (isInterim) continue;
            if (hasCall) continue; // call content is represented in steps
            if (suppressBubble) continue; // answered elicitation → execution details only

            const id = m.id || m.Id;
            const createdAt = toISOSafe(m.createdAt || m.CreatedAt);
            const turnIdRef = m.turnId || m.TurnId || turnId;

            // Try to detect elicitation payload embedded as JSON content on control messages
            let elic = m.elicitation || m.Elicitation;
            let callbackURL = m.callbackURL || m.CallbackURL;
            // Inspect content for serialized elicitation
            if (!elic) {
                try {
                    const maybe = typeof (m.content || m.Content) === 'string' ? JSON.parse(m.content || m.Content || '') : (m.content || m.Content);
                    if (maybe && typeof maybe === 'object' && (maybe.requestedSchema || maybe.elicitationId)) {
                        elic = {
                            elicitationId: maybe.elicitationId,
                            message: maybe.message || maybe.prompt || '',
                            requestedSchema: maybe.requestedSchema || {},
                            url: maybe.url || '',
                            mode: maybe.mode || '',
                        };
                        if (!callbackURL && typeof maybe.callbackURL === 'string') {
                            callbackURL = maybe.callbackURL;
                        }
                        try { console.debug('[ElicitationDetect]', {id, role: roleLower, status: m.status || m.Status, mode: elic.mode, url: elic.url, hasSchema: !!elic.requestedSchema}); } catch(_) {}
                    }
                } catch (_) {}
            }

            const isControlElicitation = (String(m.type || m.Type || '').toLowerCase() === 'control') && !!elic;

            const status = m.status || m.Status || '';
            if (!isControlElicitation && roleLower !== 'user' && roleLower !== 'assistant') continue;
            // Control elicitation visibility policy:
            //  - assistant: mount dialog only when this message is the global last AND pending
            //  - tool:      mount dialog on any pending occurrence
            const isLast = (m.id || m.Id) === globalLastMsgId;
            if (isControlElicitation) {
                const st = String(status).toLowerCase();
                if (roleLower === 'assistant') {
                    if (!(isLast && st === 'pending')) {
                        continue; // assistants’ non-last or resolved → execution details only
                    }
                } else if (roleLower === 'tool') {
                    if (st !== 'pending') {
                        continue; // resolved tool elicitations → details only
                    }
                } else {
                    // Other roles: do not mount as dialog
                    continue;
                }
            }
            // Assistant elicitation carried in a text message (non-control):
            // show only when last+pending; otherwise suppress bubble/modal entirely (execution-only).
            if (!isControlElicitation && roleLower === 'assistant' && !!elic) {
                const st = String(status).toLowerCase();
                if (!(isLast && st === 'pending')) {
                    continue;
                }
            }

            // Row usage derived from model call only when attached to this row later; leave null here.
            const row = {
                id,
                conversationId: m.conversationId || m.ConversationId,
                // For any elicitation row we allow (assistant last+pending or tool pending), force synthetic role.
                role: (isControlElicitation || (roleLower === 'assistant' && !!elic)) ? 'elicition' : roleLower,
                // Do not show any bubble content for elicitation rows; dialog carries the UI.
                content: (isControlElicitation || (roleLower === 'assistant' && !!elic)) ? '' : (m.content || m.Content || ''),
                createdAt,
                toolName: m.toolName || m.ToolName,
                turnId: turnIdRef,
                parentId: turnIdRef,
                executions: [],
                usage: null,
                // Normalize to "open" for elicitation so the dialog renderer reliably mounts
                status: (isControlElicitation || (roleLower === 'assistant' && !!elic)) ? 'open' : status,
                elicitation: elic,
                callbackURL,
            };
            try { console.debug('[ChatRow]', row); } catch(_) {}
            turnRows.push(row);
        }

        // 3) Attach steps to a single carrier message in the same turn (prefer first user)
        if (steps.length && turnRows.length) {
            let carrierIdx = turnRows.findIndex(r => r.role === 'user');
            if (carrierIdx < 0) carrierIdx = 0;
            // Compute usage from the thinking step model if available
            let usage = null;
            // No token fields on step; usage computed elsewhere in poll path; keep null here.
            // Also attach ToolExecution so ExecutionBubble can render ToolFeed inline without DS/meta changes.
            const toolExec = Array.isArray(turn?.ToolExecution) ? turn.ToolExecution
                : Array.isArray(turn?.toolExecution) ? turn.toolExecution
                : Array.isArray(turn?.ToolFeed) ? turn.ToolFeed
                : Array.isArray(turn?.toolFeed) ? turn.toolFeed : [];
            try { console.debug('[chat][turn][toolExec]', { turnId, count: Array.isArray(toolExec) ? toolExec.length : 0 }); } catch(_) {}
            turnRows[carrierIdx] = {
                ...turnRows[carrierIdx],
                executions: [{steps}],
                toolExecutions: toolExec,
                usage,
                turnStatus,
                turnCreatedAt,
                turnUpdatedAt,
                turnElapsedSec,
                isLastTurn,
            };
        }

        for (const r of turnRows) rows.push(r);

        // Note: No separate ToolFeed row push; ExecutionBubble will render ToolFeed inline when toolExecutions attached.

        // 4) If the turn has failed, add a dedicated error bubble so the user sees it immediately
        if ((turnStatus === 'failed' || (turnError && turnError.trim() !== '')) && turnId) {
            rows.push({
                id: `${turnId}/error`,
                conversationId: turn?.conversationId || turn?.ConversationId,
                role: 'assistant',
                content: `Error: ${turnError || 'turn failed'}`,
                createdAt: toISOSafe(turn?.createdAt || turn?.CreatedAt),
                turnId: turnId,
                parentId: turnId,
                status: 'failed',
                executions: [],
                usage: null,
            });
        }
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
                log.debug('[chat][signals] messages', {len, loading, ts: Date.now()});
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
            log.debug('[chat][connector][GET] messages', opts);
            const res = await origGet(opts);
            log.debug('[chat][connector][GET][done] messages', {status: res?.status, keys: Object.keys(res || {})});
            return res;
        };
    }
    if (origPost) {
        conn.post = async (opts) => {
            log.debug('[chat][connector][POST] messages', opts);
            const res = await origPost(opts);
            log.debug('[chat][connector][POST][done] messages', {
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
    log.debug('selectFolder 1', context)

    context.handlers.dialog.commit()
    log.debug('selectFolder', props)
}

// --------------------------- Debug helpers ------------------------------

export function debugHistoryOpen({context}) {
    try {
        const convCtx = context.Context('history') || context.Context('conversations');
        const selected = convCtx?.handlers?.dataSource?.peekSelection?.()?.selected || {};
        const id = selected?.id;
        log.debug('[chat][history] open click at', Date.now(), 'convId:', id, 'row:', selected);
    } catch (e) {
        log.debug('[chat][history] open click log error', e);
    }
}

export function debugHistorySelection({context}) {
    try {
        const convCtx = context.Context('history') || context.Context('conversations');
        const sel = convCtx?.handlers?.dataSource?.peekSelection?.();
        log.debug('[chat][history] selection', Date.now(), sel);
    } catch (e) {
        log.debug('[chat][history] selection log error', e);
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
        log.debug('[chat][messagesDS] onSuccess at', Date.now(), 'turns:', turns, 'messages:', messages);
    } catch (e) {
        log.debug('[chat][messagesDS] onSuccess log error', e);
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
        log.debug('[chat][messagesDS] onError at', Date.now(), error);
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
                log.debug('[chat] hydrateMessagesCollection: restored snapshot', {
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
        // Purge legacy toolfeed rows now that ToolFeed renders inline under ExecutionBubble
        try {
            prev = (prev || []).filter(r => !(r && (r.toolFeed === true || String(r?.id || '').endsWith('/toolfeed'))));
        } catch(_) {}
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
    const {agent, model, tool, showExecutionDetails, showToolFeed} = source

    console.log('saveSettings --- ', source)
    const convContext = context.Context('conversations');
    const convDataSource = convContext?.handlers?.dataSource;
    const current = convDataSource.peekFormData()
    convDataSource.setFormData?.({values: {...current, agent, model, tools:tool}});
    try { setExecutionDetailsEnabled(!!showExecutionDetails); } catch(_) {}
    try { setToolFeedEnabled(!!showToolFeed); } catch(_) {}
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

// Initialize settings dialog fields on open
export function prepareSettings(args) {
    const { context } = args || {};
    try {
        const metaCtx = context?.Context?.('meta');
        const execEnabled = getExecutionDetailsEnabled();
        const feedEnabled = getToolFeedEnabled();
        metaCtx?.handlers?.dataSource?.setFormField?.({ item: { id: 'showExecutionDetails' }, value: !!execEnabled });
        metaCtx?.handlers?.dataSource?.setFormField?.({ item: { id: 'showToolFeed' }, value: !!feedEnabled });
    } catch(_) {}
}

// Toggle Execution details visibility
export function toggleExecDetails(args) {
    try {
        const { context, selected, value } = args || {};
        let enabled;
        if (typeof selected === 'boolean') enabled = selected;
        else if (typeof value === 'boolean') enabled = value;
        else enabled = !!context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.()?.showExecutionDetails;
        setExecutionDetailsEnabled(!!enabled);
    } catch(_) {}
}

// Toggle Tool feed visibility
export function toggleToolFeed(args) {
    try {
        const { context, selected, value } = args || {};
        let enabled;
        if (typeof selected === 'boolean') enabled = selected;
        else if (typeof value === 'boolean') enabled = value;
        else enabled = !!context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.()?.showToolFeed;
        setToolFeedEnabled(!!enabled);
    } catch(_) {}
}

// Open settings dialog via composer settings icon
export async function onSettings(args) {
    const { context } = args || {};
    try { prepareSettings({ context }); } catch(_) {}
    try {
        await context?.handlers?.window?.openDialog?.({ execution: { args: ['settings'] } });
    } catch(_) {
        // Fallback to dialog.open event route
        try { await context?.handlers?.dialog?.open?.({ id: 'settings' }); } catch(_) {}
    }
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
    log.debug('[chat] submitMessage props', props);

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
        // Collect Forge-uploaded attachments from message (support multiple shapes) and form level
        try {
            log.debug('[chat] draft message attachments', message?.attachments);
            log.debug('[chat] draft message files', message?.files);
        } catch(_) {}
        const msgAtts = Array.isArray(message?.attachments) ? message.attachments : [];
        const msgFiles = Array.isArray(message?.files) ? message.files : [];
        // Also check DS form data as Forge Chat may store files under uploadField (we set uploadField: 'files')
        let formFiles = [];
        try {
            const formData = messagesContext?.handlers?.dataSource?.peekFormData?.()?.values
                || messagesContext?.handlers?.dataSource?.peekFormData?.();
            log.debug('[chat] peekFormData values', formData);
            if (Array.isArray(formData?.files)) formFiles = formData.files;
            else if (Array.isArray(formData?.upload)) formFiles = formData.upload;
        } catch (_) {}
        log.debug('[chat] pendingUploads (pre-merge)', pendingUploads);
        const allAtts = [...pendingUploads, ...msgAtts, ...msgFiles, ...formFiles];
        log.debug('[chat] collected attachments (raw)', allAtts);
        if (allAtts.length > 0) {
            body.attachments = allAtts.map(a => {
                const src = a?.data || a; // sometimes nested under data
                const uri = src?.uri || src?.url || src?.path || src?.href;
                const folder = src?.stagingFolder || src?.folder || src?.staging || src?.dir;
                const mime = src?.mime || src?.type || src?.contentType;
                const name = src?.name || (typeof uri === 'string' ? uri.split('/').pop() : undefined);
                const size = src?.size || src?.length || src?.bytes;
                return { name, size, stagingFolder: folder, uri, mime };
            }).filter(x => x && x.uri);
            // reset the stash after consuming
            pendingUploads = [];
            log.debug('[chat] body.attachments', body.attachments);
        }

        if (parameters && parameters.model) {
            body.model = parameters.model;
        }
        if (parameters && parameters.agent) {
            body.agent = parameters.agent;
        }

        // Mark conversation running so UI can show Abort based on data-driven selector
        try {
            const convCtx = context.Context('conversations');
            convCtx?.handlers?.dataSource?.setFormField?.({ item: { id: 'running' }, value: true });
            console.debug('[chat][submit] conversations.running=true');
        } catch(_) {}

        // Post user message
        const postResp = await messagesAPI.post({
            inputParameters: {convID},
            body: body
        });
        log.debug('[chat] post message response', postResp);

        const messageId = postResp?.data?.id;
        if (!messageId) {
            log.error('Message accepted but no id returned', postResp);
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
        log.error('submitMessage error', error);
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
 * Receives upload results from Forge chat and attaches them to the pending message.
 * @param {Object} props
 * @param {Object} props.message - mutable draft message object (Forge provides)
 * @param {Array|Object} props.result - upload result(s)
 */
export async function onUpload(props) {
    try {
        const { message } = props || {};
        const exec = props?.execution || {};
        const result = props?.result;
        // Forge variants: execution.result | execution.output | execution.data | props.result | props.files | props.data
        let list = exec.result || exec.output || exec.data || result || props?.files || props?.data;
        if (list && !Array.isArray(list)) {
            // Some backends return single object or wrap under {data: {...}}
            list = list.files || list.data || [list];
        }
        if (!list || list.length === 0) return;
        const normalized = list.map(a => {
            const src = a?.data || a;
            const uri = src?.uri || src?.url || src?.path || src?.href;
            const folder = src?.stagingFolder || src?.folder || src?.staging || src?.dir;
            const mime = src?.mime || src?.type || src?.contentType;
            const name = src?.name || (typeof uri === 'string' ? uri.split('/').pop() : undefined);
            const size = src?.size || src?.length || src?.bytes;
            return { name, size, stagingFolder: folder, uri, mime };
        }).filter(x => x && x.uri);
        if (!normalized.length) return;
        // Update composer message when provided
        if (message) {
            if (!Array.isArray(message.attachments)) message.attachments = [];
            message.attachments.push(...normalized);
        }
        // Also stash globally to ensure submit picks them up
        pendingUploads.push(...normalized);
    } catch (e) {
        // eslint-disable-next-line no-console
        log.warn('chat.onUpload: failed to attach uploads', e);
    }
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
        log.warn('chatService.abortConversation: invalid context');
        return false;
    }

    try {
        const convCtx = context.Context('conversations');
        const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id ||
            convCtx?.handlers?.dataSource?.getSelection?.()?.selected?.id;

        if (!convID) {
            log.warn('chatService.abortConversation – no active conversation');
            return false;
        }

        // Build absolute URL using configured agentlyAPI endpoint
        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/terminate`;
        const resp = await fetch(url, { method: 'POST' });
        let payload = null;
        try {
            // 204 → no body; guard parsing
            const text = await resp.text();
            if (text) payload = JSON.parse(text);
        } catch (_) {}
        const statusStr = resp.ok ? 'ok' : String(resp.status || 'error');
        // Update running only when termination succeeded (cancelled=true)
        const cancelled = !!(payload && payload.data && payload.data.cancelled);
        if (cancelled) {
            setStage({phase: 'terminated'});
            try {
                const convCtx2 = context.Context('conversations');
                convCtx2?.handlers?.dataSource?.setFormField?.({ item: { id: 'running' }, value: false });
            } catch(_) {}
        }
        return true;
    } catch (err) {
        log.error('chatService.abortConversation error', err);
        // Show error in UI if possible.
        const convCtx = context.Context('conversations');
        convCtx?.handlers?.setError?.(err);
        return false;
    }
}

/**
 * Compacts the current conversation by calling backend compaction endpoint.
 * Adds a summary message and flags prior messages as compacted (server-side).
 */
export async function compactConversation(props) {
    const { context } = props || {};
    if (!context || typeof context.Context !== 'function') {
        log.warn('chatService.compactConversation: invalid context');
        return false;
    }
    try {
        const convCtx = context.Context('conversations');
        const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id ||
            convCtx?.handlers?.dataSource?.getSelection?.()?.selected?.id;
        if (!convID) {
            log.warn('chatService.compactConversation – no active conversation');
            return false;
        }
        // Set stage to compacting
        try { setStage({ phase: 'compacting' }); } catch(_) {}
        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/compact`;
        const resp = await fetch(url, { method: 'POST' });
        if (!resp.ok) {
            const text = await resp.text().catch(() => '');
            throw new Error(text || `HTTP ${resp.status}`);
        }
        // Refresh messages to pick up summary/flags
        try {
            const msgCtx = context.Context('messages');
            const inSig = msgCtx?.signals?.input;
            if (inSig) {
                const cur = (typeof inSig.peek === 'function') ? (inSig.peek() || {}) : (inSig.value || {});
                const params = { ...(cur.parameters || {}), convID, since: '' };
                const next = { ...cur, parameters: params, fetch: true };
                if (typeof inSig.set === 'function') inSig.set(next); else inSig.value = next;
            } else {
                await msgCtx?.handlers?.dataSource?.getCollection?.();
            }
        } catch (_) {}
        try { setStage({ phase: 'done' }); } catch(_) {}
        return true;
    } catch (e) {
        log.error('compactConversation error', e);
        try { setStage({ phase: 'error' }); } catch(_) {}
        // surface as DS error so UI shows banner
        try { context?.Context('messages')?.handlers?.dataSource?.setError?.(e); } catch(_) {}
        return false;
    }
}

// Toolbar readonly predicate: return true to disable Compact when fewer than 2 messages
export function compactReadonly(args) {
    try {
        const { context } = args || {};
        const msgCtx = context?.Context?.('messages');
        const coll = (typeof msgCtx?.signals?.collection?.peek === 'function')
            ? (msgCtx.signals.collection.peek() || [])
            : (msgCtx?.handlers?.dataSource?.peekCollection?.() || []);
        const count = Array.isArray(coll) ? coll.length : 0;
        return count < 2;
    } catch (_) {
        return true;
    }
}

/**
 * Deletes the selected conversation from the history list and refreshes the datasource.
 */
export async function deleteConversation({context}) {
    try {
        const historyCtx = context?.Context('history') || context?.Context('conversations');
        const ds = historyCtx?.handlers?.dataSource;
        const sel = ds?.peekSelection?.();
        const id = sel?.selected?.id || ds?.peekFormData?.()?.id;
        if (!id) {
            log.warn('chatService.deleteConversation – no conversation selected');
            return false;
        }
        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const url = `${base}/v1/api/conversations/${encodeURIComponent(id)}`;
        const resp = await fetch(url, { method: 'DELETE' });
        if (!resp.ok && resp.status !== 204) {
            const text = await resp.text();
            throw new Error(text || `delete failed: ${resp.status}`);
        }
        // Refresh the history browser table only using DS handlers
        try {
            ds?.resetSelection?.();
            ds?.fetchCollection?.();
        } catch(_) {}
        // If this conversation is open in messages, clear it locally
        try {
            const convCtx = context.Context('conversations');
            const convHandlers = convCtx?.handlers?.dataSource;
            const form = convHandlers?.peekFormData?.() || {};
            if (form.id === id) {
                convHandlers?.setFormData?.({values: {id: ''}});
                const messagesCtx = context.Context('messages');
                const msgHandlers = messagesCtx?.handlers?.dataSource;
                msgHandlers?.setCollection?.([]);
                msgHandlers?.resetSelection?.();
                // Trigger a DS-driven refresh to propagate cleared state
                try { msgHandlers?.fetchCollection?.(); } catch(_) {}
            }
        } catch (_) {}
        return true;
    } catch (err) {
        log.error('chatService.deleteConversation error', err);
        return false;
    }
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
    const convId = data?.[0]?.conversationId;
    log.debug('[chat] receiveMessages', {count: data.length, sinceId, convId});
    // Merge incoming messages (append/update)
    mergeMessages(messagesContext, data);
    // Purge any user reply bubbles that correspond to elicitation userData
    try {
        const hideIds = new Set();
        for (const r of (data || [])) {
            const exes = Array.isArray(r?.executions) ? r.executions : [];
            for (const ex of exes) {
                const steps = Array.isArray(ex?.steps) ? ex.steps : [];
                for (const s of steps) {
                    if (s && s.reason === 'elicitation' && s.userData && s.replyMessageId) {
                        hideIds.add(s.replyMessageId);
                    }
                }
            }
        }
        if (hideIds.size > 0) {
            const collSig = messagesContext?.signals?.collection;
            if (collSig && Array.isArray(collSig.value)) {
                const before = collSig.value.length;
                collSig.value = collSig.value.filter(row => !hideIds.has(row?.id));
                const after = collSig.value.length;
                try { log.debug('[chat] receiveMessages: purged elicitation reply bubbles', {removed: before - after}); } catch(_) {}
            }
        }
    } catch(_) {}
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
    abortConversation,
    compactConversation,
    compactReadonly,
    deleteConversation,
    onInit,
    onDestroy,
    onMetaLoaded,
    onFetchMeta,
    onSettings,

    saveSettings,
    prepareSettings,
    toggleExecDetails,
    toggleToolFeed,
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
        elicition: ElicitionForm,
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
    const values = {
        ...current,
        agent: defaults.agent || '',
        model: defaults.model || '',
        // Pre-select tools allowed for the default agent so the Settings
        // dialog shows an accurate initial state when opened before any
        // user interaction.
        //
        // The mapping comes from meta.agentInfo where each agent entry may
        // declare a list of tools it can execute. When a new chat window
        // is opened we want those tools pre-selected, however this field
        // was previously omitted which caused the Settings dialog to show
        // an empty tools list.

        tool: (data.agentInfo && data.agentInfo[defaults.agent]
            && Array.isArray(data.agentInfo[defaults.agent].tools))
            ? data.agentInfo[defaults.agent].tools
            : [],
        agentInfo: data.agentInfo || {},
    }
    console.log('setting values:', values)
    ds.setFormData?.({values});

}

// Prevent DS from trying to assign object payload to a collection; return [] so
// the collection path is left untouched and onSuccess can map to the form.
function onFetchMeta(args) {
    const {collection = []} = args;
    const updated = collection.map(data => {
       const agentInfo = data.agentInfo || {};
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
            // Default selection of tools for the default agent. Use plural
            // property name to stay consistent with the rest of the code
            // base (saveSettings, conversationService, etc.).
            tool: agentInfo[data.defaults.agent]?.tools || []
        };
    });
    return updated;
}

//
