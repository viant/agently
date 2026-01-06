// ExecutionDetails.jsx – moved from Forge to Agently for domain-specific execution UI

import React, { useMemo, useEffect } from "react";
import {BasicTable as Basic} from "forge/components";
import { addWindow } from "forge/core";
import { Dialog } from "@blueprintjs/core";
import JsonViewer from "../JsonViewer.jsx";
import { signal } from "@preact/signals-react";
import { endpoints } from "../../endpoint";

// Global window-level signals (copied unchanged)
import {
    getCollectionSignal,
    getControlSignal,
    getSelectionSignal,
} from "forge/core";

import {BasicTable} from "../../../../../forge/index.js";
// no extra util needed

// Column template; dynamic handlers injected later
const COLUMNS_BASE = [
    { id: "icon",    name: "",         width: 28, align: "center", minWidth: "28px", enforceColumnSize: false },
    { id: "kind",    name: "Kind",      width: 40, align: "center", minWidth: "68px" },
    { id: "name",    name: "Name",      flex: 1 },
    { id: "actor",   name: "Actor",     width: 100 },
    { id: "content", name: "Content",   flex: 8 },
    { id: "chain",   name: "Thread",    width: 80, type: 'button', cellProperties: { text: 'Open', minimal: true, small: true }, on: [ { event: "onClick", handler: "exec.openLinkedConversation" }, { event: 'onVisible', handler: 'exec.isLinkRow' } ] },
    { id: "oobOpen", name: "",          width: 66, type: 'button', cellProperties: { text: 'open', minimal: true, small: true }, on: [ { event: 'onClick', handler: 'exec.openOOB' }, { event: 'onVisible', handler: 'exec.isElicitationWithURL' } ] },
    { id: "status",  name: "Status",    width: 50 },
    { id: "elapsed", name: "Time",      width: 50 },
    {
        id: "detail",
        name: "Detail",
        width: 70,
        type: "button",
        cellProperties: { text: "details 🔍", minimal: true, small: true },
        on: [ { event: "onClick", handler: "exec.openDetail" } ],
    },
];

function buildExecutionContext(parentContext, dataSourceId, openDialog, viewPart) {
    const collectionSig = getCollectionSignal(dataSourceId);
    const controlSig    = getControlSignal(dataSourceId);
    const selectionSig  = getSelectionSignal(dataSourceId, { selection: [] });

    const fakeHandlers = {
        dataSource: {
            getCollection: () => collectionSig.peek(),
            peekCollection: () => collectionSig.peek(),
            getSelection: () => selectionSig.peek(),
            setAllSelection: () => {
                const all = collectionSig.peek().map((_, idx) => ({ rowIndex: idx }));
                selectionSig.value = { selection: all };
            },
            resetSelection: () => {
                selectionSig.value = { selection: [] };
            },
            isSelected: ({ rowIndex }) => {
                const sel = selectionSig.peek().selection || [];
                return sel.some((s) => s.rowIndex === rowIndex);
            },
            setSilentFilterValues: () => {},
            peekFilter: () => ({}),
            getFilterSets: () => [],
        },
    };

    const toggleSelection = ({ rowIndex }) => {
        const sel = selectionSig.peek().selection || [];
        const already = sel.some((s) => s.rowIndex === rowIndex);
        if (already) {
            selectionSig.value = { selection: [] };
        } else {
            selectionSig.value = { selection: [{ rowIndex }] };
        }
    };

    const lookupHandler = (id) => {
        switch (id) {
            case "exec.openRequest":
                return ({ row }) => viewPart('request', row);
            case "exec.openResponse":
                return ({ row }) => viewPart('response', row);
            case "exec.openProviderRequest":
                return ({ row }) => viewPart('providerRequest', row);
            case "exec.openProviderResponse":
                return ({ row }) => viewPart('providerResponse', row);
            case "dataSource.toggleSelection":
                return ({ rowIndex }) => toggleSelection({ rowIndex });
            case "dataSource.isSelected":
                return ({ rowIndex }) => selectionSig.peek().selection?.some((s)=>s.rowIndex===rowIndex);
            case "exec.openElicitation":
                return ({ row }) => viewPart('elicitation', row);
            case "exec.openDetail":
                return ({ row }) => openDialog('Details', { kind: 'detail', row });
            case "exec.openLinkedConversation":
                return async (ctx) => {
                    const row = ctx?.row || {};
                    const colId = (ctx && (ctx.col?.id || ctx.columnId || ctx.column?.id || ctx.colId)) || '';
                    if (colId !== 'chain') {
                        console.log('[exec.openLinkedConversation] skip: clicked column is', colId);
                        return;
                    }
                    try {
                        const linked = row?._linkedConversationId;
                        if (!linked) {
                            console.warn('[exec] openLinkedConversation: missing _linkedConversationId', row);
                            return;
                        }
                        // Open using the same windowKey as Chat History, but
                        // pass per‑DS parameters in the proper addWindow slot.
                        try {
                            const dsParams = {
                                conversations: { parameters: { id: linked } },
                                messages:      { parameters: { convID: linked } },
                            };
                            // Try to propagate agent from the current window to support create-on-miss
                            try {
                                const convCtx = parentContext?.Context ? parentContext.Context('conversations') : null;
                                const metaCtx = parentContext?.Context ? parentContext.Context('meta') : null;
                                const curAgent = (convCtx?.handlers?.dataSource?.peekFormData?.()?.agent || '').trim();
                                let defAgent = '';
                                try {
                                    const metaCol = metaCtx?.handlers?.dataSource?.peekCollection?.() || [];
                                    defAgent = String((metaCol[0]?.defaults?.agent || '')).trim();
                                } catch (_) {}
                                const agentId = curAgent || defAgent;
                                if (agentId) {
                                    dsParams.conversations.parameters.agent = agentId;
                                }
                            } catch (_) {}
                            const title = 'Link Chat';
                            console.log('[exec.openLinkedConversation] opening', { title, params: dsParams });
                            // Reuse existing window when parameters (convID/id) match (no newInstance)
                            // and auto‑index title for different conversations of the same type
                            addWindow(title, null, 'chat/new', null, true, dsParams, { autoIndexTitle: true });
                            return;
                        } catch (e) {
                            console.error('[exec] addWindow failed; falling back to hash nav', e);
                        }
                        // Fallback: hash navigation (kept as last resort)
                        try {
                            const href = `#/chat/new?convID=${encodeURIComponent(linked)}`;
                            window.location.hash = href;
                        } catch (_) {}
                    } catch (e) { console.error('[exec] openLinkedConversation failed', e); }
                };
            case "exec.isLinkRow":
                return ({ row }) => !!(row && row._reason === 'link' && row._linkedConversationId);
            case "exec.isElicitationWithURL":
                return ({ row }) => {
                    try {
                        return String(row?._reason || '').toLowerCase() === 'elicitation' && !!(row?._elicitation?.url);
                    } catch (_) { return false; }
                };
            case "exec.openOOB":
                return async ({ row }) => {
                    try {
                        const url = row?._elicitation?.url;
                        const cb = row?._elicitation?.callbackURL || row?.callbackURL;
                        if (url) {
                            try { window.open(url, '_blank', 'noopener,noreferrer'); } catch(_) {}
                        }
                        if (cb) {
                            const base = endpoints?.agentlyAPI?.baseURL || (typeof window !== 'undefined' ? window.location.origin : '');
                            const root = (base || '').replace(/\/+$/, '');
                            const abs = /^https?:\/\//i.test(cb) ? cb : `${root}${cb.startsWith('/') ? '' : '/'}${cb}`;
                            try { await fetch(abs, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action: 'accept', payload: {} }), credentials: 'include' }); } catch(_) {}
                        }
                    } catch(_) {}
                };
            default:
                return parentContext?.lookupHandler ? parentContext.lookupHandler(id) : () => {};
        }
    };

    return {
        ...parentContext,
        signals: {
            collection: collectionSig,
            control: controlSig,
            selection: selectionSig,
            message:  signal([]),
            form:     signal({}),
            input:    signal({}),
            collectionInfo: signal({}),
        },
        dataSource: {
            selectionMode: "single",
            paging: { enabled: false, size: 0 },
        },
        handlers: {
            ...(parentContext?.handlers || {}),
            ...fakeHandlers,
        },
        lookupHandler,
        tableSettingKey: (id) => `exec-table-${id}`,
    };
}

function flattenExecutions(executions = []) {
    if (!executions) return [];
    const allowed = new Set([ 'thinking', 'tool_call', 'elicitation', 'link', 'error' ]);
    const rows = executions.flatMap(exe => (exe.steps || [])
        .filter(s => allowed.has(String(s?.reason || '').toLowerCase()))
        .map(s => {
            const reason = String(s?.reason || '').toLowerCase();
            const hasBool = typeof s.successBool === 'boolean';
            const successBool = hasBool ? s.successBool : (typeof s.success === 'boolean' ? s.success : undefined);
            let statusText = (s.statusText || (successBool === undefined ? 'pending' : (successBool ? 'completed' : 'error'))).toLowerCase();
            // Elicitation special-case: if we have a submitted payload id, treat as accepted
            if (String(s?.reason || '').toLowerCase() === 'elicitation') {
                try {
                    const hasPayload = !!(s.elicitationPayloadId);
                    if (hasPayload && (statusText === '' || statusText === 'pending' || statusText === 'open')) {
                        statusText = 'accepted';
                    }
                } catch (_) {}
            }
            const hasError = !!(s.error || s.errorCode) || statusText === 'failed' || statusText === 'error' || statusText === 'canceled';
            // Status icon: check for all completed-like states, hourglass while in-progress, exclamation on error
            // Treat rejected/declined as terminal to avoid perpetual hourglass
            const isDoneOk = ['completed','accepted','done','succeeded','success','rejected','declined'].includes(statusText);
            const icon = hasError ? '❗' : (isDoneOk ? '✅' : '⏳');
            // Kind glyph: brain for model (thinking), tool for tool_call, keyboard for elicitation, link for link, warning for error
            const kindGlyph = reason === 'thinking' ? '🧠'
                : reason === 'tool_call' ? '🛠️'
                : reason === 'link' ? '🔗'
                : reason === 'error' ? '⚠️'
                : '⌨️';
            const annotatedName = reason === 'elicitation' && s.originRole
                ? `${s.name} (${s.originRole})`
                : reason === 'link' ? 'Thread'
                : reason === 'error' ? (s.error || 'Error')
                : (s.name || reason);
            const elapsedDisplay = reason === 'link' ? '🔗' : s.elapsed;
            // Derive human content when available (elicitation prompt, error message, etc.)
            let content = '';
            try {
                if (reason === 'elicitation') {
                    content = (s.elicitation && (s.elicitation.message || '')) || '';
                } else if (reason === 'error') {
                    content = String(s.error || '');
                }
            } catch(_) {}
            return {
                icon,
                kind: kindGlyph,
                name: annotatedName,
                actor: s.createdByUserId || '',
                content,
                chain: (reason === 'link') ? 'Open' : '',
                status: statusText,
                elapsed: s.elapsed,
                requestPayloadId: s.requestPayloadId,
                responsePayloadId: s.responsePayloadId,
                streamPayloadId: s.streamPayloadId,
                providerRequestPayloadId: s.providerRequestPayloadId,
                providerResponsePayloadId: s.providerResponsePayloadId,
                _reason: reason,
                _provider: s.provider,
                _model: s.model,
                _createdByUserId: s.createdByUserId,
                _linkedConversationId: s.linkedConversationId,
                _finishReason: s.finishReason,
                _errorCode: s.errorCode,
                _error: s.error,
                _attempt: s.attempt,
                _startedAt: s.startedAt,
                _endedAt: s.endedAt,
                _toolName: s.toolName || s.name,
                _elicitation: s.elicitation,
                _userData: s.userData,
                _originRole: s.originRole,
                _promptTokens: s.promptTokens,
                _promptCachedTokens: s.promptCachedTokens,
                _promptAudioTokens: s.promptAudioTokens,
                _completionTokens: s.completionTokens,
                _completionReasoningTokens: s.completionReasoningTokens,
                _completionAudioTokens: s.completionAudioTokens,
                _totalTokens: s.totalTokens,
                elicitationPayloadId: s.elicitationPayloadId,
            };
        }));

    return rows;
}

function ExecutionDetails({ executions = [], context, messageId, turnStatus, turnError, onError, useForgeDialog = false, resizable = false, useCodeMirror = false }) {
    const [dialog, setDialog] = React.useState(null); // Details or generic payload viewer
    const [payloadDialog, setPayloadDialog] = React.useState(null); // Secondary dialog for payloads when details is open
    const [dlgSize, setDlgSize] = React.useState({ width: 960, height: 640 });
    const [dlgPos, setDlgPos] = React.useState({ left: 120, top: 80 });
    const [payloadPos, setPayloadPos] = React.useState({ left: 160, top: 120 });
    const dataSourceId = `ds${messageId ?? ""}`;
    const rows = useMemo(() => flattenExecutions(executions), [executions]);
    // removed table debug log
    // Derive a single turn-level error message (if any) to render as a table footer.
    const errorFooter = React.useMemo(() => {
        // 1) Prefer explicit error step message when present
        const allSteps = (executions || []).flatMap(exe => exe.steps || []);
        const errSteps = allSteps.filter(s => String(s?.reason || '').toLowerCase() === 'error' && (s?.error || s?.statusText === 'failed'));
        if (errSteps.length) {
            const last = errSteps[errSteps.length - 1];
            const msg = String(last.error || last.statusText || 'failed');
            if (msg.trim()) return msg.trim();
        }
        // 2) Fall back to the turn-level error when provided
        if ((String(turnStatus || '').toLowerCase() === 'failed' || String(turnStatus || '').toLowerCase() === 'error') && turnError) {
            const msg = String(turnError);
            if (msg.trim()) return msg.trim();
        }
        return null;
    }, [executions, turnStatus, turnError]);

    useEffect(() => {
        const sig = getCollectionSignal(dataSourceId);
        sig.value = rows;
    }, [rows, dataSourceId]);


    const viewPart = async (part, row) => {
        try {
            const title = part === 'request' ? 'Request' : part === 'providerRequest' ? 'Provider Request' : part === 'providerResponse' ? 'Provider Response' : (part === 'elicitation' ? 'Elicitation' : (part === 'stream' ? 'Stream' : 'Response'));
            const setWhich = (dialog && dialog.kind && String(dialog.kind).startsWith('detail-')) ? setPayloadDialog : setDialog;
            if (!useForgeDialog) setWhich({ title, payload: null, loading: true });
            // Use only the provided payload ID fields
            // Prefer provider-specific payloads when available
            const pid = part === 'request'
                ? (row.requestPayloadId)
                : part === 'providerRequest'
                    ? (row.providerRequestPayloadId)
                    : part === 'response'
                        ? (row.responsePayloadId)
                        : part === 'providerResponse'
                            ? (row.providerResponsePayloadId)
                            : part === 'elicitation'
                                ? (row.elicitationPayloadId)
                                : row.streamPayloadId;
            let url;
            if (!pid) { setDialog({ title, payload: '(no payload)' }); return; }
            // Build absolute or relative URL robustly
            const base = endpoints?.agentlyAPI?.baseURL || (typeof window !== 'undefined' ? window.location.origin : '');
            const root = (base || '').replace(/\/+$/,'');
            url = `${root}/v1/api/payload/${encodeURIComponent(pid)}`;

            // If requested, open as a Forge dialog (resizable) and return early
            if (useForgeDialog && context?.handlers?.window?.openDialog) {
                try {
                    const execArgs = [
                        'payloadViewer',        // dialog id expected in Forge metadata
                        title,                   // window title
                        {
                            awaitResult: false,
                            resizable: true,
                            width: 960,
                            height: 720,
                            params: { url },
                        },
                    ];
                    await context.handlers.window.openDialog({ execution: { args: execArgs } });
                    return;
                } catch (e) {
                    // Bubble error and fall back to inline dialog
                    if (typeof onError === 'function') {
                        try { onError(e); } catch (_) {}
                    }
                    // continue to fetch and render inline below
                    if (!useForgeDialog) setWhich({ title, payload: null, loading: true });
                }
            }
            const resp = await fetch(url, { credentials: 'include' });
            if (!resp.ok) throw new Error(`${resp.status}`);
            const ct = resp.headers.get('content-type') || '';
            const text = await resp.text();
            let payload = text;
            if (ct.includes('application/json')) {
                try { payload = JSON.parse(text); } catch (_) {}
            }
            setWhich({ title, payload, loading: false, contentType: ct, kind: part });
        } catch (err) {
            if (typeof onError === 'function') {
                try { onError(err); } catch (_) { /* ignore */ }
            }
            const setWhich = (dialog && dialog.kind && String(dialog.kind).startsWith('detail-')) ? setPayloadDialog : setDialog;
            setWhich({ title: 'Error', payload: String(err) });
        }
    };

    const execContext = useMemo(
        () => {
            const ctx = buildExecutionContext(context, dataSourceId, (title, payload) => setDialog({ title, payload }), viewPart);
            const originalLookup = ctx.lookupHandler;
            ctx.lookupHandler = (id) => {
                if (id === 'exec.openRequest') return ({ row }) => viewPart('request', row);
                if (id === 'exec.openResponse') return ({ row }) => viewPart('response', row);
                if (id === 'exec.openStream') return ({ row }) => viewPart('stream', row);
                if (id === 'exec.openDetail') return async ({ row }) => {
                    try {
                        setDialog({ title: 'Details', kind: `detail-${row?._reason || ''}`, row });
                    } catch (_) {}
                };
                return originalLookup ? originalLookup(id) : () => {};
            };
            return ctx;
        },
        [context, dataSourceId]
    );

    return (
        <>
            <Basic
                context={execContext}
                container={{ id: `exec-${messageId}`, table: { enforceColumnSize: false, fullWidth: false } }}
                columns={COLUMNS_BASE}
            />

            {errorFooter && (
                <div style={{
                    width: '100%',
                    maxWidth: '80vw',
                    borderTop: '1px solid var(--light-gray1)',
                    background: 'rgba(205, 92, 92, 0.08)',
                    padding: '6px 10px',
                    marginTop: 4,
                    color: 'var(--red3)',
                    fontSize: 12,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                }}>
                    <span role="img" aria-label="error">⚠️</span>
                    <span style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{errorFooter}</span>
                </div>
            )}

            <Dialog
                isOpen={!!dialog}
                onClose={() => setDialog(null)}
                title={dialog?.title || ""}
                className={resizable ? 'agently-resizable-dialog' : undefined}
                style={resizable
                    ? { width: dlgSize.width, height: dlgSize.height, minWidth: 480, minHeight: 320, maxWidth: '95vw', maxHeight: '95vh' }
                    : { width: "60vw", minWidth: "60vw", minHeight: "60vh" }
                }
            >
                {dialog && dialog.kind && String(dialog.kind).startsWith('detail-') && (() => {
                    const r = dialog.row || {};
                    const kind = String(dialog.kind).slice('detail-'.length);
                    if (kind === 'thinking') {
                        return (
                            <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
                                <div><strong>Model:</strong> {r._model || r.name}</div>
                                <div><strong>Provider:</strong> {r._provider || ''}</div>
                                {r._createdByUserId && <div><strong>Actor:</strong> {r._createdByUserId}</div>}
                                <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginTop: 6 }}>
                                    {typeof r._promptTokens === 'number' && (
                                        <div><strong>Prompt Total</strong>: {r._promptTokens}</div>
                                    )}
                                    {typeof r._promptCachedTokens === 'number' && (
                                        <div><strong>Cached</strong>: {r._promptCachedTokens}</div>
                                    )}
                                    {typeof r._promptAudioTokens === 'number' && (
                                        <div><strong>Prompt Audio</strong>: {r._promptAudioTokens}</div>
                                    )}
                                    {typeof r._completionTokens === 'number' && (
                                        <div><strong>Completion Total</strong>: {r._completionTokens}</div>
                                    )}
                                    {typeof r._completionReasoningTokens === 'number' && (
                                        <div><strong>Completion Reasoning</strong>: {r._completionReasoningTokens}</div>
                                    )}
                                    {typeof r._completionAudioTokens === 'number' && (
                                        <div><strong>Completion Audio</strong>: {r._completionAudioTokens}</div>
                                    )}
                                    {typeof r._totalTokens === 'number' && (
                                        <div><strong>Total</strong>: {r._totalTokens}</div>
                                    )}
                                </div>
                                {r._finishReason && <div><strong>Finish Reason:</strong> {r._finishReason}</div>}
                                {r._errorCode && <div><strong>Error Code:</strong> {String(r._errorCode)}</div>}
                                {r._error && <div style={{color: 'red'}}><strong>Error Message:</strong> {String(r._error)}</div>}
                                <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
                                    <div style={{ display: 'flex', gap: 8 }}>
                                        <button className="bp4-button bp4-small" disabled={!r.requestPayloadId} onClick={() => viewPart('request', r)}>Open Request</button>
                                        <button className="bp4-button bp4-small" disabled={!r.providerRequestPayloadId} onClick={() => viewPart('providerRequest', r)}>Open Provider Request</button>
                                    </div>
                                    <div style={{ display: 'flex', gap: 8, width: '100%', marginTop: 6 }}>
                                        <button className="bp4-button bp4-small" disabled={!r.responsePayloadId} onClick={() => viewPart('response', r)}>Open Response</button>
                                        <button className="bp4-button bp4-small" disabled={!r.providerResponsePayloadId} onClick={() => viewPart('providerResponse', r)}>Open Provider Response</button>
                                    </div>
                                    <div style={{ display: 'flex', gap: 8, width: '100%', marginTop: 6 }}>
                                        <button className="bp4-button bp4-small" disabled={!r.streamPayloadId} onClick={() => viewPart('stream', r)}>Open Stream</button>
                                    </div>
                                </div>
                            </div>
                        );
                    }
                    if (kind === 'tool_call') {
                        return (
                            <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
                                <div><strong>Tool:</strong> {r._toolName || r.name}</div>
                                {r._createdByUserId && <div><strong>Actor:</strong> {r._createdByUserId}</div>}
                                {typeof r._attempt === 'number' && <div><strong>Attempt:</strong> {r._attempt}</div>}
                                {r._errorCode && <div><strong>Error Code:</strong> {String(r._errorCode)}</div>}
                                {r._error && <div style={{color: 'red'}}><strong>Error Message:</strong> {String(r._error)}</div>}
                                <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                                    <button className="bp4-button bp4-small" disabled={!r.requestPayloadId} onClick={() => viewPart('request', r)}>Open Request</button>
                                    <button className="bp4-button bp4-small" disabled={!r.responsePayloadId} onClick={() => viewPart('response', r)}>Open Response</button>
                                </div>
                            </div>
                        );
                    }
                    if (kind === 'elicitation') {
                        const schema = r?._elicitation?.requestedSchema;
                        const prompt = r?._elicitation?.message;
                        const url = r?._elicitation?.url;
                        const hasPayload = !!r?.elicitationPayloadId;
                        return (
                            <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
                                <div><strong>Origin:</strong> {r._originRole === 'assistant' ? 'LLM' : (r._originRole || 'unknown')}</div>
                                {r._createdByUserId && <div><strong>Actor:</strong> {r._createdByUserId}</div>}
                                {prompt && <div><strong>Message:</strong> {prompt}</div>}
                                {url && <div><strong>URL:</strong> <a href={url} target="_blank" rel="noopener noreferrer">{url}</a></div>}
                                {schema && (
                                    <div>
                                        <div style={{marginBottom: 4}}><strong>Requested Schema:</strong></div>
                                        <JsonViewer value={schema} useCodeMirror={useCodeMirror} height={'200px'} language={'json'} />
                                    </div>
                                )}
                                {r?._userData && (
                                    <div>
                                        <div style={{marginBottom: 4}}><strong>User Data (inline):</strong></div>
                                        <JsonViewer value={r._userData} useCodeMirror={useCodeMirror} height={'200px'} language={'json'} />
                                    </div>
                                )}
                                <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                                    <button className="bp4-button bp4-small" disabled={!hasPayload} onClick={() => viewPart('elicitation', r)}>View Submitted Payload</button>
                                </div>
                            </div>
                        );
                    }

                    if (kind === 'error') {
                        return (
                            <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
                                <div><strong>Status:</strong> error</div>
                                {r._model && <div><strong>Model:</strong> {r._model}</div>}
                                {r._provider && <div><strong>Provider:</strong> {r._provider}</div>}
                                {r._errorCode && <div><strong>Error Code:</strong> {String(r._errorCode)}</div>}
                                {r._error && <div style={{color: 'red'}}><strong>Error Message:</strong> {String(r._error)}</div>}
                                <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                                    <button className="bp4-button bp4-small" disabled={!r.requestPayloadId} onClick={() => viewPart('request', r)}>Open Request</button>
                                    <button className="bp4-button bp4-small" disabled={!r.responsePayloadId} onClick={() => viewPart('response', r)}>Open Response</button>
                                </div>
                            </div>
                        );
                    }

                    if (kind === 'link') {
                        const linked = r._linkedConversationId || '';
                        return (
                            <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
                                <div><strong>Thread Link</strong></div>
                                <div><strong>Conversation ID:</strong> {linked || '(unknown)'} </div>
                                <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                                    <button
                                        className="bp4-button bp4-small"
                                        disabled={!linked}
                                        onClick={async () => {
                                            try {
                                                const fn = execContext.lookupHandler('exec.openLinkedConversation');
                                                if (typeof fn === 'function') {
                                                    await fn({ row: r, col: { id: 'chain' } });
                                                }
                                            } catch (_) {}
                                        }}
                                    >Open Thread</button>
                                </div>
                            </div>
                        );
                    }

                    // Fallback: show raw row as JSON so Details is never empty
                    try {
                        return (
                            <div style={{ padding: 12 }}>
                                <JsonViewer value={r} useCodeMirror={useCodeMirror} height={'60vh'} language={'json'} />
                            </div>
                        );
                    } catch (_) { return null; }
                })()}
                {dialog && (!dialog.kind || !String(dialog.kind).startsWith('detail-')) && (
                    <div style={{ position: 'relative', display: 'flex', flexDirection: 'column', gap: 8, padding: 12, height: resizable ? 'calc(100% - 24px)' : 'auto', maxHeight: resizable ? 'none' : '70vh', paddingRight: resizable ? 20 : 12, paddingBottom: resizable ? 20 : 12 }}>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                            <button
                                type="button"
                                className="bp4-button bp4-small"
                                onClick={async () => {
                                    try {
                                        const text = typeof dialog.payload === 'string' ? dialog.payload : JSON.stringify(dialog.payload, null, 2);
                                        await navigator.clipboard.writeText(text);
                                    } catch (e) {
                                        if (typeof onError === 'function') {
                                            try { onError(e); } catch (_) {}
                                        }
                                    }
                                }}
                            >Copy</button>
                        </div>
                        <div style={{ flex: 1, minHeight: resizable ? 0 : undefined, overflow: 'auto' }}>
                            {dialog.loading && <span>Loading …</span>}
                            {!dialog.loading && dialog.payload !== null && (() => {
                                const ct = (dialog.contentType || '').toLowerCase();
                                const isString = typeof dialog.payload === 'string';
                                const looksHTML = isString && /<\s*(table|thead|tbody|tr|td|th|div|span|p|html|body)\b/i.test(dialog.payload);
                                // Render raw HTML payloads as HTML (e.g., elicitation tables)
                                if (ct.includes('text/html') || looksHTML) {
                                    return (
                                        <div
                                            className="prose max-w-full"
                                            style={{ width: '100%', overflowX: 'auto' }}
                                            dangerouslySetInnerHTML={{ __html: String(dialog.payload) }}
                                        />
                                    );
                                }
                                // Otherwise, show JSON or plain text via JsonViewer
                                const language = (dialog.kind === 'stream' || (ct && !ct.includes('application/json'))) ? 'text' : 'json';
                                return (
                                    <JsonViewer
                                        value={dialog.payload}
                                        useCodeMirror={useCodeMirror}
                                        height={resizable ? 'calc(100% - 8px)' : '60vh'}
                                        language={language}
                                    />
                                );
                            })()}
                        </div>
                        {resizable && (
                            <div
                                onMouseDown={(e) => {
                                    const startX = e.clientX;
                                    const startY = e.clientY;
                                    const startW = dlgSize.width;
                                    const startH = dlgSize.height;
                                    const onMove = (evt) => {
                                        const dx = evt.clientX - startX;
                                        const dy = evt.clientY - startY;
                                        const w = Math.max(480, startW + dx);
                                        const h = Math.max(320, startH + dy);
                                        setDlgSize({ width: w, height: h });
                                    };
                                    const onUp = () => {
                                        window.removeEventListener('mousemove', onMove);
                                        window.removeEventListener('mouseup', onUp);
                                    };
                                    window.addEventListener('mousemove', onMove);
                                    window.addEventListener('mouseup', onUp);
                                }}
                                title="Drag to resize"
                                style={{
                                    position: 'absolute',
                                    right: 6,
                                    bottom: 6,
                                    width: 14,
                                    height: 14,
                                    background: 'rgba(0,0,0,0.2)',
                                    borderRadius: 2,
                                    cursor: 'se-resize',
                                    zIndex: 5
                                }}
                            />
                        )}
                    </div>
                )}
            </Dialog>

            <Dialog
                isOpen={!!payloadDialog}
                onClose={() => setPayloadDialog(null)}
                title={payloadDialog?.title || ""}
                style={{ width: '70vw', minWidth: '60vw', minHeight: '60vh' }}
            >
                {payloadDialog && (
                    <div style={{ position: 'relative', display: 'flex', flexDirection: 'column', gap: 8, padding: 12, maxHeight: '70vh' }}>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                            <button
                                type="button"
                                className="bp4-button bp4-small"
                                onClick={async () => {
                                    try {
                                        const text = typeof payloadDialog.payload === 'string' ? payloadDialog.payload : JSON.stringify(payloadDialog.payload, null, 2);
                                        await navigator.clipboard.writeText(text);
                                    } catch (_) {}
                                }}
                            >Copy</button>
                        </div>
                        <div style={{ flex: 1, overflow: 'auto' }}>
                            {payloadDialog.loading && <span>Loading …</span>}
                            {!payloadDialog.loading && payloadDialog.payload !== null && (() => {
                                const ct = (payloadDialog.contentType || '').toLowerCase();
                                const isString = typeof payloadDialog.payload === 'string';
                                const looksHTML = isString && /<\s*(table|thead|tbody|tr|td|th|div|span|p|html|body)\b/i.test(payloadDialog.payload);
                                if (ct.includes('text/html') || looksHTML) {
                                    return (
                                        <div className="prose max-w-full" style={{ width: '100%', overflowX: 'auto' }} dangerouslySetInnerHTML={{ __html: String(payloadDialog.payload) }} />
                                    );
                                }
                                const language = (payloadDialog.kind === 'stream' || (ct && !ct.includes('application/json'))) ? 'text' : 'json';
                                return (
                                    <JsonViewer value={payloadDialog.payload} useCodeMirror={useCodeMirror} height={'60vh'} language={language} />
                                );
                            })()}
                        </div>
                    </div>
                )}
            </Dialog>
        </>
    );
}
function execSignature(executions = []) {
    try {
        const parts = [];
        for (const exe of (executions || [])) {
            const steps = Array.isArray(exe?.steps) ? exe.steps : [];
            for (const s of steps) {
                const r = String(s?.reason || '');
                const st = String(s?.statusText || '').toLowerCase();
                const eid = s?.elicitation?.elicitationId || s?.elicitationId || '';
                const pid = s?.elicitationPayloadId || '';
                const id = s?.id || '';
                parts.push(`${r}:${st}:${eid}:${pid}:${id}`);
            }
        }
        return parts.join('|');
    } catch(_) {
        return '';
    }
}
export default React.memo(ExecutionDetails, (a, b) => {
    if ((a.messageId || '') !== (b.messageId || '')) return false;
    if ((a.turnStatus || '') !== (b.turnStatus || '')) return false;
    if ((a.turnError || '') !== (b.turnError || '')) return false;
    return execSignature(a.executions) === execSignature(b.executions);
});

// Map a DAO model call view (optionally enriched with request/response) to table row
function mapModelCall(row = {}) {
    const call = row.call || row; // support enriched shape {call, request, response}
    const started = call.startedAt ? new Date(call.startedAt) : null;
    const completed = call.completedAt ? new Date(call.completedAt) : null;
    const elapsed = (started && completed) ? ((completed - started) / 1000).toFixed(2) + 's' : '';
    const finish = call.finishReason || '';
    const success = finish ? 'success' : 'pending';
    return {
        state: completed ? '✔︎' : '⏳',
        name: `${call.provider || ''}/${call.model || ''}`,
        reason: finish,
        success,
        elapsed,
        request: row.request,
        response: row.response,
        requestPayloadId: call.requestPayloadId,
        responsePayloadId: call.responsePayloadId,
    };
}

// Map a DAO tool call view (optionally enriched with request/response) to table row
function mapToolCall(row = {}) {
    const call = row.call || row;
    const started = call.startedAt ? new Date(call.startedAt) : null;
    const completed = call.completedAt ? new Date(call.completedAt) : null;
    const elapsed = (started && completed) ? ((completed - started) / 1000).toFixed(2) + 's' : '';
    const status = call.status || '';
    const beginDrag = (kind, e) => {
        try {
            const startX = e.clientX;
            const startY = e.clientY;
            const start = kind === 'payload' ? { ...payloadPos } : { ...dlgPos };
            const onMove = (evt) => {
                const dx = evt.clientX - startX;
                const dy = evt.clientY - startY;
                const next = { left: Math.max(0, start.left + dx), top: Math.max(0, start.top + dy) };
                if (kind === 'payload') setPayloadPos(next); else setDlgPos(next);
            };
            const onUp = () => {
                window.removeEventListener('mousemove', onMove);
                window.removeEventListener('mouseup', onUp);
            };
            window.addEventListener('mousemove', onMove);
            window.addEventListener('mouseup', onUp);
            e.preventDefault();
        } catch(_) {}
    };

    const success = status === 'completed' ? 'success' : (status ? 'error' : 'pending');
    return {
        state: completed ? '✔︎' : '⏳',
        name: call.name || '',
        reason: call.errorMessage || '',
        success,
        elapsed,
        request: row.request,
        response: row.response,
        requestPayloadId: call.requestPayloadId,
        responsePayloadId: call.responsePayloadId,
    };
}
