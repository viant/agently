/**
 * Feed Forge wiring utilities — ported from original agently ToolFeed.jsx.
 * Pure functions for resolving feed data sources and wiring Forge signals.
 */
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal } from 'forge/core';

export function selectPath(selector, root) {
  if (!selector) return root;
  if (selector === 'output') return (root && typeof root === 'object' && 'output' in root) ? root.output : root;
  if (selector === 'input') return (root && typeof root === 'object' && 'input' in root) ? root.input : root;
  let cur = root;
  const norm = String(selector).replace(/\[(\d+)\]/g, '.$1').replace(/^\./, '');
  if (
    cur
    && typeof cur === 'object'
    && !Array.isArray(cur)
    && !Object.prototype.hasOwnProperty.call(cur, 'output')
    && !Object.prototype.hasOwnProperty.call(cur, 'input')
    && (norm.startsWith('output.') || norm.startsWith('input.'))
  ) {
    return selectPath(norm.replace(/^(output|input)\./, ''), cur);
  }
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

export function asArray(val) {
  if (Array.isArray(val)) return val;
  if (val == null) return [];
  return [val];
}

export function computeDataMap(exe) {
  if (!exe) return {};
  const dsMap = exe.dataSources || {};
  const rootName = String(exe?.dataFeed?.name || '').trim();
  const rootData = exe?.dataFeed?.data;
  const computed = {};

  const names = Object.keys(dsMap);
  for (const name of names) {
    const ds = dsMap[name] || {};
    const source = String(ds?.source || '').trim();
    if (!source) continue;
    computed[name] = asArray(selectPath(source, rootData));
  }
  if (rootName && !computed.hasOwnProperty(rootName)) {
    computed[rootName] = asArray(rootData);
  }

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
      const parentRoot = Array.isArray(parentData) && parentData.length === 1
        ? parentData[0]
        : (Array.isArray(parentData) ? parentData : (parentData || {}));
      computed[name] = asArray(selectPath(sel, parentRoot));
    } else {
      if (!computed.hasOwnProperty(name)) computed[name] = [];
    }
  }
  for (const n of names) resolve(n);
  return computed;
}

export function buildAutoColumns(rows) {
  if (!Array.isArray(rows) || rows.length === 0) return [];
  const first = rows[0];
  if (!first || typeof first !== 'object' || Array.isArray(first)) return [];
  return Object.keys(first).map((key) => ({ id: key, name: key, width: 140 }));
}

export function applyAutoTableColumns(container, dataMap) {
  if (!container || typeof container !== 'object') return container;
  const visit = (node) => {
    if (!node || typeof node !== 'object') return;
    if (node.style && typeof node.style === 'object' && !Array.isArray(node.style)) {
      if (!node.table && (Array.isArray(node.style.columns) || node.style.pagination)) {
        node.table = {};
      }
      if (node.table && (!Array.isArray(node.table.columns) || node.table.columns.length === 0) && Array.isArray(node.style.columns)) {
        node.table.columns = node.style.columns;
        delete node.style.columns;
      }
      if (node.table && node.table.pagination == null && node.style.pagination != null) {
        node.table.pagination = node.style.pagination;
        delete node.style.pagination;
      }
    }
    if (node.table && (!Array.isArray(node.table.columns) || node.table.columns.length === 0)) {
      const dsRef = String(node.dataSourceRef || '').trim();
      const rows = dsRef ? dataMap[dsRef] : [];
      const cols = buildAutoColumns(rows);
      if (cols.length > 0) node.table.columns = cols;
    }
    const children = Array.isArray(node.containers) ? node.containers
      : Array.isArray(node.items) ? node.items : [];
    for (const child of children) visit(child);
  };
  visit(container);
  return container;
}

export function normalizeDataSources(defs = {}) {
  const out = {};
  for (const [name, def] of Object.entries(defs || {})) {
    const d = (def && typeof def === 'object') ? { ...def } : {};
    d.selectors = (d.selectors && typeof d.selectors === 'object') ? { ...d.selectors } : {};
    if (!('data' in d) && !d.dataSourceRef) d.data = [];
    out[name] = d;
  }
  for (const d of Object.values(out)) {
    const ref = d?.dataSourceRef;
    if (typeof ref === 'string' && ref.trim() && !out.hasOwnProperty(ref)) {
      out[ref] = { data: [] };
    }
  }
  return out;
}

/**
 * Wire computed feed data into Forge signals so ForgeContainer can render.
 * Returns the number of data sources wired.
 */
/**
 * Wire computed feed data into Forge signals so ForgeContainer can render.
 * windowId should include conversation ID for isolation: `feed-{feedId}-{convId}`
 */
export function wireFeedSignals(exe, windowId) {
  if (!exe) return 0;
  const computed = computeDataMap(exe);
  const toDsId = (n) => `${windowId}DS${n}`;

  let wired = 0;
  for (const [name, data] of Object.entries(computed)) {
    const dsId = toDsId(name);
    const sig = getCollectionSignal(dsId);
    sig.value = Array.isArray(data) ? data : asArray(data);
    try {
      const ctrl = getControlSignal(dsId);
      if (ctrl?.set) ctrl.set({ ...(ctrl.peek?.() || {}), loading: false });
      else if (ctrl) ctrl.value = { ...(ctrl.value || {}), loading: false };
    } catch (_) {}
    try {
      const arr = Array.isArray(sig?.value) ? sig.value : [];
      if (arr.length === 1) {
        const formSig = getFormSignal(dsId);
        formSig.value = arr[0];
      }
    } catch (_) {}
    wired++;
  }
  // Seed root selection
  const rootName = String(exe?.dataFeed?.name || '').trim();
  if (rootName && Array.isArray(computed[rootName]) && computed[rootName].length > 0) {
    const dsId = toDsId(rootName);
    const selSig = getSelectionSignal(dsId, { selected: null, rowIndex: -1 });
    selSig.value = { selected: computed[rootName][0], rowIndex: 0 };
  }
  return wired;
}

/**
 * Clean up Forge signals for a feed that became inactive.
 */
export function cleanupFeedSignals(feedId, dsNames = [], conversationId = '') {
  const windowId = conversationId ? `feed-${feedId}-${conversationId}` : `feed-${feedId}`;
  for (const name of dsNames) {
    const dsId = `${windowId}DS${name}`;
    try { getCollectionSignal(dsId).value = []; } catch (_) {}
    try { getFormSignal(dsId).value = {}; } catch (_) {}
  }
}
