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
    const uiWithDS = ui && typeof ui === 'object'
      ? { ...ui, dataSources: dsNormalized }
      : { dataSources: dsNormalized, ...(ui || {}) };
    const title = ui?.title || exe?.title || feedId;
    return { key: feedId, title, ui: uiWithDS, exe };
  }), [feeds]);

  if (!tabs.length) return null;

  return (
    <div style={{ border: '1px solid var(--light-gray1)', borderRadius: 4, padding: 8, marginTop: 6 }}>
      <Tabs id={`toolfeed-${turnId}`} renderActiveTabPanelOnly>
        {tabs.map((t, tabIdx) => {
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
