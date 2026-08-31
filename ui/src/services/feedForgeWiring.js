/**
 * Feed Forge wiring utilities — ported from original agently ToolFeed.jsx.
 * Pure functions for resolving feed data sources and wiring Forge signals.
 */
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal, getFormStatusSignal } from 'forge/core';

export function selectPath(selector, root) {
  if (!selector) return root;
  if (selector === '$') return root;
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

function projectDateParts(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return value ?? '';
  const year = Number(value.year);
  const month = Number(value.month ?? (Number(value.monthIndex) + 1));
  const day = Number(value.day);
  if (![year, month, day].every(Number.isFinite)) return '';
  return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

export function projectFeedFields(root, fields = {}) {
  if (!fields || typeof fields !== 'object' || Array.isArray(fields)) return root;
  const projected = {};
  for (const [name, raw] of Object.entries(fields)) {
    const config = typeof raw === 'string' ? { path: raw } : (raw || {});
    const transform = String(config.transform || '').trim().toLowerCase();
    let value = selectPath(config.path || config.selector || name, root);
    if (transform === 'daterange' || transform === 'daterangelabel') {
      const start = projectDateParts(selectPath(config.startPath || 'start', root));
      const end = projectDateParts(selectPath(config.endPath || 'end', root));
      value = {
        start,
        end,
      };
      if (transform === 'daterangelabel') value = [start, end].filter(Boolean).join(' – ');
    } else if (transform === 'dateparts') {
      value = projectDateParts(value);
    } else if (transform === 'boolean') {
      value = value === true || value === 1 || value === '1' || String(value || '').toLowerCase() === 'true';
    }
    projected[name] = value;
  }
  return projected;
}

export function flattenFeedRows(parentRows = [], config = {}) {
  const sources = Array.isArray(config?.sources) ? config.sources : [];
  const output = [];
  for (const parent of asArray(parentRows)) {
    for (const source of sources) {
      const children = asArray(selectPath(source?.path || '', parent));
      for (const child of children) {
        if (child == null) continue;
        const exclude = source?.exclude && typeof source.exclude === 'object' ? source.exclude : null;
        if (exclude && selectPath(exclude.field || '', child) === exclude.equals) continue;
        let row = source?.fields ? projectFeedFields(child, source.fields) : (
          child && typeof child === 'object' && !Array.isArray(child) ? { ...child } : { value: child }
        );
        for (const [field, path] of Object.entries(source?.parentFields || {})) {
          row[field] = selectPath(path, parent);
        }
        for (const [field, value] of Object.entries(source?.values || {})) {
          row[field] = value;
        }
        output.push(row);
      }
    }
  }
  return output;
}

export function filterFeedRows(rows = [], config = {}) {
  const excludes = Array.isArray(config?.exclude) ? config.exclude : (config?.exclude ? [config.exclude] : []);
  if (excludes.length === 0) return asArray(rows);
  return asArray(rows).filter((row) => !excludes.some((rule) => {
    if (!rule || typeof rule !== 'object') return false;
    const actual = selectPath(rule.field || rule.path || '', row);
    if (rule.equalsIgnoreCase != null) {
      return String(actual ?? '').trim().toLowerCase() === String(rule.equalsIgnoreCase).trim().toLowerCase();
    }
    return actual === rule.equals;
  }));
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
      const selected = selectPath(sel, parentRoot);
      const projectedRows = ds?.flatten
        ? flattenFeedRows(selected, ds.flatten)
        : asArray(ds?.fields ? projectFeedFields(selected, ds.fields) : selected);
      const rows = filterFeedRows(projectedRows, ds);
      computed[name] = ds?.aggregate?.countAs
        ? [{ [String(ds.aggregate.countAs)]: rows.length }]
        : rows;
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
    const definition = exe?.dataSources?.[name] || {};
    const selectionMode = String(definition?.selectionMode || '').trim().toLowerCase();
    const uniqueFields = (Array.isArray(definition?.uniqueKey) ? definition.uniqueKey : [])
      .map((entry) => String(entry?.field || entry || '').trim())
      .filter(Boolean);
    const rowKey = (row = null) => {
      if (!row || uniqueFields.length === 0) return '';
      return uniqueFields.map((field) => JSON.stringify(row?.[field] ?? null)).join('|');
    };
    if (selectionMode === 'multi') {
      const selectionSignal = getSelectionSignal(dsId, { selection: [] });
      const previous = selectionSignal.peek?.() || selectionSignal.value || { selection: [] };
      const previousRows = Array.isArray(previous?.selection) ? previous.selection : [];
      let nextSelection = [];
      if (previous?._initialized) {
        const selectedKeys = new Set(previousRows.map(rowKey).filter(Boolean));
        nextSelection = uniqueFields.length > 0
          ? data.filter((row) => selectedKeys.has(rowKey(row)))
          : previousRows.filter((row) => data.includes(row));
      } else {
        const selectionConfig = definition?.selection && typeof definition.selection === 'object' ? definition.selection : {};
        const field = String(selectionConfig?.field || 'selected').trim() || 'selected';
        nextSelection = String(selectionConfig?.initial || '').trim().toLowerCase() === 'all'
          ? [...data]
          : data.filter((row) => !!row?.[field]);
      }
      selectionSignal.value = { selection: nextSelection, _initialized: true };
    }
    const sig = getCollectionSignal(dsId);
    sig.value = Array.isArray(data) ? data : asArray(data);
    try { getFormStatusSignal(dsId).value = { dirty: false }; } catch (_) {}
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
  // Seed root selection for single-selection feeds only.
  const rootName = String(exe?.dataFeed?.name || '').trim();
  if (
    rootName
    && String(exe?.dataSources?.[rootName]?.selectionMode || '').trim().toLowerCase() !== 'multi'
    && Array.isArray(computed[rootName])
    && computed[rootName].length > 0
  ) {
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
