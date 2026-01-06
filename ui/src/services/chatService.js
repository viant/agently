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
import {ensureConversation, newConversation, getActiveConversationID} from './conversationService';
import SummaryNote from '../components/chat/SummaryNote.jsx';
import {setStage} from '../utils/stageBus.js';
import {setComposerBusy} from '../utils/composerBus.js';
import {isElicitationSuppressed, markElicitationShown} from '../utils/elicitationBus.js';
import {detectVoiceControl} from '../utils/voiceControl.js';
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
const toISOSafe = (value) => {
    if (!value) {
        return new Date().toISOString();
    }

    const date = new Date(value);
    if (isNaN(date.getTime())) {
        return new Date().toISOString();
    }
    return date.toISOString();
};

// -------------------------------
// Explorer feed handlers
// -------------------------------

export function explorerOpenIcon() {
    return 'document-open';
}

export async function explorerOpen(props) {
    const row = props?.row || props?.item || props?.node || {};
    const uri = row?.uri || row?.URI || row?.Path || row?.path || '';
    if (!uri) return false;
    return await explorerRead({ ...props, uri });
}

export async function explorerRead(props) {
    const context = props?.context;
    const uri = String(props?.uri || '').trim();
    if (!context || !uri) return false;

    const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
    const title = uri.split('/').pop() || uri;

    try {
        const mod = await import('../utils/dialogBus.js');
        mod.openFileViewDialog({ title, uri, loading: true, content: '' });
    } catch (_) {
    }

    try {
        // Reuse existing file-browser download endpoint.
        const url = `${base}/v1/workspace/file-browser/download?uri=${encodeURIComponent(uri)}`;
        const resp = await fetch(url, { credentials: 'include' });
        const content = resp.ok ? await resp.text() : '';
        const mod = await import('../utils/dialogBus.js');
        mod.updateFileViewDialog({ title, uri, content, loading: false });
    } catch (e) {
        try {
            const mod = await import('../utils/dialogBus.js');
            mod.updateFileViewDialog({ content: String(e?.message || e), loading: false });
        } catch (_) {
        }
    }
    return true;
}

export async function explorerSearch(props) {
    const context = props?.context;
    if (!context) return false;
    const ds = context?.handlers?.dataSource;
    if (!ds) return false;
    const form = ds.peekFormData?.() || {};

    // Accept either "include"/"exclude" OR "inclusion"/"exclusion".
    const include = String(form.include || form.inclusion || '').trim();
    const exclude = String(form.exclude || form.exclusion || '').trim();
    const pattern = String(form.pattern || form.query || '').trim();
    const root = String(form.root || form.path || '').trim();
    const showFiles = !!(form.showFiles ?? true);

    if (!pattern) {
        ds.setError?.('Missing pattern');
        return false;
    }

    // NOTE: This is a tool feed for a *resources* tool call. The actual search
    // is performed by the tool call itself (resources.grepFiles), and the feed
    // renders its output.
    return true;
}

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
        console.log('[chat.onInit] start');
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
        // If a convID was provided via window params, seed the conversations DS immediately
        try {
            const wp = context?.windowParams || {};
            const fromParam = wp.convID || wp.conversationId || wp.id;
            if (fromParam) {
                handlers?.setFormData?.({ values: { id: fromParam } });
                try {
                    const inSig = convCtx?.signals?.input;
                    if (inSig) {
                        const cur = (typeof inSig.peek === 'function') ? (inSig.peek() || {}) : (inSig.value || {});
                        inSig.value = { ...cur, id: fromParam, filter: { id: fromParam } };
                    }
                } catch (_) { /* ignore */ }
            }
        } catch (_) { /* ignore */ }
        const start = Date.now();
        const deadline = start + 1000;
        const timer = setInterval(async () => {
            let convID = '';
            try {
                const inSnap = convCtx?.signals?.input?.peek?.() || {};
                const form = handlers?.peekFormData?.() || {};
                convID = form.id
                    || inSnap.id
                    || (inSnap.filter && inSnap.filter.id)
                    || (inSnap.parameters && (inSnap.parameters.convID || inSnap.parameters.id));
                console.log('[chat.onInit] detect convID', { form, input: inSnap, convID });
            } catch (e) {
                console.warn('[chat.onInit] detect convID error', e);
            }
            if (convID) {
                clearInterval(timer);
                try {
                    handlers?.setFormData?.({values: {id: convID}});
                    console.log('[chat.onInit] set conversations form id', convID);
                } catch (_) {
                }
                // Ensure conversation exists; if not, create one using agent from form when available
                try {
                    const apiBase = (endpoints?.agentlyAPI?.baseURL || (typeof window !== 'undefined' ? window.location.origin : '')).replace(/\/+$/, '');
                    const checkUrl = `${apiBase}/v1/api/conversations/${encodeURIComponent(convID)}`;
                    console.log('[chat.onInit] GET conversation', checkUrl);
                    const resp = await fetch(checkUrl, { credentials: 'include' });
                    if (resp && resp.status === 404) {
                        // Try create-on-miss using agent id from conversations form
                        let agentId = '';
                        try { agentId = (handlers?.peekFormData?.()?.agent || '').trim(); } catch (_) {}
                        if (!agentId) {
                            try {
                                const inSnap2 = convCtx?.signals?.input?.peek?.() || {};
                                agentId = String((inSnap2.parameters && inSnap2.parameters.agent) || '').trim();
                            } catch (_) {}
                        }
                        if (!agentId) {
                            try {
                                const metaCtx = context.Context('meta');
                                const metaCol = metaCtx?.handlers?.dataSource?.peekCollection?.() || [];
                                const data = metaCol[0] || {};
                                agentId = String((data.defaults && data.defaults.agent) || '').trim();
                            } catch (_) {}
                        }
                        console.log('[chat.onInit] 404 – createOnMiss with agent', agentId);
                        if (agentId) {
                            const listUrl = `${apiBase}/v1/api/conversations?createIfMissing=1&agentId=${encodeURIComponent(agentId)}`;
                            console.log('[chat.onInit] createOnMiss GET', listUrl);
                            const listResp = await fetch(listUrl, { credentials: 'include' });
                            const json = await listResp.json().catch(() => ({}));
                            const arr = (json && (json.data || json)) || [];
                            if (Array.isArray(arr) && arr.length > 0 && arr[0].id) {
                                convID = arr[0].id;
                                try { handlers?.setFormData?.({ values: { id: convID, agent: agentId } }); } catch (_) {}
                                console.log('[chat.onInit] created conversation', convID);
                            }
                        }
                    }
                } catch (_) { /* ignore */ }
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
                        console.log('[chat.onInit] messages initial fetch', next);
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
            // No active conversation: don’t wipe the messages collection repeatedly
            // (it interferes with history rendering). Just ensure loading is off.
            try {
                const msgCtx = context.Context('messages');
                const ctrlSig = msgCtx?.signals?.control;
                if (ctrlSig) {
                    const prev = (typeof ctrlSig.peek === 'function') ? (ctrlSig.peek() || {}) : (ctrlSig.value || {});
                    if (prev.loading) ctrlSig.value = { ...prev, loading: false };
                }
            } catch(_) {}
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
            const stageLower = String(convStage || '').toLowerCase();
            const hasBusyStage = (
                stageLower === 'thinking' ||
                stageLower === 'running' ||
                stageLower === 'processing' ||
                stageLower === 'waiting_for_user'
            );
            const anyActiveSteps = rows.some((row) => {
                const executions = Array.isArray(row?.executions) ? row.executions : [];
                return executions.some((ex) => hasActiveSteps(ex?.steps || []));
            });
            // Optional debug: log current turn + step status histogram
            try {
                // removed debug summary
            } catch(_) {}
            if (rows.length) {
                receiveMessages(messagesCtx, rows, since);
            }
            // Safety release: if DS still loading but no active steps, force unlock
            try {
                const ctrlSig = messagesCtx?.signals?.control;
                if (ctrlSig) {
                    const prev = (typeof ctrlSig.peek === 'function') ? (ctrlSig.peek() || {}) : (ctrlSig.value || {});
                    if (prev.loading && !anyActiveSteps) {
                        // removed debug unlock log
                        ctrlSig.value = { ...prev, loading: false };
                    }
                }
            } catch(_) {}
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
                    const tokensWithCacheText = (() => {
                        const base = totalTokensText || '';
                        const cache = promptCachedTokensText || '';
                        return cache ? `${base} (cached ${cache})` : base;
                    })();
                    const values = { ...norm, cost, costText, totalTokensText, promptCachedTokensText, tokensWithCacheText };
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
                    if (agent) {
                        const agentID = String(agent).trim();
                        ds2.setFormField?.({item: {id: 'agent'}, value: agentID});
                        try {
                            const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
                            const agentName = String(metaForm?.agentInfo?.[agentID]?.name || metaForm?.agentInfo?.[agentID]?.Name || '').trim();
                            if (agentName) ds2.setFormField?.({item: {id: 'agentName'}, value: agentName});
                        } catch (_) {}
                    }
                    const model = conv?.DefaultModel || conv?.Model || conv?.model || '';
                    if (model) ds2.setFormField?.({item: {id: 'model'}, value: String(model)});
                }
            } catch (_) { /* ignore */ }
            // Derive running state from active steps / stage / turn statuses
            // to keep Abort/Queue UX accurate.
            // Important: do not bind "running" to the messages DS loading state, otherwise
            // the Forge Chat composer becomes read-only and users can't enqueue messages.
            try {
                const turns = Array.isArray(transcript) ? transcript : [];
                const isBusyTurnStatus = (value) => {
                    const status = String(value || '').toLowerCase();
                    return (
                        status === 'running' ||
                        status === 'waiting_for_user' ||
                        status === 'thinking' ||
                        status === 'processing'
                    );
                };

                let latestStatus = '';
                for (let i = turns.length - 1; i >= 0; i--) {
                    const status = String(turns[i]?.status || turns[i]?.Status || '').trim();
                    if (status) {
                        latestStatus = status;
                        break;
                    }
                }

                const latestTurnBusy = isBusyTurnStatus(latestStatus);
                // Some backends may leave conversation.stage as "running" after completion.
                // Prefer execution steps + latest turn status for accuracy; stage is only a fallback
                // when no transcript exists yet.
                const isRunning = !!anyActiveSteps || !!latestTurnBusy || (!!hasBusyStage && turns.length === 0);
                const convCtx = context.Context('conversations');
                if (convCtx?.handlers?.dataSource?.setFormField) {
                    convCtx.handlers.dataSource.setFormField({ item: { id: 'running' }, value: !!isRunning });
                }
            } catch (_) {}

            // Derive queued items from transcript for integrated queue UX (badge/popover).
            try {
                const queuedTurns = buildQueuedTurnsFromTranscript(transcript);
                const convCtx = context.Context('conversations');
                const ds = convCtx?.handlers?.dataSource;
                if (ds?.setFormField) {
                    ds.setFormField({ item: { id: 'queuedCount' }, value: queuedTurns.length });
                    ds.setFormField({ item: { id: 'queuedTurns' }, value: queuedTurns });
                }
            } catch (_) {}
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

const queuedRequestTagPrefix = 'agently:queued_request:';

function extractQueuedRequest(tags) {
    try {
        const raw = String(tags || '');
        const idx = raw.indexOf(queuedRequestTagPrefix);
        if (idx === -1) return null;
        let jsonPart = raw.slice(idx + queuedRequestTagPrefix.length).trim();
        if (!jsonPart) return null;
        try {
            return JSON.parse(jsonPart);
        } catch (_) {
            const last = jsonPart.lastIndexOf('}');
            if (last !== -1) {
                jsonPart = jsonPart.slice(0, last + 1);
                return JSON.parse(jsonPart);
            }
        }
        return null;
    } catch (_) {
        return null;
    }
}

function pickTurnStartedMessage(turn) {
    try {
        const startedByMessageId = turn?.startedByMessageId || turn?.StartedByMessageId || turn?.started_by_message_id;
        const messages = Array.isArray(turn?.message) ? turn.message
            : Array.isArray(turn?.Message) ? turn.Message : [];
        if (!messages.length) return null;
        if (startedByMessageId) {
            const match = messages.find(m => String(m?.id || m?.Id || '').trim() === String(startedByMessageId).trim());
            if (match) return match;
        }
        const userMsg = messages.find(m => String(m?.role || m?.Role || '').toLowerCase() === 'user');
        return userMsg || messages[0] || null;
    } catch (_) {
        return null;
    }
}

function normalizePreview(text) {
    const s = String(text || '').replace(/\s+/g, ' ').trim();
    if (!s) return '';
    const maxLen = 120;
    if (s.length <= maxLen) return s;
    return s.slice(0, maxLen - 1) + '…';
}

function pickFirstStringField(obj, fieldNames) {
    const src = obj && typeof obj === 'object' ? obj : null;
    if (!src) return '';
    for (const fieldName of fieldNames) {
        const v = src[fieldName];
        if (typeof v === 'string' && v.trim()) return v.trim();
    }
    return '';
}

function pickFirstStringFieldDeep(obj, fieldNames) {
    const direct = pickFirstStringField(obj, fieldNames);
    if (direct) return direct;
    const src = obj && typeof obj === 'object' ? obj : null;
    if (!src) return '';
    for (const key of ['row', 'item', 'record', 'node', 'data', 'value', 'payload', 'selected', 'selection', 'parameters', 'input', 'inputParameters']) {
        const inner = src[key];
        const nested = pickFirstStringField(inner, fieldNames);
        if (nested) return nested;
    }
    return '';
}

function buildQueuedTurnsFromTranscript(transcript) {
    const turns = Array.isArray(transcript) ? transcript : [];
    const queued = turns
        .filter(t => String(t?.status || t?.Status || '').toLowerCase() === 'queued')
        .map(t => {
            const startedMessage = pickTurnStartedMessage(t);
            const queuedMeta = extractQueuedRequest(startedMessage?.tags || startedMessage?.Tags);
            const tools = Array.isArray(queuedMeta?.tools) ? queuedMeta.tools : [];
            const turnID = (
                t?.id ||
                t?.Id ||
                t?.turnId ||
                t?.TurnId ||
                t?.turnID ||
                t?.TurnID
            );
            const content = String(startedMessage?.content || startedMessage?.Content || '');
            return {
                id: turnID,
                conversationId: t?.conversationId || t?.ConversationId,
                status: t?.status || t?.Status,
                createdAt: toISOSafe(t?.createdAt || t?.CreatedAt),
                queueSeq: t?.queueSeq ?? t?.QueueSeq,
                startedByMessageId: t?.startedByMessageId || t?.StartedByMessageId,
                content,
                preview: normalizePreview(content),
                overrides: {
                    agent: queuedMeta?.agent || '',
                    model: queuedMeta?.model || '',
                    tools: tools,
                },
            };
        });
    queued.sort((a, b) => {
        const aSeq = (a?.queueSeq == null) ? null : Number(a.queueSeq);
        const bSeq = (b?.queueSeq == null) ? null : Number(b.queueSeq);
        if (aSeq != null && bSeq != null && Number.isFinite(aSeq) && Number.isFinite(bSeq) && aSeq !== bSeq) {
            return aSeq - bSeq;
        }
        const aTime = Date.parse(a?.createdAt || '');
        const bTime = Date.parse(b?.createdAt || '');
        if (!Number.isNaN(aTime) && !Number.isNaN(bTime) && aTime !== bTime) {
            return aTime - bTime;
        }
        return String(a?.id || '').localeCompare(String(b?.id || ''));
    });
    return queued;
}

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
        // Elicitation correlation (OOB accept): bubble through IDs when present
        elicitationId: tc.elicitationId || tc.ElicitationId,
        elicitationPayloadId: tc.elicitationPayloadId || tc.ElicitationPayloadId,
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
    const dbgOn = true;
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
            if (s1) { /* debug log removed */ }
            const s2 = buildToolStepFromToolCall(tc);
            if (s2) { /* debug log removed */ }
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
                } catch (_) {}
                // Fallback: some tool control messages carry fields on the envelope but empty content
                if (!elic && (m.elicitationId || m.ElicitationId)) {
                    elic = {
                        elicitationId: (m.elicitationId || m.ElicitationId),
                        message: (m.message || m.Message || ''),
                        requestedSchema: (m.requestedSchema || m.RequestedSchema || {}),
                        url: (m.url || m.URL || ''),
                        mode: (m.mode || m.Mode || ''),
                        callbackURL: (m.callbackURL || m.CallbackURL || ''),
                    };
                    // debug log removed
                }
                if (elic) {
                    const created = m?.createdAt || m?.CreatedAt;
                    const updated = m?.updatedAt || m?.UpdatedAt || created;
                    const isLast = (m.id || m.Id) === globalLastMsgId;
                    // Decouple execution details from dialog rendering: always include elicitation step
                    // so the timeline reflects pending/accepted/rejected regardless of dialog state.
                    const includeNow = true;
                    if (includeNow) {
                        s3 = {
                            id: (m.id || m.Id || '') + '/elicitation',
                            name: 'elicitation',
                            reason: 'elicitation',
                            successBool: (status2 === 'accepted'),
                            statusText: status2 || 'pending',
                            originRole: roleLower2,
                            startedAt: created,
                            endedAt: updated,
                            responsePayloadId: payloadId,
                            elicitationPayloadId: payloadId,
                            elicitation: {
                                message: elic.message || elic.prompt || '',
                                requestedSchema: elic.requestedSchema || {},
                                url: elic.url || '',
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
                                // If user data is present, treat this elicitation as accepted
                                s3.successBool = true;
                                s3.statusText = 'accepted';
                            }
                        } catch (_) {
                        }
                        // If server provided a payload id, consider it accepted for table rendering
                        if (payloadId && !s3.successBool) {
                            s3.successBool = true;
                            s3.statusText = 'accepted';
                        }
                        // debug log removed
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

        // 1b) OOB elicitation acceptance reconciliation
        // If a tool call reports accepted elicitation (with payload id), update the
        // corresponding elicitation step status/payload so UI reflects acceptance.
        try {
            // Map last elicitation step per elicitationId
            const lastElicById = new Map();
            for (let i = 0; i < steps.length; i++) {
                const s = steps[i];
                if (s && s.reason === 'elicitation') {
                    const eid = s?.elicitation?.elicitationId;
                    if (eid) lastElicById.set(String(eid), s);
                }
            }
            // Walk tool calls for accepted status carrying elicitation linkage
            for (const s of steps) {
                if (!s || s.reason !== 'tool_call') continue;
                const st = String(s.statusText || '').toLowerCase();
                const eid = s.elicitationId || s.ElicitationId;
                const pid = s.elicitationPayloadId || s.ElicitationPayloadId;
                if (!eid) continue;
                if (st === 'accepted' || st === 'done' || st === 'succeeded' || s.success === true) {
                    const target = lastElicById.get(String(eid));
                    if (target) {
                        target.statusText = 'accepted';
                        target.successBool = true;
                        if (pid && !target.elicitationPayloadId) target.elicitationPayloadId = pid;
                    }
                }
            }
        } catch (_) {
        }

        // Sort steps by timestamp (prefer startedAt, fallback endedAt)
        steps.sort((a, b) => {
            const ta = a?.startedAt || a?.endedAt || '';
            const tb = b?.startedAt || b?.endedAt || '';
            const da = ta ? new Date(ta).getTime() : 0;
            const db = tb ? new Date(tb).getTime() : 0;
            return da - db;
        });

        // Collapse duplicate elicitation steps by elicitationId – keep the latest state
        try {
            const byEid = new Map(); // eid -> index of last occurrence
            for (let i = 0; i < steps.length; i++) {
                const s = steps[i];
                if (!s || String(s.reason||'').toLowerCase() !== 'elicitation') continue;
                const eid = s?.elicitation?.elicitationId;
                if (!eid) continue;
                byEid.set(String(eid), i);
            }
            if (byEid.size > 0) {
                const keep = new Set(byEid.values());
                const collapsed = [];
                for (let i = 0; i < steps.length; i++) {
                    const s = steps[i];
                    if (String(s?.reason||'').toLowerCase() !== 'elicitation') {
                        collapsed.push(s);
                        continue;
                    }
                    const eid = s?.elicitation?.elicitationId;
                    if (!eid) { collapsed.push(s); continue; }
                    if (keep.has(i)) {
                        // If the kept one has no terminal status but any later tool acceptance exists, it will have been reconciled above
                        collapsed.push(s);
                    }
                }
                steps.splice(0, steps.length, ...collapsed);
            }
        } catch(_) {}

        // Optional debug trace per turn
        try {
            // removed per-turn debug log
        } catch(_) {}

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
                        // Treat presence of user data as acceptance for execution status
                        try { target.statusText = 'accepted'; target.successBool = true; } catch(_) {}
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
                // No localStorage-based suppression; polling is temporarily paused while the dialog is open
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
            if (isControlElicitation || (roleLower === 'assistant' && !!elic)) {
                if (isElicitationSuppressed(thisId)) {
                    continue;
                }
            }

            // Row usage derived from model call only when attached to this row later; leave null here.
            const rowRole = (isControlElicitation || (roleLower === 'assistant' && !!elic)) ? 'elicition' : roleLower;
            const prefRaw = (() => { try { const r = m.rawContent || m.RawContent; return (typeof r === 'string' && r.trim().length > 0) ? r : ''; } catch(_) { return ''; } })();
            const row = {
                id,
                conversationId: m.conversationId || m.ConversationId,
                // For any elicitation row we allow (assistant last+pending or tool pending), force synthetic role.
                role: rowRole,
                name: (m.createdByUserId || m.CreatedByUserId || ''),
                // Do not show any bubble content for elicitation rows; dialog carries the UI.
                content: (isControlElicitation || (roleLower === 'assistant' && !!elic)) ? '' : (prefRaw || m.content || m.Content || ''),
                createdAt,
                toolName: m.toolName || m.ToolName,
                turnId: turnIdRef,
                parentId: turnIdRef,
                executions: [],
                usage: null,
                // Preserve normalized backend status so accepted/failed states do not remount dialogs
                status: String(status).toLowerCase(),
                elicitation: elic,
                callbackURL,
            };
            if (rowRole !== 'elicition') {
                turnRows.push(row);
            }
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
            // Compute execution signature (used to ensure publish on meaningful changes)
            const execSignature = (() => {
                try {
                    const parts = (steps||[]).map(s => {
                        const r = String(s?.reason||'');
                        const st = String(s?.statusText||'').toLowerCase();
                        const eid = s?.elicitation?.elicitationId || '';
                        const mid = s?.id || '';
                        return `${r}:${st}:${eid}:${mid}`;
                    });
                    return parts.join('|');
                } catch(_) { return ''; }
            })();

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
                _execSignature: execSignature,
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

            // Reorder within turn: user → execution → tool feed → elicitation dialogs → others
            const userRows = turnRows.filter(r => r && r.role === 'user');
            const otherRows = turnRows.filter(r => !userRows.includes(r));
            // Push user rows first (usually one)
            for (const r of userRows) rows.push(r);
            // Then execution details
            rows.push(execRow);
            // Then tool feed (if any)
            if (toolRow) rows.push(toolRow);
            // Then elicitation dialog rows derived solely from message content (independent of execution)
            try {
                const dlg = buildElicitationDialogRows(messages, turnId, (turn?.conversationId || turn?.ConversationId), globalLastMsgId);
                for (const r of dlg) {
                    try { markElicitationShown(r.id, 2500); } catch(_) {}
                    rows.push(r);
                }
            } catch(_) {}
            // Finally the remaining non-dialog rows (assistant/etc)
            for (const r of otherRows) {
                if (r?.role === 'elicition') continue; // skip; dialogs added above
                rows.push(r);
            }
            // Continue to error bubble handling below if applicable
            continue;
        }

        // 3b) If we have steps but no visible base rows, still render execution + dialogs
        if (steps.length && !turnRows.length) {
            // Build execution row for this turn
            const execSignature = (() => {
                try {
                    const parts = (steps||[]).map(s => {
                        const r = String(s?.reason||'');
                        const st = String(s?.statusText||'').toLowerCase();
                        const eid = s?.elicitation?.elicitationId || '';
                        const mid = s?.id || '';
                        return `${r}:${st}:${eid}:${mid}`;
                    });
                    return parts.join('|');
                } catch(_) { return ''; }
            })();

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
                _execSignature: execSignature,
                turnStatus,
                turnError,
                turnCreatedAt,
                turnUpdatedAt,
                turnElapsedSec,
                isLastTurn,
            };
            rows.push(execRow);
            // Tool feed (if any)
            const toolExec = Array.isArray(turn?.ToolExecution) ? turn.ToolExecution
                : Array.isArray(turn?.toolExecution) ? turn.toolExecution
                    : Array.isArray(turn?.ToolFeed) ? turn.ToolFeed
                        : Array.isArray(turn?.toolFeed) ? turn.toolFeed : [];
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
            if (toolRow) rows.push(toolRow);
            // Dialog rows
            try {
                const dlg = buildElicitationDialogRows(messages, turnId, (turn?.conversationId || turn?.ConversationId), globalLastMsgId);
                for (const r of dlg) { try { markElicitationShown(r.id, 2500); } catch(_) {}; rows.push(r); }
            } catch(_) {}
            // Skip to next turn
            continue;
        }

        // No execution steps – push rows as-is, then any dialog rows based on message content
        for (const r of turnRows) rows.push(r);
        try {
            const dlg = buildElicitationDialogRows(messages, turnId, (turn?.conversationId || turn?.ConversationId), globalLastMsgId);
            for (const r of dlg) {
                try { markElicitationShown(r.id, 2500); } catch(_) {}
                rows.push(r);
            }
        } catch(_) {}

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

// -------------------------------- Elicitation dialog builder --------------------------------

function safeParseJSON(s) {
    try { return JSON.parse(s ?? ''); } catch { return null; }
}

function isTerminalDialogStatus(st) {
    const v = String(st || '').toLowerCase();
    return ['accepted','done','succeeded','success','rejected','declined','failed','error','canceled'].includes(v);
}

function normalizeDialogStatus(st) {
    const v = String(st || '').toLowerCase();
    if (!v || v === 'open') return 'pending';
    return v;
}

// Determines if the dialog represented by message should be considered already resolved
function hasResolvedElicitation(message) {
    try {
        const ued = message?.UserElicitationData || message?.userElicitationData;
        const hasUED = !!(ued && (ued.InlineBody !== undefined || ued.inlineBody !== undefined));
        const hasPayload = !!(message?.elicitationPayloadId || message?.ElicitationPayloadId || message?.payloadId || message?.PayloadId);
        const st = String(message?.status || message?.Status || '').toLowerCase();
        return hasUED || hasPayload || isTerminalDialogStatus(st);
    } catch(_) { return false; }
}

// Extracts elicitation data from assorted message shapes. Returns null when none/rejected to mount.
function extractElicitation(message) {
    const dbgOn = true;
    const mid = message?.id || message?.Id;
    const roleLower = String(message?.role || message?.Role || '').toLowerCase();
    const typeLower = String(message?.type || message?.Type || '').toLowerCase();
    const statusRaw = message?.status || message?.Status || '';
    const status = normalizeDialogStatus(statusRaw);
    const createdAt = message?.createdAt || message?.CreatedAt;
    const updatedAt = message?.updatedAt || message?.UpdatedAt || createdAt;

    // Fast exit: if clearly resolved, don’t build dialog
    if (hasResolvedElicitation(message)) {
        // debug log removed
        return null;
    }

    // Sources for elicitation info
    let obj = null;
    // 1) direct field (tool/assistant already pre-parsed upstream sometimes)
    if (message?.elicitation || message?.Elicitation) {
        const e = message.elicitation || message.Elicitation;
        if (e && (e.requestedSchema || e.elicitationId)) obj = e;
    }
    // 2) content JSON
    if (!obj && (message?.content || message?.Content)) {
        const maybe = typeof (message.content || message.Content) === 'string'
            ? safeParseJSON(message.content || message.Content)
            : (message.content || message.Content);
        if (maybe && typeof maybe === 'object' && (maybe.requestedSchema || maybe.elicitationId)) {
            obj = maybe;
        }
    }
    // 3) envelope fields (control messages or assistant with empty content)
    if (!obj && (message?.elicitationId || message?.ElicitationId)) {
        obj = {
            elicitationId: (message.elicitationId || message.ElicitationId),
            message: (message.message || message.Message || ''),
            requestedSchema: (message.requestedSchema || message.RequestedSchema || {}),
            url: (message.url || message.URL || ''),
            callbackURL: (message.callbackURL || message.CallbackURL || ''),
        };
    }

    const callbackURL = obj?.callbackURL || message?.callbackURL || message?.CallbackURL || '';
    const elicitationId = obj?.elicitationId || obj?.ElicitationId;
    const requestedSchema = obj?.requestedSchema || {};
    const prompt = obj?.message || obj?.prompt || '';
    const urlVal = obj?.url || message?.url || message?.URL || '';
    const modeVal = obj?.mode || message?.mode || message?.Mode || '';

    const hasCore = !!elicitationId && !!requestedSchema;
    if (!obj || !hasCore) {
        // debug log removed
        return null;
    }

    // Only allow when pending/open
    if (!(status === 'pending' || status === 'open')) {
        // debug log removed
        return null;
    }

    // debug log removed

    return {
        id: mid,
        role: 'elicition',
        content: '',
        createdAt,
        updatedAt,
        status: status,
        elicitation: { requestedSchema, message: prompt, elicitationId, url: urlVal, mode: modeVal, callbackURL },
        callbackURL,
        turnId: message?.turnId || message?.TurnId,
        parentId: message?.turnId || message?.TurnId,
        conversationId: message?.conversationId || message?.ConversationId,
    };
}

// Build a list of dialog rows for a turn, collapsing duplicates by elicitationId (latest wins)
function buildElicitationDialogRows(messages = [], turnIdHint = '', convIdHint = '', lastMsgId = '') {
    const dbgOn = true;
    const byEid = new Map();
    const byEidOrder = [];
    for (const m of (Array.isArray(messages) ? messages : [])) {
        const mid = m?.id || m?.Id;
        const ex = extractElicitation(m);
        if (!ex) continue;
        // Ensure structural fields present even when message lacks turn/conversation ids
        if (!ex.turnId && turnIdHint) ex.turnId = turnIdHint;
        if (!ex.parentId && turnIdHint) ex.parentId = turnIdHint;
        if (!ex.conversationId && convIdHint) ex.conversationId = convIdHint;

        // Collapse by elicitationId
        const eid = ex?.elicitation?.elicitationId || `m:${mid}`;
        if (!byEid.has(eid)) byEidOrder.push(eid);
        byEid.set(eid, ex); // overwrite => latest wins
    }
    const out = byEidOrder.map(eid => byEid.get(eid)).filter(Boolean);
    // debug log removed
    return out;
}

// Detect if any step remains active
export function hasActiveSteps(steps) {
    try {
        const arr = Array.isArray(steps) ? steps : [];
        for (const s of arr) {
            const reason = String(s?.reason || '').toLowerCase();
            if (!reason || reason === 'error' || reason === 'link') continue;
            const st = String(s?.statusText || '').toLowerCase();
            if (st === '' || st === 'pending' || st === 'open' || st === 'running' || st === 'processing' || (typeof s.successBool === 'boolean' && s.successBool === false)) {
                return true;
            }
        }
        return false;
    } catch(_) {
        return false;
    }
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
    const { context } = props;
    try {
        const fsCtx = context?.Context('fs');
        const sel = fsCtx?.handlers?.dataSource?.getSelection?.();
        const picked = sel?.selected ?? sel;
        const value = (() => {
            if (!picked) return '';
            if (typeof picked === 'string') return picked;
            return picked.uri || picked.url || picked.path || picked.id || picked.name || '';
        })();
        // Commit with value so awaitResult resolves for the caller
        if (value) {
            context?.handlers?.dialog?.commit?.(value);
        } else {
            context?.handlers?.dialog?.commit?.(picked || null);
        }
    } catch (_) {
        try { context?.handlers?.dialog?.commit?.(null); } catch (_) {}
    }
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
        // Merge with the current DS state to avoid regressing execution steps
        // when the fetch snapshot lags behind dsTick’s merge.
        let prev = [];
        try {
            const msgCtx = context?.Context?.('messages');
            prev = Array.isArray(msgCtx?.signals?.collection?.peek?.()) ? (msgCtx.signals.collection.peek() || []) : [];
        } catch(_) {}

        const dbgOn = (() => { try { const v = (localStorage.getItem('agently_debug_exec')||'').toLowerCase(); return v==='1'||v==='true'; } catch(_) { return false; } })();
        const byIdPrev = new Map((prev||[]).map(r => [r?.id, r]).filter(([k,v]) => !!k && !!v));
        const byIdBuilt = new Map((built||[]).map(r => [r?.id, r]).filter(([k,v]) => !!k && !!v));
        // debug log removed

        const merged = [];
        const allIds = Array.from(new Set([...(prev||[]).map(r=>r?.id), ...(built||[]).map(r=>r?.id)])).filter(Boolean);

        function pickExecutionRow(oldRow, newRow) {
            if (!oldRow && newRow) return newRow;
            if (oldRow && !newRow) return oldRow;
            try {
                const pSig = String(oldRow?._execSignature || '');
                const nSig = String(newRow?._execSignature || '');
                if (pSig !== nSig) { return newRow; }
            } catch(_) {}
            try {
                const pSteps = oldRow?.executions?.[0]?.steps || [];
                const nSteps = newRow?.executions?.[0]?.steps || [];
                const pE = pSteps.filter(s=>String(s?.reason||'').toLowerCase()==='elicitation');
                const nE = nSteps.filter(s=>String(s?.reason||'').toLowerCase()==='elicitation');
                const term = (st) => {
                    const s = String(st||'').toLowerCase();
                    if (['accepted','done','succeeded','success'].includes(s)) return 3;
                    if (['rejected','declined'].includes(s)) return 2;
                    if (['error','failed','canceled'].includes(s)) return 1;
                    return 0; // pending/open/unknown
                };
                const score = (arr) => arr.reduce((m,s)=>Math.max(m, term(s?.statusText)), 0);
                const pScore = score(pE);
                const nScore = score(nE);
                // Prefer the row with a more terminal elicitation status; tie-break on step count
                if (pScore > nScore) return oldRow;
                if (nScore > pScore) return newRow;
                if (nSteps.length > pSteps.length) return newRow;
                // Prefer the row with newer turnUpdatedAt/createdAt timestamps as a final tie-breaker
                const pTs = new Date(newRow?.turnUpdatedAt || newRow?.createdAt || 0).getTime();
                const oTs = new Date(oldRow?.turnUpdatedAt || oldRow?.createdAt || 0).getTime();
                if (pTs > oTs) return newRow;
                return oldRow;
            } catch(_) {
                return newRow || oldRow;
            }
        }

        for (const id of allIds) {
            const oldRow = byIdPrev.get(id);
            const newRow = byIdBuilt.get(id);
            if (!oldRow && newRow) { merged.push(newRow); continue; }
            if (oldRow && !newRow) { merged.push(oldRow); continue; }
            if (String(id||'').endsWith('/execution')) {
                const picked = pickExecutionRow(oldRow, newRow);
                // debug log removed
                merged.push(picked);
            } else {
                // For non-execution rows, prefer built snapshot to keep ordering and text fresh
                merged.push(newRow || oldRow);
            }
        }

        return merged;
    } catch (_) {
        return [];
    }
}

// onFetch handler for queued turns DS: filter transcript to queued turns only.
export function onFetchQueuedTurns(props) {
    try {
        const {collection} = props || {};
        const transcript = Array.isArray(collection) ? collection : [];
        return buildQueuedTurnsFromTranscript(transcript).filter(t => !!String(t?.id || '').trim());
    } catch (_) {
        return [];
    }
}

function resolveConversationIDFromContext(context, props) {
    try {
        // 1) If handler props include a conversation id (some Forge event adapters do)
        const fromProps = pickFirstStringFieldDeep(props || {}, [
            'conversationID', 'conversationId',
            'convID', 'convId',
            'id', 'Id',
        ]);
        if (fromProps) return fromProps;

        // 2) Prefer parent conversations DataSource (normal case)
        try {
            const convCtx = context?.Context?.('conversations');
            const convDS = convCtx?.handlers?.dataSource;
            const convForm = convDS?.peekFormData?.() || {};
            const fromForm = String(convForm?.id || convForm?.Id || '').trim();
            if (fromForm) return fromForm;
            const sel = convDS?.getSelection?.()?.selected;
            const fromSel = String(sel?.id || sel?.Id || '').trim();
            if (fromSel) return fromSel;
        } catch (_) {}

        // 3) Fallback: use the current DataSource input (queueTurns dialog context)
        // The queueTurns DataSource is opened with parameters.id bound from conversations:form.id.
        try {
            const ds = context?.handlers?.dataSource;
            const inputObj =
                ds?.peekInput?.() ||
                ds?.peekInputData?.() ||
                ds?.getInputData?.() ||
                ds?.signals?.input?.peek?.() ||
                ds?.signals?.input?.value;
            const fromInput = pickFirstStringFieldDeep(inputObj || {}, [
                'id', 'Id',
                'convID', 'convId',
                'conversationID', 'conversationId',
            ]);
            if (fromInput) return fromInput;
        } catch (_) {}

        // 4) Fallback: use the last active conversation tracked by conversationService
        const active = String(getActiveConversationID?.() || '').trim();
        if (active) return active;

        return '';
    } catch (_) {
        return '';
    }
}

function resolveQueueTurnIDFromEventProps(props) {
    const turnID = pickFirstStringFieldDeep(props || {}, [
        'id', 'Id',
        'turnId', 'TurnId',
        'turnID', 'TurnID',
    ]);
    if (turnID) return turnID;
    // Some Forge table handlers pass the row object under `row` but the full record under `item.data`.
    try {
        const row = props?.row || props?.item || props?.record;
        const deep = pickFirstStringFieldDeep(row || {}, ['id', 'Id', 'turnId', 'TurnId', 'turnID', 'TurnID']);
        return deep;
    } catch (_) {
        return '';
    }
}

function refreshQueueTurns(context) {
    try {
        const directDS = context?.handlers?.dataSource;
        if (directDS?.fetchCollection) {
            directDS.fetchCollection();
            return;
        }
    } catch (_) {}
    try {
        const qCtx = context?.Context?.('queueTurns');
        qCtx?.handlers?.dataSource?.fetchCollection?.();
    } catch (_) {}
}

function refreshMessages(context) {
    try {
        const msgCtx = context?.Context?.('messages');
        msgCtx?.handlers?.dataSource?.fetchCollection?.();
    } catch (_) {}
}

export async function cancelQueuedTurnByID({context, conversationID, turnID}) {
    try {
        if (!context) return false;
        const convID = String(conversationID || '').trim();
        const tID = String(turnID || '').trim();
        if (!convID || !tID) return false;

        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/turns/${encodeURIComponent(tID)}`;
        const resp = await fetch(url, {method: 'DELETE', credentials: 'include'});
        if (!resp.ok) {
            const txt = await resp.text().catch(() => '');
            throw new Error(txt || `cancel failed: ${resp.status}`);
        }

        refreshQueueTurns(context);
        refreshMessages(context);
        return true;
    } catch (err) {
        log.error('chatService.cancelQueuedTurnByID error', err);
        return false;
    }
}

export async function cancelQueuedTurn(props) {
    try {
        const context = props?.context;
        if (!context) return false;
        const convID = resolveConversationIDFromContext(context, props);
        if (!convID) return false;

        const turnID = resolveQueueTurnIDFromEventProps(props);
        if (!turnID) return false;

        return await cancelQueuedTurnByID({context, conversationID: convID, turnID});
    } catch (err) {
        log.error('chatService.cancelQueuedTurn error', err);
        return false;
    }
}

export async function moveQueuedTurnUp(props) {
    const context = props?.context;
    if (!context) return false;
    const convID = resolveConversationIDFromContext(context, props);
    const turnID = resolveQueueTurnIDFromEventProps(props);
    if (!convID || !turnID) return false;
    return moveQueuedTurn({context, conversationID: convID, turnID, direction: 'up'});
}

export async function moveQueuedTurnDown(props) {
    const context = props?.context;
    if (!context) return false;
    const convID = resolveConversationIDFromContext(context, props);
    const turnID = resolveQueueTurnIDFromEventProps(props);
    if (!convID || !turnID) return false;
    return moveQueuedTurn({context, conversationID: convID, turnID, direction: 'down'});
}

export async function moveQueuedTurn({context, conversationID, turnID, direction}) {
    try {
        if (!context) return false;
        const convID = String(conversationID || '').trim();
        const tID = String(turnID || '').trim();
        const dir = String(direction || '').trim().toLowerCase();
        if (!convID || !tID || (dir !== 'up' && dir !== 'down')) return false;

        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const url = `${base}/v1/api/conversations/${encodeURIComponent(convID)}/turns/${encodeURIComponent(tID)}/move`;
        const resp = await fetch(url, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ direction: dir }),
        });
        if (!resp.ok) {
            const txt = await resp.text().catch(() => '');
            throw new Error(txt || `move failed: ${resp.status}`);
        }

        refreshQueueTurns(context);
        refreshMessages(context);
        return true;
    } catch (err) {
        log.error('chatService.moveQueuedTurn error', err);
        return false;
    }
}

async function postConversationMessage({context, conversationID, body}) {
    const convID = String(conversationID || '').trim();
    if (!context || !convID || !body) return null;
    const messagesContext = context.Context?.('messages');
    const messagesAPI = messagesContext?.connector;
    if (!messagesAPI?.post) return null;
    return await messagesAPI.post({inputParameters: {convID}, body});
}

export async function editQueuedTurn(props) {
    try {
        const context = props?.context;
        if (!context) return false;
        const convID = resolveConversationIDFromContext(context, props);
        const turnID = resolveQueueTurnIDFromEventProps(props);
        if (!convID || !turnID) return false;

        const row = props?.row || props?.item || props?.record || {};
        const currentContent = String(row?.content || row?.Content || '').trim();
        const initial = currentContent || String(row?.preview || row?.Preview || '').trim();
        const next = window.prompt('Edit queued message', initial);
        if (next == null) return false;
        const nextContent = String(next || '').trim();
        if (!nextContent) return false;
        if (currentContent && nextContent === currentContent) return false;

        const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
        const overrides = row?.overrides || row?.Overrides || {};
        const toolsOverride = Array.isArray(overrides?.tools) ? overrides.tools : [];
        const body = {
            content: nextContent,
            tools: toolsOverride.length ? toolsOverride : (metaForm.tool || []),
            agent: String(overrides?.agent || metaForm.agent || ''),
            model: String(overrides?.model || metaForm.model || ''),
            toolCallExposure: metaForm.toolCallExposure,
            reasoningEffort: metaForm.reasoningEffort,
            autoSummarize: metaForm.autoSummarize,
            disableChains: metaForm.disableChains,
            allowedChains: metaForm.allowedChains || [],
        };

        // Cancel old queued turn first, then enqueue the edited one.
        const cancelled = await cancelQueuedTurnByID({context, conversationID: convID, turnID});
        if (!cancelled) return false;
        await postConversationMessage({context, conversationID: convID, body});

        refreshQueueTurns(context);
        refreshMessages(context);
        return true;
    } catch (err) {
        log.error('chatService.editQueuedTurn error', err);
        return false;
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


    // removed debug snapshot log
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
        // Keep conversation header in sync with the selected agent so the chat panel
        // reflects what will be used for the next turns.
        try {
            const convCtx = context?.Context?.('conversations');
            const convDS = convCtx?.handlers?.dataSource;
            convDS?.setFormField?.({item: {id: 'agent'}, value: key});
            const agentName = String(form?.agentInfo?.[key]?.name || form?.agentInfo?.[key]?.Name || '').trim();
            if (agentName) convDS?.setFormField?.({item: {id: 'agentName'}, value: agentName});
        } catch (_) {}
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

    const {agent, model, tool, toolCallExposure, reasoningEffort, autoSummarize, disableChains, allowedChains=[]} = metaForm

    const messagesContext = context.Context('messages');
    const messagesAPI = messagesContext.connector;

    try {
        // Voice control support: allow mic/dictation phrases like "cancel it now"
        // or "submit it now" embedded in the message.
        const originalContent = String(message?.content || '');
        const vc = detectVoiceControl(originalContent);
        if (vc.action === 'cancel') {
            // Clear draft and avoid sending anything.
            message.content = '';
            pendingUploads = [];
            return;
        }
        if (vc.action === 'submit') {
            // Strip the control phrase before submit.
            message.content = vc.cleanedText;
        }

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
            content: message.content, tools: tool,
            agent, model, toolCallExposure, reasoningEffort, autoSummarize, disableChains, allowedChains,
        }

        // If voice command reduced content to empty and there are no attachments,
        // treat it as a no-op (avoid creating empty messages).
        if (!String(body.content || '').trim() && pendingUploads.length === 0) {
            return;
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
        // removed debug log

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
        if (!resp.ok) {
            const text = await resp.text().catch(() => '');
            throw new Error(text || `HTTP ${resp.status}`);
        }

        // 204 → no body; 202 → JSON body, but we treat both as successful termination requests.
        try {
            setStage({phase: 'terminated'});
        } catch (_) {}
        try {
            const convCtx2 = context.Context('conversations');
            convCtx2?.handlers?.dataSource?.setFormField?.({item: {id: 'running'}, value: false});
        } catch (_) {}
        try {
            const msgCtx = context.Context('messages');
            msgCtx?.handlers?.dataSource?.fetchCollection?.();
        } catch (_) {}
        try {
            refreshQueueTurns(context);
        } catch (_) {}
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
            // Force publish when execution steps signature changes
            const isExec = typeof updated?.id === 'string' && updated.id.endsWith('/execution');
            const prevSig = isExec ? String(prev?._execSignature || '') : '';
            const nextSig = isExec ? String(updated?._execSignature || '') : '';
            if (isExec && prevSig !== nextSig) {
                // debug log removed
                current[idx] = updated;
                changed = true;
            } else if (!deepEqualShallow(prev, updated)) {
                // debug log removed
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
        const acceptedElicitationIds = new Set(); // correlate across different message ids
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
                        try {
                            const eid = s?.elicitation?.elicitationId;
                            if (accepted && eid) acceptedElicitationIds.add(String(eid));
                        } catch(_) {}
                    }
                }
            }
        }
        if (hideIds.size > 0 || resolvedElicitationBaseIds.size > 0 || acceptedElicitationIds.size > 0) {
            const collSig = messagesContext?.signals?.collection;
            if (collSig && Array.isArray(collSig.value)) {
                const before = collSig.value.length;
                collSig.value = collSig.value.filter(row => {
                    const id = row?.id;
                    if (!id) return true;
                    if (hideIds.has(id)) return false;
                    // Drop stale elicitation dialog rows when an accepted step for the same message id was observed
                    if ((row?.role === 'elicition') && resolvedElicitationBaseIds.has(id)) return false;
                    // Also drop by elicitationId correlation across assistant/tool control messages
                    try {
                        const eid = row?.elicitation?.elicitationId;
                        if (row?.role === 'elicition' && eid && acceptedElicitationIds.has(String(eid))) return false;
                    } catch(_) {}
                    return true;
                });
                const after = collSig.value.length;
                try {
                    log.debug('[chat] receiveMessages: purged rows', {
                        removed: before - after,
                        removedTypes: {
                            replies: hideIds.size,
                            resolvedElicitations: resolvedElicitationBaseIds.size,
                            resolvedByEid: acceptedElicitationIds.size,
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
    cancelQueuedTurn,
    cancelQueuedTurnByID,
    moveQueuedTurn,
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
    onFetchQueuedTurns,
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
    const convCtx = context?.Context?.('conversations');
    const convDS = convCtx?.handlers?.dataSource;

    const updated = collection.map(data => {
        const agentInfo = data.agentInfo || {};
        const agentsRaw = Array.isArray(data?.agents)
            ? data.agents
            : (data?.agentInfo ? Object.keys(data.agentInfo) : []);

        const modelsRaw = Array.isArray(data?.models)
            ? data.models
            : (data?.defaults?.model ? [data.defaults.model] : []);

        const toolsRaw = Array.isArray(data?.tools) ? data.tools : [];
        const toolInfo = data?.toolInfo || {};
        const toolBundlesRaw = Array.isArray(data?.toolBundles) ? data.toolBundles : [];

        // TreeMultiSelect expects a flat option list and will build the tree
        // by splitting the value with properties.separator. Keep options flat
        // and let the widget handle grouping to avoid runtime errors.

        const agentChainTargets = {};
        Object.entries(agentInfo).forEach(([k, v]) => {
            agentChainTargets[k] = Array.isArray(v?.chains) ? v.chains : [];
        });
        // Preserve current agent selection if present; otherwise use defaults
        const curAgent = String(currentForm.agent || data.defaults.agent || '');
        const curAgentName = (agentInfo?.[curAgent]?.name) ? String(agentInfo[curAgent].name) : curAgent;

        // Keep chat header in sync: header binds to conversations.agentName.
        try {
            if (convDS?.setFormField) {
                if (curAgent) convDS.setFormField({ item: { id: 'agent' }, value: curAgent });
                if (curAgentName) convDS.setFormField({ item: { id: 'agentName' }, value: curAgentName });
            }
        } catch (_) { /* ignore */ }

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
                const toolInfoEntry = toolInfo?.[raw] || toolInfo?.[String(raw).trim()];
                const bundle = Array.isArray(toolInfoEntry?.bundles) && toolInfoEntry.bundles.length
                    ? String(toolInfoEntry.bundles[0])
                    : '';
                const bundleLabel = (() => {
                    if (!bundle) return '';
                    const def = (toolBundlesRaw || []).find(b => String(b?.id || '').toLowerCase() === bundle.toLowerCase());
                    return def?.title ? String(def.title) : '';
                })();
                const groupKey = raw.replaceAll('/', '-');
                return { id: raw, value: raw, label: raw, groupKey, bundle, bundleLabel };
            }),
            agentChainTargets,
            ...settings,

        };
    });
    return updated;
}

//
