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
    { id: "state", name: "", width: 24, align: "center", minWidth: "24px", enforceColumnSize: false },
    { id: "name",     name: "Name",     width: 120 },
    { id: "reason",   name: "Reason",   flex: 2 },
    { id: "success",  name: "Status",  width: 60 },
    { id: "elapsed",  name: "Time",     width: 90 },
    {
        id: "request",
        name: "Request",
        width: 80,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true, disabledExpr: '!row.requestPayloadId' },
        on: [ { event: "onClick", handler: "exec.openRequest" } ],
    },
    {
        id: "providerRequest",
        name: "Prov. Req",
        width: 90,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true, disabledExpr: '!row.providerRequestPayloadId' },
        on: [ { event: "onClick", handler: "exec.openProviderRequest" } ],
    },
    {
        id: "response",
        name: "Response",
        width: 80,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true, disabledExpr: '!row.responsePayloadId' },
        on: [ { event: "onClick", handler: "exec.openResponse" } ],
    },
    {
        id: "providerResponse",
        name: "Prov. Resp",
        width: 90,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true, disabledExpr: '!row.providerResponsePayloadId' },
        on: [ { event: "onClick", handler: "exec.openProviderResponse" } ],
    },
    {
        id: "stream",
        name: "Stream",
        width: 80,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true, disabledExpr: '!row.streamPayloadId' },
        on: [ { event: "onClick", handler: "exec.openStream" } ],
    },
    {
        id: "elicitation",
        name: "Elicitation",
        width: 100,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true, disabledExpr: '!row.elicitationPayloadId' },
        on: [ { event: "onClick", handler: "exec.openElicitation" } ],
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
    // Show model/tool steps and completed elicitations in the table
    const allowed = new Set([ 'thinking', 'tool_call', 'elicitation' ]);
    return executions.flatMap(exe => (exe.steps || [])
        .filter(s => allowed.has(String(s?.reason || '').toLowerCase()))
        .map(s => {
            const hasBool = typeof s.successBool === 'boolean';
            const successBool = hasBool ? s.successBool : (typeof s.success === 'boolean' ? s.success : undefined);
            const statusText = (s.statusText || (successBool === undefined ? 'pending' : (successBool ? 'success' : 'error'))).toLowerCase();
            // Derive icon: accepted ✔︎, rejected ✖︎, cancel ⏸︎, pending ⏳
            const icon = statusText === 'accepted' ? '✔︎' : statusText === 'rejected' ? '✖︎' : statusText === 'cancel' ? '⏸︎' : '⏳';
            // Name annotation for elicitation origin (assistant/tool)
            const isElic = String(s?.reason || '').toLowerCase() === 'elicitation';
            const annotatedName = isElic && s.originRole ? `${s.name} (${s.originRole})` : s.name;
            return {
                traceId:  s.traceId,
                state:    icon,
                name:     annotatedName,
                reason:   '',
                success:  statusText,
                elapsed:  s.elapsed,
                // include request/response refs so the viewer can lazy-load payloads
                request:  s.request,
                response: s.response,
                // pass through payload IDs from step exactly as presented
                requestPayloadId: s.requestPayloadId,
                responsePayloadId: s.responsePayloadId,
                streamPayloadId: s.streamPayloadId,
                providerRequestPayloadId: s.providerRequestPayloadId,
                providerResponsePayloadId: s.providerResponsePayloadId,
            };
        })
    );
}

export default function ExecutionDetails({ executions = [], context, messageId, onError, useForgeDialog = false, resizable = false, useCodeMirror = false }) {
    const [dialog, setDialog] = React.useState(null);
    const [dlgSize, setDlgSize] = React.useState({ width: 960, height: 640 });
    const dataSourceId = `ds${messageId ?? ""}`;
    const rows = useMemo(() => flattenExecutions(executions), [executions]);

    useEffect(() => {
        const sig = getCollectionSignal(dataSourceId);
        sig.value = rows;
    }, [rows, dataSourceId]);


    const viewPart = async (part, row) => {
        try {
            const title = part === 'request' ? 'Request' : part === 'providerRequest' ? 'Provider Request' : part === 'providerResponse' ? 'Provider Response' : (part === 'elicitation' ? 'Elicitation' : (part === 'stream' ? 'Stream' : 'Response'));
            if (!useForgeDialog) {
                setDialog({ title, payload: null, loading: true });
            }
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
                    if (!useForgeDialog) {
                        setDialog({ title, payload: null, loading: true });
                    }
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
            setDialog({ title, payload, loading: false, contentType: ct, kind: part });
        } catch (err) {
            if (typeof onError === 'function') {
                try { onError(err); } catch (_) { /* ignore */ }
            }
            setDialog({ title: 'Error', payload: String(err) });
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
                container={{ id: `exec-${messageId}`, table: { enforceColumnSize: false, fullWidth: true } }}
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
                {dialog && (
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
