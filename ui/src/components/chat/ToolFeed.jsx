// ToolFeed.jsx – renders per-turn tool feeds by wiring provided DataSources and UI (via Forge Container).

import React, { useEffect, useMemo, useState } from 'react';
import { Tabs, Tab } from '@blueprintjs/core';
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal } from 'forge/core';
import { Container as ForgeContainer } from '../../../../../forge/index.js';
import WindowContentDataSourceContainer from '../../../../../forge/src/components/WindowContentDataSourceContainer.jsx';
import ExplorerFeed from './ExplorerFeed.jsx';

function selectPath(selector, root) {
  if (!selector) return root;
  // Backward-compatible: feeds may provide a root object with `input`/`output` keys,
  // while older feeds used the output object as the root directly.
  if (selector === 'output') return (root && typeof root === 'object' && 'output' in root) ? root.output : root;
  if (selector === 'input') return (root && typeof root === 'object' && 'input' in root) ? root.input : root;
  let cur = root;
  const norm = String(selector).replace(/\[(\d+)\]/g, '.$1').replace(/^\./, '');
  const parts = norm.split('.').filter(Boolean);
  for (const token of parts) {
    if (cur == null) return null;
    const idx = String(token).match(/^\d+$/) ? parseInt(token, 10) : null;
    if (Array.isArray(cur)) {
      if (idx == null || idx < 0 || idx >= cur.length) return null;
      cur = cur[idx];
    } else if (typeof cur === 'object') {
      if (!Object.prototype.hasOwnProperty.call(cur, token)) return null;
      cur = cur[token];
    } else {
      return null;
    }
  }
  return cur === undefined ? null : cur;
}

function asArray(val) { if (Array.isArray(val)) return val; if (val == null) return []; return [val]; }

function computeDataMap(exe) {
  if (!exe) return {};
  const dsMap = exe.dataSources || {};
  const rootName = String(exe?.dataFeed?.name || '').trim();
  const rootData = exe?.dataFeed?.data;
  const computed = {};
  if (rootName) computed[rootName] = asArray(rootData);

  const names = Object.keys(dsMap);
  const visiting = new Set();
  function resolve(name) {
    if (computed.hasOwnProperty(name)) return;
    const ds = dsMap[name] || {};
    const parent = String(ds?.dataSourceRef || '').trim();
    const sel = ds?.selectors?.data || 'output';
    if (parent) {
      if (!computed.hasOwnProperty(parent)) {
        if (visiting.has(name)) return;
        visiting.add(name);
        resolve(parent);
        visiting.delete(name);
      }
      const parentData = computed[parent];
      const parentRoot = Array.isArray(parentData) && parentData.length === 1 ? parentData[0] : (Array.isArray(parentData) ? parentData : (parentData || {}));
      computed[name] = asArray(selectPath(sel, parentRoot));
    } else {
      if (!computed.hasOwnProperty(name)) computed[name] = [];
    }
  }
  for (const n of names) resolve(n);
  return computed;
}

function buildAutoColumns(rows) {
  if (!Array.isArray(rows) || rows.length === 0) return [];
  const first = rows[0];
  if (!first || typeof first !== 'object' || Array.isArray(first)) return [];
  return Object.keys(first).map((key) => ({
    id: key,
    name: key,
    width: 140,
  }));
}

function applyAutoTableColumns(container, dataMap) {
  if (!container || typeof container !== 'object') return container;
  const visit = (node) => {
    if (!node || typeof node !== 'object') return;
    if (node.table && (!Array.isArray(node.table.columns) || node.table.columns.length === 0)) {
      const dsRef = String(node.dataSourceRef || '').trim();
      const rows = dsRef ? dataMap[dsRef] : [];
      const cols = buildAutoColumns(rows);
      if (cols.length > 0) {
        node.table.columns = cols;
      }
    }
    const children = Array.isArray(node.containers) ? node.containers : Array.isArray(node.items) ? node.items : [];
    for (const child of children) visit(child);
  };
  visit(container);
  return container;
}

function wireFeedSignals(exe, windowContext) {
  if (!exe) return 0;
  const dsMap = exe.dataSources || {};
  const rootName = String(exe?.dataFeed?.name || '').trim();
  const rootData = exe?.dataFeed?.data;
  const computed = {};
  if (rootName) computed[rootName] = asArray(rootData);

  const names = Object.keys(dsMap);
  const visiting = new Set();
  function resolve(name) {
    if (computed.hasOwnProperty(name)) return;
    const ds = dsMap[name] || {};
    const parent = String(ds?.dataSourceRef || '').trim();
    const sel = ds?.selectors?.data || 'output';
    if (parent) {
      if (!computed.hasOwnProperty(parent)) {
        if (visiting.has(name)) return;
        visiting.add(name);
        resolve(parent);
        visiting.delete(name);
      }
      const parentData = computed[parent];
      const parentRoot = Array.isArray(parentData) && parentData.length === 1 ? parentData[0] : (Array.isArray(parentData) ? parentData : (parentData || {}));
      computed[name] = asArray(selectPath(sel, parentRoot));
    } else {
      if (!computed.hasOwnProperty(name)) computed[name] = [];
    }
  }
  for (const n of names) resolve(n);

  let wired = 0;
  // Helper to map DS name -> forge DS id within this window
  const toDsId = (n) => {
    try { if (windowContext?.identity?.getDataSourceId) return windowContext.identity.getDataSourceId(n); } catch(_) {}
    try { if (windowContext?.identity?.windowId) return `${windowContext.identity.windowId}DS${n}`; } catch(_) {}
    return n;
  };
  for (const [name, data] of Object.entries(computed)) {
    const dsId = toDsId(name);
    const sig = getCollectionSignal(dsId);
    sig.value = Array.isArray(data) ? data : asArray(data);
    try { const ctrl = getControlSignal(dsId); if (ctrl?.set) ctrl.set({ ...(ctrl.peek?.() || {}), loading: false }); else if (ctrl) ctrl.value = { ...(ctrl.value || {}), loading: false }; } catch(_) {}
    // If the DS has exactly one element, publish it to form signal as the current form/model
    try {
      const arr = Array.isArray(sig?.value) ? sig.value : [];
      if (arr.length === 1) {
        const formSig = getFormSignal(dsId);
        formSig.value = arr[0];
      }
    } catch(_) {}
    wired++;
  }
  // Also seed root selection so dependent DS (via dataSourceRef) can resolve from selected
  if (rootName && Array.isArray(computed[rootName]) && computed[rootName].length > 0) {
    const dsId = toDsId(rootName);
    const selSig = getSelectionSignal(dsId, { selected: null, rowIndex: -1 });
    selSig.value = { selected: computed[rootName][0], rowIndex: 0 };
  }
  return wired;
}

export default function ToolFeed({ executions = [], turnId = '', context }) {
  const feeds = Array.isArray(executions) ? executions : [];
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let total = 0;
    for (const exe of feeds) total += wireFeedSignals(exe, context);
    // Defer container render until after wiring completes
    setReady(true);
  }, [feeds]);

  const tabs = useMemo(() => feeds.map((exe, idx) => {
    const feedId = String(exe?.id || exe?.ID || `feed-${idx}`);
    const ui = exe?.ui || {};
    // Ensure Forge receives dataSources definitions alongside the UI
    const rawDS = ui && typeof ui === 'object'
      ? (ui.dataSources || exe?.dataSources || {})
      : (exe?.dataSources || {});

    // Normalize DS map so Forge never sees undefined entries and optional fields.
    const normalizeDataSources = (defs = {}) => {
      const out = {};
      // First pass: clone and ensure structure
      for (const [name, def] of Object.entries(defs || {})) {
        const d = (def && typeof def === 'object') ? { ...def } : {};
        d.selectors = (d.selectors && typeof d.selectors === 'object') ? { ...d.selectors } : {};
        if (!('data' in d) && !d.dataSourceRef) {
          d.data = [];
        }
        out[name] = d;
      }
      // Second pass: ensure referenced parents exist
      for (const d of Object.values(out)) {
        const ref = d?.dataSourceRef;
        if (typeof ref === 'string' && ref.trim() && !out.hasOwnProperty(ref)) {
          out[ref] = { data: [] };
        }
      }
      return out;
    };

    const dsNormalized = normalizeDataSources(rawDS);
    const uiBase = (ui && typeof ui === 'object') ? JSON.parse(JSON.stringify(ui)) : (ui || {});
    const uiWithDS = uiBase && typeof uiBase === 'object'
      ? { ...uiBase, dataSources: dsNormalized }
      : { dataSources: dsNormalized, ...(uiBase || {}) };
    const dataMap = computeDataMap(exe);
    applyAutoTableColumns(uiWithDS, dataMap);
    const title = ui?.title || exe?.title || feedId;
    return { key: feedId, title, ui: uiWithDS, exe };
  }), [feeds]);

  const titledTabs = useMemo(() => {
    const counts = new Map();
    for (const t of tabs) counts.set(t.title, (counts.get(t.title) || 0) + 1);
    const seen = new Map();
    return tabs.map((t) => {
      const total = counts.get(t.title) || 0;
      if (total <= 1) return t;
      const next = (seen.get(t.title) || 0) + 1;
      seen.set(t.title, next);
      return { ...t, title: `${t.title} ${next}` };
    });
  }, [tabs]);

  if (!titledTabs.length) return null;

  return (
    <div style={{ border: '1px solid var(--light-gray1)', borderRadius: 4, padding: 8, marginTop: 6 }}>
      <Tabs id={`toolfeed-${turnId}`} renderActiveTabPanelOnly>
        {titledTabs.map((t, tabIdx) => {
          const dsDefs = (t?.ui?.dataSources || {});
          // Mutate the parent window metadata so Forge's Context can resolve DS immediately
          try {
            if (context?.metadata && context.metadata.dataSource) {
              for (const [k, v] of Object.entries(dsDefs)) {
                if (!context.metadata.dataSource[k]) {
                  context.metadata.dataSource[k] = v;
                }
              }
            }
          } catch (_) {}
          const safeHandlers = {
            ...(context?.handlers || {}),
            on: (context?.handlers && context.handlers.on) ? context.handlers.on : (() => () => {}),
            emit: (context?.handlers && context.handlers.emit) ? context.handlers.emit : (() => {}),
          };
          const tfContext = {
            ...context,
            handlers: safeHandlers,
            tableSettingKey: (id) => `tf-${t.key}-${id}`,
          };
          return (
            <Tab id={t.key} key={t.key} title={t.title} panel={
              <div style={{ paddingTop: 8 }}>
                {!ready && (
                  <div style={{ padding: 8, color: 'var(--gray2)' }}>Initializing data sources…</div>
                )}
                {ready && (
                  (t.key === 'explorer'
                    ? <ExplorerFeed data={t.exe?.dataFeed?.data} context={tfContext} />
                    : <ForgeContainer container={t.ui} context={tfContext} />
                  )
                )}
              </div>
            } />
          );
        })}
      </Tabs>
    </div>
  );
}
