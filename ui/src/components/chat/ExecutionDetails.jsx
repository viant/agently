// ExecutionDetails.jsx – moved from Forge to Agently for domain-specific execution UI

import React, { useMemo, useEffect } from "react";
import {BasicTable as Basic} from "forge/components";
import { Dialog } from "@blueprintjs/core";
import { signal } from "@preact/signals-react";

// Global window-level signals (copied unchanged)
import {
    getCollectionSignal,
    getControlSignal,
    getSelectionSignal,
} from "forge/core";

import {BasicTable} from "../../../../../forge/index.js";

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

function buildExecutionContext(parentContext, dataSourceId, openDialog) {
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
                return ({ row }) => openDialog("Request", row.request);
            case "exec.openResponse":
                return ({ row }) => openDialog("Response", row.response);
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
    return executions.flatMap((exe) => (exe.steps || []).map((s) => ({
        state:    s.endedAt || s.success !== undefined ? (s.success ? "✔︎" : "✖︎") : "⏳",
        tool:      s.tool,
        reason:    s.reason,
        success:   s.success ? "success" : "error",
        elapsed:   s.elapsed,
        request:   typeof s.request === 'object' ? JSON.stringify(s.request) : (s.request ?? ''),
        response:  typeof s.response === 'object' ? JSON.stringify(s.response) : (s.response ?? ''),
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

    const execContext = useMemo(
        () => buildExecutionContext(context, dataSourceId, (title, payload) => setDialog({ title, payload })),
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
                        <pre className="text-xs whitespace-pre-wrap break-all">
                            {typeof dialog.payload === "string"
                                ? dialog.payload
                                : JSON.stringify(dialog.payload, null, 2)}
                        </pre>
                    </div>
                )}
            </Dialog>
        </>
    );
}
