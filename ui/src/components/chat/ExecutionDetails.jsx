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
    { id: "tool",     name: "Tool",     width: 120 },
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
        tool:     s.tool,
        reason:   s.reason || s.error || '',
        success:  s.success === undefined ? "pending" : (s.success ? "success" : "error"),
        elapsed:  s.elapsed,
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

    // Helper to fetch heavy payload on demand
    const viewPart = async (part, row) => {
        try {
            const convID = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.()?.id;
            if (!convID) return;
            const title = part === 'request' ? 'Request' : 'Response';
            setDialog({ title, payload: null, loading: true });
            const url = `${endpoints.appAPI.baseURL}/conversations/${convID}/execution/${row.traceId}/${part}`;
            const resp = await fetch(url);
            if (!resp.ok) throw new Error(`${resp.status}`);
            const json = await resp.json();
            setDialog({ title, payload: json, loading: false });
        } catch (err) {
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
