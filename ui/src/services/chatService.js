// Chat service helper used by forge SettingProvider.
// Contains submitMessage implementation extracted from App.jsx to keep
// App clean and focused on composition.

import {endpoints} from '../endpoint';
import {getLogger, ForgeLog} from 'forge/utils/logger';

const log = getLogger('agently');
import {FormRenderer} from 'forge';
import ElicitionForm from '../components/ElicitionForm.jsx';
import MCPInteraction from '../components/MCPInteraction.jsx';
import PolicyApproval from '../components/PolicyApproval.jsx';
import {poll} from './utils/apiUtils';
import {classifyMessage, normalizeMessages, isSimpleTextSchema} from './messageNormalizer';

import ExecutionBubble from '../components/chat/ExecutionBubble.jsx';
import ToolFeedBubble from '../components/chat/ToolFeedBubble.jsx';
import ToolFeed from '../components/chat/ToolFeed.jsx';
import HTMLTableBubble from '../components/chat/HTMLTableBubble.jsx';
import {ensureConversation, newConversation} from './conversationService';
import SummaryNote from '../components/chat/SummaryNote.jsx';
import {setStage} from '../utils/stageBus.js';
import {setComposerBusy} from '../utils/composerBus.js';
import {isElicitationSuppressed, markElicitationShown} from '../utils/elicitationBus.js';
import {
    setExecutionDetailsEnabled,
    setToolFeedEnabled,
    getExecutionDetailsEnabled,
    getToolFeedEnabled
} from '../utils/execFeedBus.js';

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
            }
            // Also ensure conversations form has running=false initially
            try {
                const convCtx = context.Context('conversations');
                convCtx?.handlers?.dataSource?.setFormField?.({item: {id: 'running'}, value: false});
            } catch (_) {
            }
        } catch (_) {
        }

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
                    }
                } catch (_) {
                }
                try {
                    dsTick({context});
                } catch (_) {
                }
                // try {
                //     installMessagesDebugHooks(context);
                // } catch (_) {
                // }
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

    // 5) Apply conversation defaults (agent/model) once metadata is available
    try {
        let tries = 0;
        const applyDefaultsOnce = () => {
            tries++;
            try {
                const metaCtx = context.Context('meta');
                const metaCol = metaCtx?.handlers?.dataSource?.peekCollection?.() || [];
                if (!Array.isArray(metaCol) || metaCol.length === 0) return false;
                const data = metaCol[0] || {};
                const defaults = data.defaults || {};
                const defAgent = String(defaults.agent || '');
                const defModel = String(defaults.model || '');
                const convCtx = context.Context('conversations');
                const convDS = convCtx?.handlers?.dataSource;
                if (!convDS) return true;
                const cur = convDS.peekFormData?.() || {};
                const next = {...cur};
                let changed = false;
                if (!next.agent && defAgent) {
                    next.agent = defAgent;
                    changed = true;
                }
                if (!next.model && defModel) {
                    next.model = defModel;
                    changed = true;
                }
                if (changed) {
                    convDS.setFormData?.({values: next});
                }
                return true;
            } catch (_) {
                return false;
            }
        };
        const t = setInterval(() => {
            if (applyDefaultsOnce() || tries > 25) {
                clearInterval(t);
            }
        }, 120);
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
        // Do not block polling while a turn is running; background updates rely on polling.
        // Any visual spinner suppression is handled separately via setLoading wrapper.
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
            // Expose usage tokens via a lightweight Usage DS form
            try {
                const usage = conv?.Usage || conv?.usage || {
                    promptTokens: conv?.UsageInputTokens ?? conv?.usageInputTokens,
                    completionTokens: conv?.UsageOutputTokens ?? conv?.usageOutputTokens,
                    totalTokens: conv?.Usage?.TotalTokens ?? conv?.usage?.totalTokens ?? conv?.Usage?.Total ?? conv?.usage?.total,
                    promptCachedTokens: conv?.Usage?.PromptCachedTokens ?? conv?.usage?.promptCachedTokens,
                    model: conv?.DefaultModel || conv?.defaultModel || conv?.Model || conv?.model,
                };
                if (usage) {
                    const usageCtx = context?.Context?.('usage');
                    // Normalize field casing for labels in the header
                    const normalizeModel = (val) => {
                        // Prefer a simple string. If array/object provided, derive a friendly label.
                        if (val == null) return '';
                        if (typeof val === 'string' || typeof val === 'number') return String(val);
                        if (Array.isArray(val)) {
                            const first = val[0];
                            if (!first) return '';
                            // Usage.Model[*] from backend has fields: Model, PromptTokens, etc.
                            return String(first.Model || first.model || first.Name || '');
                        }
                        if (typeof val === 'object') {
                            return String(val.Model || val.model || val.Name || '');
                        }
                        try { return JSON.stringify(val); } catch { return ''; }
                    };
                    const norm = {
                        promptTokens: usage.PromptTokens ?? usage.promptTokens ?? usage.Prompt ?? usage.prompt,
                        completionTokens: usage.CompletionTokens ?? usage.completionTokens ?? usage.Completion ?? usage.completion,
                        totalTokens: usage.TotalTokens ?? usage.totalTokens ?? usage.Total ?? usage.total,
                        promptCachedTokens: usage.PromptCachedTokens ?? usage.promptCachedTokens ?? usage.PromptCached ?? usage.cached,
                        model: normalizeModel(usage.Model ?? usage.model ?? (conv?.DefaultModel || conv?.Model || '')),
                    };
                    // Add optional prediction token fields if present
                    const acceptedPred = usage.CompletionAcceptedPredictionTokens ?? usage.completionAcceptedPredictionTokens ?? 0;
                    const rejectedPred = usage.CompletionRejectedPredictionTokens ?? usage.completionRejectedPredictionTokens ?? 0;
                    const predictionTokens = (Number(acceptedPred) || 0) + (Number(rejectedPred) || 0);
                    if (predictionTokens > 0) {
                        norm.predictionTokens = predictionTokens;
                    }
                    // Derive cost from Usage.Cost or sum of Usage.Model[*].Cost
                    let cost = undefined;
                    try {
                        if (usage.Cost != null) {
                            cost = Number(usage.Cost);
                        } else if (Array.isArray(usage.Model)) {
                            const costs = usage.Model
                                .map(m => (m && (m.Cost ?? m.cost)) != null ? Number(m.Cost ?? m.cost) : 0)
                                .filter(v => !Number.isNaN(v));
                            if (costs.length) {
                                cost = costs.reduce((a, b) => a + b, 0);
                            }
                        }
                    } catch(_) { /* ignore cost derivation errors */ }

                    const costText = (cost != null && !Number.isNaN(cost)) ? `$${Number(cost).toFixed(3)}` : '';
                    const formatThousandsWithSpaces = (n) => {
                        const v = Number(n);
                        if (!Number.isFinite(v)) return '';
                        const s = String(Math.trunc(v));
                        return s.replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
                    }
                    const totalTokensText = formatThousandsWithSpaces(norm.totalTokens);
                    const promptCachedTokensText = formatThousandsWithSpaces(norm.promptCachedTokens);
                    const values = { ...norm, cost, costText, totalTokensText, promptCachedTokensText };
                    usageCtx?.handlers?.dataSource?.setFormData?.({values});
                }
            } catch (_) { /* ignore */ }

            // Keep conversation header fields (e.g., title) in sync
            try {
                const convCtx2 = context?.Context?.('conversations');
                const ds2 = convCtx2?.handlers?.dataSource;
                if (ds2) {
                    const title = conv?.Title || conv?.title || '';
                    if (title) ds2.setFormField?.({item: {id: 'title'}, value: title});
                    const agent = conv?.AgentId || conv?.Agent || conv?.agent || '';
                    if (agent) ds2.setFormField?.({item: {id: 'agent'}, value: String(agent)});
                    const model = conv?.DefaultModel || conv?.Model || conv?.model || '';
                    if (model) ds2.setFormField?.({item: {id: 'model'}, value: String(model)});
                }
            } catch (_) { /* ignore */ }
            // Derive running/finished state from the last turn to keep Abort button accurate
            try {
                const lastTurn = Array.isArray(transcript) && transcript.length ? transcript[transcript.length - 1] : null;
                const turnStatus = String(lastTurn?.status || lastTurn?.Status || '').toLowerCase();
                const isRunning = (turnStatus === 'running' || turnStatus === 'open' || turnStatus === 'pending' || turnStatus === 'thinking' || turnStatus === 'processing');
                const isFinished = (!!turnStatus && !isRunning);
                const ctrlSig = messagesCtx?.signals?.control;
                if (ctrlSig) {
                    const prev = (typeof ctrlSig.peek === 'function') ? (ctrlSig.peek() || {}) : (ctrlSig.value || {});
                    const loading = !!isRunning;
                    if (prev.loading !== loading) {
                        ctrlSig.value = {...prev, loading};
                    }
                }
                const convCtx = context.Context('conversations');
                if (convCtx?.handlers?.dataSource?.setFormField) {
                    convCtx.handlers.dataSource.setFormField({item: {id: 'running'}, value: !!isRunning});
                }

            } catch (_) {
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
    } catch (_) {
    }
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
        } catch (_) {
        }

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
                } catch (_) {
                }
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
                                try {
                                    s3.userData = JSON.parse(udataRaw);
                                } catch (_) {
                                    s3.userData = udataRaw;
                                }
                            }
                        } catch (_) {
                        }
                        lastElicitationStep = s3;
                        recentElicitationStep = s3;
                    }
                }
            }
            // Attach chain/link context & actor from message level when present
            const linkedConvId = m.linkedConversationId || m.LinkedConversationId || null;
            const linkedConvObj = m.linkedConversation || m.LinkedConversation || null;
            const createdByUserId = m.createdByUserId || m.CreatedByUserId || null;
            const mode = m.mode || m.Mode || null;
            if (s1) steps.push({
                ...s1,
                linkedConversationId: linkedConvId,
                createdByUserId,
                mode,
                elapsed: computeElapsed(s1)
            });
            if (s2) steps.push({
                ...s2,
                linkedConversationId: linkedConvId,
                createdByUserId,
                mode,
                elapsed: computeElapsed(s2)
            });
            if (s3) steps.push({
                ...s3,
                linkedConversationId: linkedConvId,
                createdByUserId,
                mode,
                elapsed: computeElapsed(s3)
            });

            // When a message explicitly links another conversation, add a dedicated "link" step
            if (linkedConvId) {
                const lcCreated = linkedConvObj?.createdAt || linkedConvObj?.CreatedAt || m?.createdAt || m?.CreatedAt;
                const lcUpdated = linkedConvObj?.updatedAt || linkedConvObj?.UpdatedAt || lcCreated;
                const lcStatus = String(linkedConvObj?.status || linkedConvObj?.Status || '').toLowerCase() || 'pending';
                const sLink = {
                    id: (m.id || m.Id || '') + '/link',
                    name: 'link',
                    reason: 'link',
                    linkedConversationId: linkedConvId,
                    createdByUserId,
                    mode,
                    startedAt: lcCreated,
                    endedAt: lcUpdated,
                    statusText: lcStatus,
                };
                steps.push({...sLink, elapsed: computeElapsed(sLink)});
            }
        }

        // Sort steps by timestamp (prefer startedAt, fallback endedAt)
        steps.sort((a, b) => {
            const ta = a?.startedAt || a?.endedAt || '';
            const tb = b?.startedAt || b?.endedAt || '';
            const da = ta ? new Date(ta).getTime() : 0;
            const db = tb ? new Date(tb).getTime() : 0;
            return da - db;
        });

        // Keep error rendering in the table footer (ExecutionDetails) rather than as a step.

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
                        try {
                            target.replyMessageId = m.id || m.Id;
                        } catch (_) {
                        }
                        suppressBubble = true;
                    }
                    // Also suppress when user content equals any elicitation inline body in this turn
                    if (!suppressBubble && typeof txt === 'string' && elicitationUserBodies.has(txt.trim())) {
                        suppressBubble = true;
                    }
                } catch (_) {
                }
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
                    }
                } catch (_) {
                }
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
            // Suppress repeat mounts for the same elicitation id briefly to avoid flicker on fast polls
            const thisId = m.id || m.Id;
            if ((isControlElicitation || (roleLower === 'assistant' && !!elic)) && isElicitationSuppressed(thisId)) {
                continue;
            }

            // Row usage derived from model call only when attached to this row later; leave null here.
            const row = {
                id,
                conversationId: m.conversationId || m.ConversationId,
                // For any elicitation row we allow (assistant last+pending or tool pending), force synthetic role.
                role: (isControlElicitation || (roleLower === 'assistant' && !!elic)) ? 'elicition' : roleLower,
                name: (m.createdByUserId || m.CreatedByUserId || ''),
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
            turnRows.push(row);
            // Mark as shown so immediate re-fetch does not re-open the dialog right away
            if (row.role === 'elicition') {
                try {
                    markElicitationShown(row.id, 2500);
                } catch (_) {
                }
            }
        }

        // 3) Attach steps to a single carrier message in the same turn (prefer first user)
        if (steps.length && turnRows.length) {
            let carrierIdx = turnRows.findIndex(r => r.role === 'user');
            if (carrierIdx < 0) carrierIdx = 0;
            // Compute usage from the thinking step model if available
            let usage = null;
            // No token fields on step; usage computed elsewhere in poll path; keep null here.
            // Also attach ToolExecution for a dedicated ToolFeed bubble row
            const toolExec = Array.isArray(turn?.ToolExecution) ? turn.ToolExecution
                : Array.isArray(turn?.toolExecution) ? turn.toolExecution
                    : Array.isArray(turn?.ToolFeed) ? turn.ToolFeed
                        : Array.isArray(turn?.toolFeed) ? turn.toolFeed : [];
            turnRows[carrierIdx] = {
                ...turnRows[carrierIdx],
                usage,
                turnStatus,
                turnCreatedAt,
                turnUpdatedAt,
                turnElapsedSec,
                isLastTurn,
            };
            // Build separate rows but delay pushing until we reorder the turn display
            const execRow = {
                id: `${turnId}/execution`,
                conversationId: turn?.conversationId || turn?.ConversationId,
                role: 'execution',
                content: '',
                createdAt: toISOSafe(turn?.createdAt || turn?.CreatedAt),
                turnId: turnId,
                parentId: turnId,
                status: turnStatus,
                executions: [{steps}],
                turnStatus,
                turnError,
                turnCreatedAt,
                turnUpdatedAt,
                turnElapsedSec,
                isLastTurn,
            };
            const toolRow = (Array.isArray(toolExec) && toolExec.length > 0 && turnId) ? {
                id: `${turnId}/toolfeed`,
                conversationId: turn?.conversationId || turn?.ConversationId,
                role: 'tool',
                content: '',
                createdAt: toISOSafe(turn?.createdAt || turn?.CreatedAt),
                turnId: turnId,
                parentId: turnId,
                status: 'succeeded',
                toolExecutions: toolExec,
                toolFeed: true,
                isLastTurn,
            } : null;

            // Reorder within turn: user → execution → tool feed → others (assistant/elicition)
            const userRows = turnRows.filter(r => r && r.role === 'user');
            const otherRows = turnRows.filter(r => !userRows.includes(r));
            // Push user rows first (usually one)
            for (const r of userRows) rows.push(r);
            // Then execution details
            rows.push(execRow);
            // Then tool feed (if any)
            if (toolRow) rows.push(toolRow);
            // Finally the remaining rows (assistant/elicition/etc)
            for (const r of otherRows) rows.push(r);
            // Continue to error bubble handling below if applicable
            continue;
        }

        // No execution steps – just push turn rows as-is
        for (const r of turnRows) rows.push(r);

        // Separate ToolFeed row is pushed above.

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

// // ---------------------------------------------------------------------------
// // Debug helpers – instrument Forge DataSource signals & connector calls
// // ---------------------------------------------------------------------------
// function installMessagesDebugHooks(context) {
//     const messagesCtx = context?.Context?.('messages');
//     if (!messagesCtx || messagesCtx._debugInstalled) return;
//     messagesCtx._debugInstalled = true;
//
//     const collSig = messagesCtx?.signals?.collection;
//     const ctrlSig = messagesCtx?.signals?.control;
//     // Poll for changes in collection length/loading flag to detect external mutations
//     let lastLen = Array.isArray(collSig?.value) ? collSig.value.length : 0;
//     let lastLoading = !!ctrlSig?.peek?.()?.loading;
//     const tick = () => {
//         try {
//             const curr = Array.isArray(collSig?.value) ? collSig.value : [];
//             const len = curr.length;
//             const ctrlVal = ctrlSig?.peek?.() || {};
//             let loading = !!ctrlVal.loading;
//             const errVal = ctrlVal.error;
//             if (errVal && (typeof errVal === 'object')) {
//                 // Coerce Error object to string so Chat error banner can render safely
//                 const coerced = String(errVal.message || errVal.toString?.() || '');
//                 ctrlSig.value = {...ctrlVal, error: coerced};
//             }
//             // Once we have any messages, suppress spinner on background polls.
//             if (len > 0) {
//                 context.resources = context.resources || {};
//                 if (!context.resources.suppressMessagesLoading) {
//                     context.resources.suppressMessagesLoading = true;
//                 }
//             }
//             if (len !== lastLen || loading !== lastLoading) {
//                 log.debug('[chat][signals] messages', {len, loading, ts: Date.now()});
//                 lastLen = len;
//                 lastLoading = loading;
//             }
//         } catch (_) {
//         }
//     };
//     const t = setInterval(tick, 120);
//     context.resources = context.resources || {};
//     context.resources.messagesDebugTimer = t;
//
//     // Wrap window.openDialog to guard undefined dialog ids that cause ViewDialog warnings
//     try {
//         const win = context?.handlers?.window;
//         if (win && typeof win.openDialog === 'function' && !win._safeWrapped) {
//             const origOpen = win.openDialog.bind(win);
//             win.openDialog = async (arg) => {
//                 try {
//                     // Allow either string id or { id } or { execution }
//                     if (typeof arg === 'string') {
//                         if (!arg || !String(arg).trim()) {
//                             console.warn('[chat][openDialog] ignored empty id');
//                             return;
//                         }
//                     } else if (typeof arg === 'object' && arg) {
//                         const hasId = typeof arg.id === 'string' && arg.id.trim() !== '';
//                         const hasExec = arg.execution && Array.isArray(arg.execution.args) && arg.execution.args.length > 0;
//                         if (!hasId && !hasExec) {
//                             console.warn('[chat][openDialog] ignored: missing id/execution', arg);
//                             return;
//                         }
//                     } else {
//                         console.warn('[chat][openDialog] ignored: invalid args');
//                         return;
//                     }
//                     return await origOpen(arg);
//                 } catch (e) {
//                     console.error('[chat][openDialog] error', e);
//                     throw e;
//                 }
//             };
//             win._safeWrapped = true;
//         }
//     } catch (_) {
//     }
//
//     // Wrap connector GET/POST to log DF activity
//     const conn = messagesCtx.connector || {};
//     const origGet = conn.get?.bind(conn);
//     const origPost = conn.post?.bind(conn);
//     if (origGet) {
//         conn.get = async (opts) => {
//             log.debug('[chat][connector][GET] messages', opts);
//             const res = await origGet(opts);
//             log.debug('[chat][connector][GET][done] messages', {status: res?.status, keys: Object.keys(res || {})});
//             return res;
//         };
//     }
//     if (origPost) {
//         conn.post = async (opts) => {
//             log.debug('[chat][connector][POST] messages', opts);
//             const res = await origPost(opts);
//             log.debug('[chat][connector][POST][done] messages', {
//                 status: res?.status,
//                 dataKeys: Object.keys(res?.data || {})
//             });
//             return res;
//         };
//     }
//
//     // Suppress loading spinner flicker for background polling after initial load.
//     try {
//         const ds = messagesCtx?.handlers?.dataSource;
//         if (ds && typeof ds.setLoading === 'function' && !ds._setLoadingWrapped) {
//             const origSetLoading = ds.setLoading.bind(ds);
//             ds.setLoading = (flag) => {
//                 // After initial data arrives, avoid toggling loading=true; only allow clearing to false.
//                 const suppress = !!(context?.resources?.suppressMessagesLoading);
//                 if (suppress) {
//                     if (!flag) {
//                         return origSetLoading(false);
//                     }
//                     // ignore true to prevent spinner flicker
//                     return;
//                 }
//                 return origSetLoading(flag);
//             };
//             ds._setLoadingWrapped = true;
//         }
//     } catch (_) {
//     }
// }

function selectFolder(props) {
    const {context, selected} = props;
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
        // Keep dedicated ToolFeed rows; do not purge
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
    const source = context.Context('meta').handlers.dataSource.peekFormData()

    const {
        agent,
        model,
        tool,
        showExecutionDetails,
        showToolFeed,
        toolCallExposure,
        autoSummarize,
        chainsEnabled,
        allowedChains
    } = source


    console.debug('[settings.saveSettings] form snapshot', {
        source,
        agent,
        model,
        tool,
        toolCallExposure,
        autoSummarize,
        chainsEnabled,
        allowedChains
    });
    // Avoid updating conversations DS here (would invoke hooks). Preferences are persisted below.
    setExecutionDetailsEnabled(!!showExecutionDetails);
    setToolFeedEnabled(!!showToolFeed);
    const signals = context.Signals('conversations');
    const patch = signals.form.peek()
    signals.form.value = {...patch, ...source}
}


// Applies meta.agentTools mapping to the conversations.tools field when agent changes
export function selectAgent(args) {
    const {context} = args
    const ds = context.handlers.dataSource;
    const formBefore = ds.peekFormData();
    // Try to resolve selection from args or the DS (the DS may update after our handler due to event chaining)
    const sel = args && args.selected;


    let candidate = undefined;
    // 1) direct value/id
    if (sel && typeof sel === 'object') {
        candidate = sel.value ?? sel.id ?? sel.label;
    }
    // 2) top-level fields forwarded by EventAdapter
    if (!candidate && (args.value !== undefined)) candidate = args.value;
    if (!candidate && (args.event !== undefined)) candidate = args.event; // some packs put the string here


    const tryApply = (k) => {
        const key = String(k || '');
        const form = context.handlers.dataSource.peekFormData()

        if (!key) return;
        // Ensure the select control reflects the new agent value
        try {
            ds.setFormField({item: {id: 'agent'}, value: key});
        } catch (_) {
        }
        const selectedTools = form?.agentInfo?.[key]?.tools || [];
        const selectedModel = form?.agentInfo?.[key]?.model || '';
        const agentValues = {...(form?.agentInfo?.[key] || {}), tool: selectedTools}
        delete (agentValues['tools'])
        const prev = ds.peekFormData()
        ds.setFormData({values: {...prev, ...agentValues}})
    };

    if (candidate) {
        tryApply(candidate);
        return;
    }

}


// Applies meta.agentTools mapping to the conversations.tools field when agent changes
export function selectModel(args) {
    const {context, selected} = args
    context.handlers.dataSource.setFormField({item: {id: 'model'}, value: selected});
}


// Open settings dialog via composer settings icon
export async function onSettings(args) {
    const {context} = args || {};
    const win = context?.handlers?.window;
    const settingsCtx = context.Context('settings')
    const f = settingsCtx.handlers.dataSource.peekFormData()
    await win.openDialog({execution: {args: ['settings']}});
    return;
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


    const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};

    const {agent, model, tool, toolCallExposure, autoSummarize, disableChains, allowedChains=[]} = metaForm

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
            content: message.content,tools:tool,
            agent, model, toolCallExposure, autoSummarize, disableChains, allowedChains,
        }
        // Collect Forge-uploaded attachments from message (support multiple shapes) and form level
        try {
            log.debug('[chat] draft message attachments', message?.attachments);
            log.debug('[chat] draft message files', message?.files);
        } catch (_) {
        }
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
        } catch (_) {
        }
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
                return {name, size, stagingFolder: folder, uri, mime};
            }).filter(x => x && x.uri);
            // reset the stash after consuming
            pendingUploads = [];
            log.debug('[chat] body.attachments', body.attachments);
        }


        const convCtx = context.Context('conversations');
        convCtx?.handlers?.dataSource?.setFormField?.({item: {id: 'running'}, value: true});
        console.debug('[chat][submit] conversations.running=true');

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
        try {
            const convCtx = context.Context('conversations');
            convCtx?.handlers?.dataSource?.setFormField?.({item: {id: 'running'}, value: false});
        } catch (_) {
        }
        try {
            setStage({phase: 'error'});
        } catch (_) {
        }
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
        const {message} = props || {};
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
            return {name, size, stagingFolder: folder, uri, mime};
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
        const resp = await fetch(url, {method: 'POST', credentials: 'include'});
        let payload = null;
        try {
            // 204 → no body; guard parsing
            const text = await resp.text();
            if (text) payload = JSON.parse(text);
        } catch (_) {
        }
        const statusStr = resp.ok ? 'ok' : String(resp.status || 'error');
        // Update running only when termination succeeded (cancelled=true)
        const cancelled = !!(payload && payload.data && payload.data.cancelled);
        if (cancelled) {
            setStage({phase: 'terminated'});
            try {
                const convCtx2 = context.Context('conversations');
                convCtx2?.handlers?.dataSource?.setFormField?.({item: {id: 'running'}, value: false});
            } catch (_) {
            }
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
 * Adds a summary message and flags prior messages as archived (server-side).
 */
export async function compactConversation(props) {
    const {context} = props || {};
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
        try {
            setStage({phase: 'compacting'});
        } catch (_) {
        }
        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/compact`;
        const resp = await fetch(url, {method: 'POST', credentials: 'include'});
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
                const params = {...(cur.parameters || {}), convID, since: ''};
                const next = {...cur, parameters: params, fetch: true};
                if (typeof inSig.set === 'function') inSig.set(next); else inSig.value = next;
            } else {
                await msgCtx?.handlers?.dataSource?.getCollection?.();
            }
        } catch (_) {
        }
        try {
            setStage({phase: 'done'});
        } catch (_) {
        }
        return true;
    } catch (e) {
        log.error('compactConversation error', e);
        try {
            setStage({phase: 'error'});
        } catch (_) {
        }
        // surface as DS error so UI shows banner
        try {
            context?.Context('messages')?.handlers?.dataSource?.setError?.(e);
        } catch (_) {
        }
        return false;
    }
}

// Toolbar readonly predicate: return true to disable Compact when fewer than 2 messages
export function compactReadonly(args) {
    try {
        const {context} = args || {};
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
        const resp = await fetch(url, {method: 'DELETE', credentials: 'include'});
        if (!resp.ok && resp.status !== 204) {
            const text = await resp.text();
            throw new Error(text || `delete failed: ${resp.status}`);
        }
        // Refresh the history browser table only using DS handlers
        try {
            ds?.resetSelection?.();
            ds?.fetchCollection?.();
        } catch (_) {
        }
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
                try {
                    msgHandlers?.fetchCollection?.();
                } catch (_) {
                }
            }
        } catch (_) {
        }
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

            // Special placement for per-turn Tool Feed: insert right after the execution row
            if (addedBase.toolFeed && addedBase.turnId) {
                const turnId = addedBase.turnId;
                const execId = `${turnId}/execution`;
                const execIdx = current.findIndex((m) => m && m.id === execId);
                if (execIdx >= 0) {
                    current.splice(execIdx + 1, 0, addedBase);
                } else {
                    // Fallback: insert before first assistant row of the same turn if present
                    const beforeAssistIdx = current.findIndex((m) => m && (m.parentId === turnId || m.turnId === turnId) && (m.role === 'assistant' || m.role === 'elicition'));
                    if (beforeAssistIdx >= 0) current.splice(beforeAssistIdx, 0, addedBase);
                    else current.push(addedBase);
                }
            } else {
                current.push(addedBase);
            }
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
        const resolvedElicitationBaseIds = new Set();
        for (const r of (data || [])) {
            const exes = Array.isArray(r?.executions) ? r.executions : [];
            for (const ex of exes) {
                const steps = Array.isArray(ex?.steps) ? ex.steps : [];
                for (const s of steps) {
                    if (s && s.reason === 'elicitation' && s.userData && s.replyMessageId) {
                        hideIds.add(s.replyMessageId);
                    }
                    // Track resolved/accepted elicitation steps so stale dialog rows can be removed
                    if (s && s.reason === 'elicitation') {
                        const statusText = String(s.statusText || '').toLowerCase();
                        const accepted = !!s.successBool || statusText === 'accepted' || statusText === 'done' || statusText === 'succeeded';
                        if (accepted && typeof s.id === 'string' && s.id.endsWith('/elicitation')) {
                            resolvedElicitationBaseIds.add(s.id.slice(0, -('/elicitation'.length)));
                        }
                    }
                }
            }
        }
        if (hideIds.size > 0 || resolvedElicitationBaseIds.size > 0) {
            const collSig = messagesContext?.signals?.collection;
            if (collSig && Array.isArray(collSig.value)) {
                const before = collSig.value.length;
                collSig.value = collSig.value.filter(row => {
                    const id = row?.id;
                    if (!id) return true;
                    if (hideIds.has(id)) return false;
                    // Drop stale elicitation dialog rows when an accepted step for the same message id was observed
                    if ((row?.role === 'elicition') && resolvedElicitationBaseIds.has(id)) return false;
                    return true;
                });
                const after = collSig.value.length;
                try {
                    log.debug('[chat] receiveMessages: purged rows', {
                        removed: before - after,
                        removedTypes: {
                            replies: hideIds.size,
                            resolvedElicitations: resolvedElicitationBaseIds.size,
                        }
                    });
                } catch (_) {
                }
            }
        }
    } catch (_) {
    }
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


// Returns a Blueprint.js icon name for a task status.
// Supported statuses: pending, in_progress, completed
export function taskStatusIcon(props) {
    const statusRaw = props?.status ?? props?.row?.status ?? props?.row?.Status ?? '';
    try {
        const status = String(statusRaw).toLowerCase();
        if (status === 'completed' || status === 'succeeded' || status === 'done' || status === 'accepted') {
            return 'tick';
        }
        if (status === 'in_progress' || status === 'running' || status === 'processing') {
            return 'play';
        }
        if (status === 'pending' || status === 'open' || status === 'queued' || status === 'waiting') {
            return 'time';
        }
        // Fallback neutral indicator
        return 'dot';
    } catch (_) {
        return 'dot';
    }
}

export async function onChangedFileSelect(props) {
    try {
        console.log('onChangedFileSelect starting', props)
        const {context, item, node, diff} = props || {};
        const rec = item || node || {};

        const pick = (obj, names) => {
            for (const n of names) {
                const v = obj && obj[n];
                if (typeof v === 'string' && v.trim()) return v.trim();
            }
            return '';
        };
        const tryNested = (obj, names) => {
            const v = pick(obj, names);
            if (v) return v;
            for (const k of ['data', 'row', 'record', 'value', 'payload']) {
                const inner = obj && obj[k];
                if (inner && typeof inner === 'object') {
                    const vv = pick(inner, names);
                    if (vv) return vv;
                }
            }
            return '';
        };

        // 1) Prefer explicit top-level props first
        let uri = pick(props || {}, ['uri', 'url']);
        // For previous version, only accept explicit origUri/origUrl; do not derive.
        let origUri = pick(props || {}, ['origUri', 'origUrl']);
        // 2) Fall back to item/node payloads (direct fields only)
        if (!uri) uri = pick(rec || {}, ['uri', 'url', 'path', 'href', 'Uri', 'URL']);
        if (!origUri) origUri = pick(rec || {}, ['origUri', 'origUrl']);

        const title = rec?.name || rec?.file || (typeof uri === 'string' ? uri.split('/').pop() : 'Changed File');

        try {
            console.log('[changedFile][select]', {
                recKeys: Object.keys(rec || {}),
                top: {uri: props?.uri, url: props?.url, origUri: props?.origUri, origUrl: props?.origUrl},
                uri,
                origUri
            });
        } catch (_) {
        }

        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');

        async function fetchText(u, label) {
            if (!u) return '';
            const url = `${base}/v1/workspace/file-browser/download?uri=${encodeURIComponent(u)}`;
            try {
                console.log('[changedFile][fetch:start]', {label, url, raw: u});
            } catch (_) {
            }
            const t0 = Date.now();
            let resp;
            try {
                resp = await fetch(url, {credentials: 'include'});
            } catch (e) {
                try {
                    console.warn('[changedFile][fetch:error]', {label, url, error: String(e)});
                } catch (_) {
                }
                return '';
            }
            const dt = Date.now() - t0;
            try {
                console.log('[changedFile][fetch:done]', {label, status: resp?.status, ms: dt});
            } catch (_) {
            }
            if (!resp.ok) return '';
            return await resp.text();
        }

        // Open dialog in loading state immediately
        try {
            const mod = await import('../utils/dialogBus.js');
            mod.openCodeDiffDialog({
                title,
                hasPrev: !!origUri,
                loading: true,
                currentUri: uri || '',
                prevUri: origUri || ''
            });
        } catch (_) {
        }

        const fetches = [fetchText(uri, 'current')];
        const includePrev = !!origUri;
        if (includePrev) fetches.push(fetchText(origUri, 'prev'));
        const results = await Promise.allSettled(fetches);
        const currentText = results[0]?.status === 'fulfilled' ? results[0].value : '';
        const prevText = includePrev && results[1]?.status === 'fulfilled' ? results[1].value : '';
        const diffText = typeof diff === 'string' ? diff : (rec?.diff || '');

        // Update dialog content
        try {
            const mod = await import('../utils/dialogBus.js');
            mod.updateCodeDiffDialog({
                current: currentText,
                prev: prevText,
                diff: diffText,
                hasPrev: !!prevText,
                loading: false
            });
        } catch (_) {
        }
    } catch (e) {
        try {
            console.error('[changedFile][handler:error]', e);
        } catch (_) {
        }
    }
    return true
}

export function prepareChangeFiles(props) {
    const {collection, context} = props
    const {dataSource} = context
    const {dataSourceRef} = dataSource
    const patentCtx = context.Context(dataSourceRef)
    const form = patentCtx.handlers.dataSource.peekFormData()
    const {workdir} = form
    return prepareFileTree({workdir, collection})
}


export function prepareFileTree({workdir, collection = []}) {
    const norm = (p) => String(p || '').replace(/\\/g, '/');
    const wd = norm(workdir).replace(/\/+$/, '');

    const baseTail = (() => {
        // help fallback matching if path doesn't start with workdir
        const parts = wd.split('/').filter(Boolean);
        // last 2 segments (e.g., "viant/tagly") work well for repos
        return parts.slice(-2).join('/');
    })();

    const relativize = (p) => {
        let s = norm(p);
        if (!s) return '';
        if (wd && (s === wd || s.startsWith(wd + '/'))) {
            return s.slice(wd.length).replace(/^\/+/, '');
        }
        const idx = s.indexOf('/' + baseTail + '/');
        if (idx !== -1) {
            return s.slice(idx + baseTail.length + 2); // +2 for two slashes around baseTail
        }
        // last fallback: strip leading slash
        return s.replace(/^\/+/, '');
    };

    const partsOf = (rel) => rel.split('/').filter(Boolean);

    // index by relative path
    const byPath = new Map();

    const ensureFolder = (pathParts) => {
        let acc = '';
        for (let i = 0; i < pathParts.length; i++) {
            const seg = pathParts[i];
            acc = acc ? `${acc}/${seg}` : seg;
            if (!byPath.has(acc)) {
                byPath.set(acc, {
                    uri: `/${acc}`,
                    name: seg,
                    isFolder: true,
                    isExpanded: true,
                    icon: 'folder-open',
                    childNodes: [],
                    parentPath: acc.includes('/') ? acc.slice(0, acc.lastIndexOf('/')) : '',
                });
            }
        }
    };

    const ensureFile = (folderParts, fileName, meta) => {
        const folderPath = folderParts.join('/');
        const full = folderPath ? `${folderPath}/${fileName}` : fileName;
        if (!byPath.has(full)) {
            byPath.set(full, {
                uri: `/${full}`,
                name: fileName,
                isFolder: false,
                childNodes: [],
                parentPath: folderPath || '',
                ...meta,
            });
        }
    };

    // Ingest collection
    for (const item of collection) {
        const full = relativize(item.url || item.uri);
        if (!full) continue;

        const meta = {
            kind: item.kind,
            diff: item.diff,
            uri: norm(item.url),
            url: item.url,
            origUrl: norm(item.origUrl),
        };

        const parts = partsOf(full);
        if (parts.length === 0) continue;

        const folderParts = parts.slice(0, -1);
        const fileName = parts[parts.length - 1];

        if (folderParts.length) ensureFolder(folderParts);
        ensureFile(folderParts, fileName, meta);
    }

    // Link parents
    for (const node of byPath.values()) {
        if (node.parentPath && byPath.has(node.parentPath)) {
            byPath.get(node.parentPath).childNodes.push(node);
        }
    }

    // Sort children: folders first, then files, by name
    const sortChildren = (arr) => {
        arr.sort((a, b) => {
            if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
            return a.name.localeCompare(b.name);
        });
    };

    for (const node of byPath.values()) {
        if (node.childNodes?.length) sortChildren(node.childNodes);
    }

    // Collect roots
    const roots = [];
    for (const node of byPath.values()) {
        if (!node.parentPath) roots.push(node);
    }
    sortChildren(roots);

    return roots;
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
    onChangedFileSelect,
    onInit,
    onDestroy,
    onMetaLoaded,
    onFetchMeta,
    onSettings,
    taskStatusIcon,
    saveSettings,
    debugHistoryOpen,
    debugHistorySelection,
    debugMessagesLoaded,
    debugMessagesError,
    prepareChangeFiles,
    runPatchCommit: async (props) => await (async function (p) {
        const {context} = p || {};
        try {
            const convCtx = context?.Context?.('conversations');
            const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id || convCtx?.handlers?.dataSource?.getSelection?.()?.selected?.id;
            if (!convID) return false;
            const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
            const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/tools/run`;
            const resp = await fetch(url, {
                method: 'POST',
                credentials: 'include',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({service: 'system/patch', method: 'commit', args: {}})
            });
            return resp.ok;
        } catch (_) {
            return false;
        }
    })(props),
    runPatchRollback: async (props) => await (async function (p) {
        const {context} = p || {};
        try {
            const convCtx = context?.Context?.('conversations');
            const convID = convCtx?.handlers?.dataSource?.peekFormData?.()?.id || convCtx?.handlers?.dataSource?.getSelection?.()?.selected?.id;
            if (!convID) return false;
            const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
            const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/tools/run`;
            const resp = await fetch(url, {
                method: 'POST',
                credentials: 'include',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({service: 'system/patch', method: 'rollback', args: {}})
            });
            return resp.ok;
        } catch (_) {
            return false;
        }
    })(props),
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
        bubble: HTMLTableBubble,
        execution: ExecutionBubble,
        toolfeed: ToolFeedBubble,
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
    const defAgent = String(defaults.agent || '');
    const defModel = defaults.model || '';
    const values = {
        ...current,
        agent: defAgent,
        model: defModel,
        agentInfo: data.agentInfo || {},
    }
    ds.setFormData?.({values});
}


// Prevent DS from trying to assign object payload to a collection; return [] so
// the collection path is left untouched and onSuccess can map to the form.
function onFetchMeta(args) {
    const {collection = [], context} = args;
    const metaCtx = context?.Context?.('meta');
    const currentForm = metaCtx?.handlers?.dataSource?.peekFormData?.() || {};

    const updated = collection.map(data => {
        const agentInfo = data.agentInfo || {};
        const agentsRaw = Array.isArray(data?.agents)
            ? data.agents
            : (data?.agentInfo ? Object.keys(data.agentInfo) : []);

        const modelsRaw = Array.isArray(data?.models)
            ? data.models
            : (data?.defaults?.model ? [data.defaults.model] : []);

        const toolsRaw = Array.isArray(data?.tools) ? data.tools : [];

        // TreeMultiSelect expects a flat option list and will build the tree
        // by splitting the value with properties.separator. Keep options flat
        // and let the widget handle grouping to avoid runtime errors.

        const agentChainTargets = {};
        Object.entries(agentInfo).forEach(([k, v]) => {
            agentChainTargets[k] = Array.isArray(v?.chains) ? v.chains : [];
        });
        // Preserve current agent selection if present; otherwise use defaults
        const curAgent = String(currentForm.agent || data.defaults.agent || '');


        const settings = {...data.agentInfo[curAgent], tool: ''}
        settings.tool = settings.tools
        delete (settings['tools'])

        return {
            ...data,
            agentOptions: agentsRaw.map(v => {
                const id = String(v);
                const label = (agentInfo?.[id]?.name) ? String(agentInfo[id].name) : id;
                return { id, value: id, label };
            }),
            agent: curAgent,

            modelOptions: modelsRaw.map(v => ({
                id: String(v),
                value: String(v),
                label: String(v)
            })),
            model: data.defaults.model,

            // Provide a grouping key that replaces '/' with '-' for hierarchical display,
            // while preserving the original value used by the backend.
            toolOptions: toolsRaw.map((v) => {
                const raw = String(v);
                const groupKey = raw.replaceAll('/', '-');
                return { id: raw, value: raw, label: raw, groupKey };
            }),
            agentChainTargets,
            ...settings,

        };
    });
    return updated;
}

//
