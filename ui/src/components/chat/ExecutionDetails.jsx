// ExecutionDetails.jsx – moved from Forge to Agently for domain-specific execution UI

import React, { useMemo, useEffect } from "react";
import {BasicTable as Basic} from "forge/components";
import { Dialog } from "@blueprintjs/core";
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
    })));
}

export default function ExecutionDetails({ executions = [], context, messageId }) {
    const [dialog, setDialog] = React.useState(null);
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
                if (mapped.length) {
                    sig.value = mapped;
                }
            } catch (e) {
                // eslint-disable-next-line no-console
                console.error('ExecutionDetails: fetch ops error', e);
            }
        };
        fetchOps();
    }, [messageId, dataSourceId]);

    // Helper to fetch heavy payload on demand
    const resolvePayloadRef = (val) => {
        if (!val) return null;
        if (typeof val === 'string') return val; // already a URL or path
        if (typeof val === 'object') {
            if (typeof val.$ref === 'string') return val.$ref;
            if (typeof val.url === 'string') return val.url;
            if (typeof val.href === 'string') return val.href;
        }
        return null;
    };

    const joinURL = (base, path) => {
        if (!path) return base || '';
        if (/^https?:\/\//i.test(path)) return path;
        if (path.startsWith('/')) return path;
        const b = (base || '').replace(/\/+$/,'');
        const p = path.replace(/^\/+/, '');
        return b ? `${b}/${p}` : `/${p}`;
    };

    const viewPart = async (part, row) => {
        try {
            const title = part === 'request' ? 'Request' : 'Response';
            setDialog({ title, payload: null, loading: true });
            // v1 lazy link support: `$ref` URL present
            const ref = part === 'request'
                ? resolvePayloadRef(row.request)
                : resolvePayloadRef(row.response);
            let url;
            if (ref) {
                // Build from ref: if it's absolute or starts with '/', use as-is; otherwise join with app API base.
                url = joinURL(endpoints?.appAPI?.baseURL, ref);
            } else {
                // v2 payload id fallback
                const pid = part === 'request' ? (row.requestPayloadID || row.request?.id) : (row.responsePayloadID || row.response?.id);
                if (!pid) { setDialog({ title, payload: '(no payload)' }); return; }
                const base = endpoints?.agentlyAPI?.baseURL || '';
                url = `${(base || '').replace(/\/+$/,'')}/v2/api/agently/payload/${encodeURIComponent(pid)}?raw=1`;
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
            // eslint-disable-next-line no-console
            console.error('fetch payload error', err);
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
                style={{ width: "60vw", minWidth: "60vw", minHeight: "60vh" }}
            >
                {dialog && (
                    <div style={{ padding: 12, maxHeight: "70vh", overflow: "auto" }}>
                        {dialog.loading && <span>Loading …</span>}
                        {!dialog.loading && dialog.payload !== null && (
                            <pre className="text-xs whitespace-pre-wrap break-all">
                                {typeof dialog.payload === "string"
                                    ? dialog.payload
                                    : JSON.stringify(dialog.payload, null, 2)}
                            </pre>
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
        requestPayloadID: call.requestPayloadID,
        responsePayloadID: call.responsePayloadID,
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
        requestPayloadID: call.requestPayloadID || payloadIdFromSnapshot(call.requestSnapshot),
        responsePayloadID: call.responsePayloadID || payloadIdFromSnapshot(call.responseSnapshot),
    };
}

function payloadIdFromSnapshot(snapshot) {
    if (!snapshot || typeof snapshot !== 'string') return '';
    try { const x = JSON.parse(snapshot); return x.payloadId || ''; } catch (_) { return ''; }
}
