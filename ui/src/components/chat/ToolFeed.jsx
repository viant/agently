// ToolFeed.jsx – renders per-turn ToolExecution entries in an expandable panel
// with tabs, wiring Forge DataSource signals dynamically per turn.

import React, { useEffect, useMemo, useState } from 'react';
import { Tabs, Tab } from '@blueprintjs/core';
import { getCollectionSignal, getControlSignal, getSelectionSignal } from 'forge/core';
import { BasicTable } from '../../../../../forge/index.js';

function remapContainer(container = {}, turnId = '') {
  try {
    const json = JSON.stringify(container);
    const obj = JSON.parse(json);
    const rewrite = (node) => {
      if (!node || typeof node !== 'object') return;
      if (node.dataSourceRef && typeof node.dataSourceRef === 'string') {
        node.dataSourceRef = `${turnId}-${node.dataSourceRef}`;
      }
      Object.keys(node).forEach((k) => rewrite(node[k]));
    };
    rewrite(obj);
    return obj;
  } catch (_) {
    return container;
  }
}

function titleCase(s = '') {
  return String(s)
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (m) => m.toUpperCase());
}

function deriveColumnsFor(container) {
  try {
    // If container already defines columns at top-level or under table, leave to BasicTable
    if (Array.isArray(container?.columns) && container.columns.length) return null;
    if (Array.isArray(container?.table?.columns) && container.table.columns.length) return null;
    const ref = (container && container.dataSourceRef) ? String(container.dataSourceRef) : '';
    if (!ref) return null;
    const id = ref; // container already remapped; ref includes turnId prefix
    const sig = getCollectionSignal(id);
    const data = Array.isArray(sig?.value) ? sig.value : (typeof sig?.peek === 'function' ? (sig.peek() || []) : []);
    const row = Array.isArray(data) && data.length ? data[0] : null;
    if (!row || typeof row !== 'object') return null;
    const cols = Object.keys(row).slice(0, 10).map((k) => ({ id: k, name: titleCase(k) }));
    return cols.length ? cols : null;
  } catch (_) {
    return null;
  }
}

function useWireDataSources(executions = [], turnId = '') {
  useEffect(() => {
    if (!Array.isArray(executions)) return;
    let wired = 0;
    for (const exe of executions) {
      const data = Array.isArray(exe?.data) ? exe.data : [];
      for (const d of data) {
        const id = `${turnId}-${(d?.name || '').trim()}`;
        if (!id) continue;
        try {
          const sig = getCollectionSignal(id);
          // Set as-is; tables expect arrays. If object, wrap into single-row array for resilience.
          const val = Array.isArray(d?.data) ? d.data : (d?.data ? [d.data] : []);
          sig.value = val;
          wired++;
        } catch (_) {}
      }
    }
    try { console.debug('[toolfeed][signals] wired', { turnId, wired, executions: executions.length }); } catch(_) {}
  }, [executions, turnId]);
}

export default function ToolFeed({ executions = [], turnId = '', context }) {
  const [open, setOpen] = useState(false);
  const safeExecutions = Array.isArray(executions) ? executions : [];

  useWireDataSources(safeExecutions, turnId);

  const tabs = useMemo(() => safeExecutions.map((exe, idx) => {
    const feedId = (exe.ID || exe.id || exe.ruleId || `feed-${idx}`) + '';
    let base = exe.ui || exe.view || {};
    // Fallback: synthesize a simple table container from the first data source
    if (!base || Object.keys(base).length === 0) {
      const first = Array.isArray(exe?.data) && exe.data.length ? exe.data[0] : null;
      if (first && first.name) {
        base = { id: first.name, type: 'table', dataSourceRef: first.name, columns: [] };
      }
    }
    const container = remapContainer(base, turnId);
    const derived = deriveColumnsFor(container) || [];
    try { console.debug('[toolfeed][container]', { turnId, idx, title: exe.title || exe.ruleId, before: base, container, derived }); } catch(_) {}
    // Prefer container/ui-provided title; then root title; then ruleId
    const title = (base && (base.title || base.label)) || exe.title || feedId || `Execution ${idx+1}`;
    // Always pass a columns array to BasicTable to satisfy its initConfiguredColumns
    let colsToPass = derived;
    if (!colsToPass.length && Array.isArray(container?.columns)) colsToPass = container.columns;
    if (!colsToPass.length && Array.isArray(container?.table?.columns)) colsToPass = container.table.columns;
    if (!Array.isArray(colsToPass)) colsToPass = [];
    return { key: feedId.toLowerCase(), title, container, columns: colsToPass };
  }), [safeExecutions, turnId]);

  if (!tabs.length) return null;

  return (
    <div style={{ border: '1px solid var(--light-gray1)', borderRadius: 4, padding: 8, marginTop: 6 }}>
      <Tabs id={`toolfeed-${turnId}`} renderActiveTabPanelOnly>
        {tabs.map(t => {
          const dsRef = String(t?.container?.dataSourceRef || '');
          const dsId  = dsRef; // already remapped to include turnId
          // Build a minimal, signal-backed context for this data source id
          const collSig = getCollectionSignal(dsId);
          const ctrlSig = getControlSignal(dsId);
          const selSig  = getSelectionSignal(dsId, { selection: [] });
          const tfContext = {
            ...context,
            signals: {
              collection: collSig,
              control: ctrlSig,
              selection: selSig,
              message: context?.signals?.message,
              form: context?.signals?.form,
              input: context?.signals?.input,
              collectionInfo: context?.signals?.collectionInfo,
            },
            handlers: {
              ...(context?.handlers || {}),
              dataSource: {
                getCollection: () => collSig.peek(),
                peekCollection: () => collSig.peek(),
                getSelection: () => selSig.peek(),
                setAllSelection: () => {
                  const all = (collSig.peek() || []).map((_, idx) => ({ rowIndex: idx }));
                  selSig.value = { selection: all };
                },
                resetSelection: () => { selSig.value = { selection: [] }; },
                isSelected: ({ rowIndex }) => (selSig.peek().selection || []).some(s => s.rowIndex === rowIndex),
                setSilentFilterValues: () => {},
                peekFilter: () => ({}),
                getFilterSets: () => [],
              },
            },
            tableSettingKey: (id) => `tf-${dsId}-${id}`,
          };
          return (
            <Tab id={t.key} key={t.key} title={t.title} panel={
              <div style={{ paddingTop: 8 }}>
                <BasicTable container={t.container} context={tfContext} columns={t.columns || []} />
              </div>
            } />
          );
        })}
      </Tabs>
    </div>
  );
}
