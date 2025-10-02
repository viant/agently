// ExecutionDetails.jsx – moved from Forge to Agently for domain-specific execution UI

import React, { useMemo, useEffect } from "react";
import {BasicTable as Basic} from "forge/components";
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
    { id: "icon",    name: "",      width: 28, align: "center", minWidth: "28px", enforceColumnSize: false },
    { id: "kind",    name: "Kind",   width: 40, align: "center", minWidth: "68px" },
    { id: "name",    name: "Name",   flex: 2 },
    { id: "status",  name: "Status", width: 60 },
    { id: "elapsed", name: "Time",   width: 60 },
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
    const allowed = new Set([ 'thinking', 'tool_call', 'elicitation' ]);
                return executions.flatMap(exe => (exe.steps || [])
        .filter(s => allowed.has(String(s?.reason || '').toLowerCase()))
        .map(s => {
            const reason = String(s?.reason || '').toLowerCase();
            const hasBool = typeof s.successBool === 'boolean';
            const successBool = hasBool ? s.successBool : (typeof s.success === 'boolean' ? s.success : undefined);
            const statusText = (s.statusText || (successBool === undefined ? 'pending' : (successBool ? 'completed' : 'error'))).toLowerCase();
            const hasError = !!(s.error || s.errorCode) || statusText === 'failed' || statusText === 'error';
            // Status icon: hourglass if not completed, exclamation if error, ok otherwise
            const icon = hasError ? '❗' : (statusText === 'completed' || statusText === 'accepted' ? '✅' : '⏳');
            // Kind glyph: brain for model (thinking), tool for tool_call, keyboard for elicitation
            const kindGlyph = reason === 'thinking' ? '🧠' : (reason === 'tool_call' ? '🛠️' : '⌨️');
            const annotatedName = reason === 'elicitation' && s.originRole ? `${s.name} (${s.originRole})` : (s.name || reason);
            return {
                icon,
                kind: kindGlyph,
                name: annotatedName,
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
}

export default function ExecutionDetails({ executions = [], context, messageId, onError, useForgeDialog = false, resizable = false, useCodeMirror = false }) {
    const [dialog, setDialog] = React.useState(null); // Details or generic payload viewer
    const [payloadDialog, setPayloadDialog] = React.useState(null); // Secondary dialog for payloads when details is open
    const [dlgSize, setDlgSize] = React.useState({ width: 960, height: 640 });
    const [dlgPos, setDlgPos] = React.useState({ left: 120, top: 80 });
    const [payloadPos, setPayloadPos] = React.useState({ left: 160, top: 120 });
    const dataSourceId = `ds${messageId ?? ""}`;
    const rows = useMemo(() => flattenExecutions(executions), [executions]);

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
            const resp = await fetch(url, { credentials: 'same-origin' });
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
                if (id === 'exec.openDetail') return ({ row }) => setDialog({ title: 'Details', kind: `detail-${row._reason}`, row });
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
                    return null;
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
