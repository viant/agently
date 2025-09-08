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
        cellProperties: { text: "view", minimal: true, small: true },
        on: [ { event: "onClick", handler: "exec.openRequest" } ],
    },
    {
        id: "response",
        name: "Response",
        width: 80,
        type: "button",
        cellProperties: { text: "view", minimal: true, small: true },
        on: [ { event: "onClick", handler: "exec.openResponse" } ],
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
            case "dataSource.toggleSelection":
                return ({ rowIndex }) => toggleSelection({ rowIndex });
            case "dataSource.isSelected":
                return ({ rowIndex }) => selectionSig.peek().selection?.some((s)=>s.rowIndex===rowIndex);
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
    // executions array now directly contains trace objects (lightweight – no heavy payload).
    return executions.flatMap(exe => (exe.steps || []).map(s => ({
        traceId:  s.traceId,
        state:    s.endedAt || s.success !== undefined ? (s.success ? "✔︎" : "✖︎") : "⏳",
        name:     s.name,
        reason:   s.reason || s.error || '',
        success:  s.success === undefined ? "pending" : (s.success ? "success" : "error"),
        elapsed:  s.elapsed,
        // include request/response refs so the viewer can lazy-load payloads
        request:  s.request,
        response: s.response,
        // pass through payload IDs from step exactly as presented
        requestPayloadId: s.requestPayloadId,
        responsePayloadId: s.responsePayloadId,
    })));
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

    // v2 operations fallback: when no legacy executions are present, load grouped operations
    useEffect(() => {
        const fetchOps = async () => {
            if (!messageId) return;
            try {
                const base = endpoints?.agentlyAPI?.baseURL || '';
                const url = `${(base || '').replace(/\/+$/,'')}/v2/api/agently/messages/${encodeURIComponent(messageId)}/operations?includePayloads=1`;
                const resp = await fetch(url);
                if (!resp.ok) return;
                const json = await resp.json();
                const data = json?.data || {};
                const toolCalls = Array.isArray(data.toolCalls) ? data.toolCalls : [];
                const modelCalls = Array.isArray(data.modelCalls) ? data.modelCalls : [];
                const mapped = [
                    ...modelCalls.map(mc => mapModelCall(mc)),
                    ...toolCalls.map(tc => mapToolCall(tc)),
                ];
                const sig = getCollectionSignal(dataSourceId);
                const current = typeof sig.peek === 'function' ? sig.peek() : sig.value;
                const hasExisting = Array.isArray(current) && current.length > 0;
                if (!hasExisting && mapped.length) {
                    sig.value = mapped;
                }
            } catch (e) {
                if (typeof onError === 'function') {
                    onError(e);
                }
            }
        };
        fetchOps();
    }, [messageId, dataSourceId, onError]);

    const viewPart = async (part, row) => {
        try {
            const title = part === 'request' ? 'Request' : 'Response';
            if (!useForgeDialog) {
                setDialog({ title, payload: null, loading: true });
            }
            // Use only the provided payload ID fields
            const pid = part === 'request'
                ? row.requestPayloadId
                : row.responsePayloadId;
            let url;
            if (!pid) { setDialog({ title, payload: '(no payload)' }); return; }
            const base = endpoints?.agentlyAPI?.baseURL || '';
            url = `${(base || '').replace(/\/+$/,'')}/v2/api/agently/payload/${encodeURIComponent(pid)}?raw=1`;

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
            const resp = await fetch(url);
            if (!resp.ok) throw new Error(`${resp.status}`);
            const ct = resp.headers.get('content-type') || '';
            const text = await resp.text();
            let payload = text;
            if (ct.includes('application/json')) {
                try { payload = JSON.parse(text); } catch (_) {}
            }
            setDialog({ title, payload, loading: false });
        } catch (err) {
            if (typeof onError === 'function') {
                try { onError(err); } catch (_) { /* ignore */ }
            }
            setDialog({ title: 'Error', payload: String(err) });
        }
    };

    const execContext = useMemo(
        () => buildExecutionContext(context, dataSourceId, (title, payload) => setDialog({ title, payload }), viewPart),
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
                            {!dialog.loading && dialog.payload !== null && (
                                <JsonViewer value={dialog.payload} useCodeMirror={useCodeMirror} height={resizable ? 'calc(100% - 8px)' : '60vh'} />
                            )}
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
