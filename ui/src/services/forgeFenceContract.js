export const FORGE_UI_FENCE = 'forge-ui';
export const FORGE_DATA_FENCE = 'forge-data';
export const FORGE_REPORT_FENCE = 'forge-report';
export const DEFAULT_FORGE_REPORT_GRAMMAR = 'dashboard-v1';
const FORGE_REPORT_LIMITS = Object.freeze({ fragments: 64, blocks: 100, dataSources: 32, rows: 10000, reportBytes: 5 * 1024 * 1024, messageBytes: 10 * 1024 * 1024, depth: 8 });
const DASHBOARD_REPORT_KINDS = new Set([
  'dashboard.summary', 'dashboard.kpiTable', 'dashboard.compare', 'dashboard.timeline',
  'dashboard.composition', 'dashboard.dimensions', 'dashboard.geoMap', 'dashboard.status',
  'dashboard.filters', 'dashboard.feed', 'dashboard.table', 'dashboard.report',
  'dashboard.detail', 'dashboard.messages', 'dashboard.badges',
]);
const CANONICAL_REPORT_KINDS = new Set([
  'markdownBlock', 'filterBarBlock', 'refinementBarBlock', 'kpiBlock', 'badgesBlock',
  'chartBlock', 'tableBlock', 'geoMapBlock', 'sectionBlock', 'tabGroupBlock',
  'compositeBlock', 'stepperBlock', 'infoPanelBlock', 'calloutBlock', 'kanbanBlock',
  'timelineBlock', 'collectionBlock',
]);
const REPORT_ENVELOPE_FIELDS = new Set([
  'version', 'scope', 'id', 'sequence', 'mode', 'grammar', 'target',
  'title', 'subtitle', 'description', 'theme', 'blocks', 'layout',
  'removeBlockIds', 'fallback', 'metadata', 'datasets', 'dataSources',
]);
const DATA_ENVELOPE_FIELDS = new Set([
  'version', 'scope', 'id', 'reportRef', 'sequence', 'format', 'mode', 'data',
]);

export function isPlainObject(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

export function parseForgeFenceBody(text = '') {
  const body = String(text || '').trim();
  if (!body) {
    throw new Error('Empty fence body');
  }
  const parsed = JSON.parse(body);
  if (!isPlainObject(parsed)) {
    throw new Error('Fence body must be a JSON object');
  }
  return parsed;
}

// validateForgeDataBlock normalizes a forge-data payload. Missing `version`
// defaults to 1 and missing `mode` defaults to "replace" for backward
// compatibility. `id` is still required because it's the join key. An unknown
// format or mode is coerced to a sensible default rather than throwing.
export function validateForgeDataBlock(block = {}) {
  if (!isPlainObject(block)) throw new Error('forge-data block must be an object');
  if (String(block.id || '').trim() === '') throw new Error('forge-data.id is required');
  const version = String(block.version || '').trim() === '' ? 1 : block.version;
  let format = String(block.format || '').trim().toLowerCase();
  if (!['json', 'csv'].includes(format)) {
    // Infer from `data` shape when format is missing/unknown.
    format = typeof block.data === 'string' ? 'csv' : 'json';
  }
  let mode = String(block.mode || 'replace').trim().toLowerCase();
  if (!['replace', 'append', 'patch'].includes(mode)) {
    mode = 'replace';
  }
  return {
    ...block,
    version,
    format,
    mode,
    id: String(block.id).trim(),
  };
}

export function parseCsv(text = '') {
  const source = String(text || '').trim();
  if (!source) return [];
  const lines = source.split(/\r?\n/).filter(Boolean);
  if (!lines.length) return [];
  const headers = splitCsvLine(lines[0]);
  return lines.slice(1).map((line) => {
    const cells = splitCsvLine(line);
    const row = {};
    headers.forEach((header, index) => {
      row[header] = autoValue(cells[index] ?? '');
    });
    return row;
  });
}

function splitCsvLine(line = '') {
  const cells = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < line.length; i += 1) {
    const char = line[i];
    const next = line[i + 1];
    if (char === '"' && inQuotes && next === '"') {
      current += '"';
      i += 1;
      continue;
    }
    if (char === '"') {
      inQuotes = !inQuotes;
      continue;
    }
    if (char === ',' && !inQuotes) {
      cells.push(current);
      current = '';
      continue;
    }
    current += char;
  }
  cells.push(current);
  return cells.map((cell) => cell.trim());
}

function autoValue(value = '') {
  const text = String(value || '').trim();
  if (text === '') return '';
  if (text.toLowerCase() === 'true') return true;
  if (text.toLowerCase() === 'false') return false;
  if (/^-?\d+$/.test(text)) return Number(text);
  if (/^-?\d+\.\d+$/.test(text)) return Number(text);
  return text;
}

export function materializeForgeData(block = {}) {
  const normalized = validateForgeDataBlock(block);
  if (normalized.format === 'csv') {
    return {
      ...normalized,
      rows: parseCsv(normalized.data),
    };
  }
  return {
    ...normalized,
    rows: Array.isArray(normalized.data)
      ? normalized.data
      : isPlainObject(normalized.data)
        ? normalized.data
        : [],
  };
}

function escapeCsvCell(value = '') {
  const text = String(value ?? '');
  if (/[",\n]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`;
  }
  return text;
}

export function rowsToCsv(rows = [], columns = []) {
  const normalizedColumns = (Array.isArray(columns) ? columns : [])
    .map((column) => isPlainObject(column) ? { key: String(column.key || '').trim(), label: String(column.label || column.key || '').trim() } : { key: String(column || '').trim(), label: String(column || '').trim() })
    .filter((column) => column.key);
  if (!normalizedColumns.length) return '';
  const lines = [
    normalizedColumns.map((column) => escapeCsvCell(column.label)).join(','),
    ...(Array.isArray(rows) ? rows : []).map((row) =>
      normalizedColumns.map((column) => escapeCsvCell(row?.[column.key] ?? '')).join(',')
    ),
  ];
  return lines.join('\n');
}

export function applyForgeDataBlocks(blocks = []) {
  const store = {};
  for (const block of Array.isArray(blocks) ? blocks : []) {
    const normalized = materializeForgeData(block);
    const existing = store[normalized.id];
    switch (normalized.mode) {
      case 'replace':
        store[normalized.id] = normalized;
        break;
      case 'append':
        if (!existing) {
          store[normalized.id] = normalized;
          break;
        }
        if (Array.isArray(existing.rows) && Array.isArray(normalized.rows)) {
          store[normalized.id] = { ...normalized, rows: [...existing.rows, ...normalized.rows] };
        } else {
          throw new Error(`append only supported for row-oriented data sources: ${normalized.id}`);
        }
        break;
      case 'patch':
        if (!existing) {
          store[normalized.id] = normalized;
          break;
        }
        if (isPlainObject(existing.rows) && isPlainObject(normalized.rows)) {
          store[normalized.id] = { ...normalized, rows: { ...existing.rows, ...normalized.rows } };
        } else {
          throw new Error(`patch only supported for object data sources: ${normalized.id}`);
        }
        break;
      default:
        throw new Error(`Unsupported forge-data mode: ${normalized.mode}`);
    }
  }
  return store;
}

function cloneJSON(value) {
  return value == null ? value : JSON.parse(JSON.stringify(value));
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`;
  if (isPlainObject(value)) {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${canonicalJSON(value[key])}`).join(',')}}`;
  }
  return JSON.stringify(value);
}

function normalizeReportSegment(value = '', fallback = '') {
  return String(value || '').trim().replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || fallback;
}

function isSafeReportSegment(value = '') {
  const text = String(value || '').trim();
  return !!text && normalizeReportSegment(text) === text;
}

function reportSource(payload = {}) {
  const source = {};
  Object.entries(payload).forEach(([key, value]) => {
    if (!['version', 'scope', 'id', 'sequence', 'mode', 'grammar', 'target', 'removeBlockIds'].includes(key)) {
      source[key] = cloneJSON(value);
    }
  });
  if (!Array.isArray(source.blocks)) source.blocks = [];
  return source;
}

function mergeReportJSON(target, patch) {
  Object.entries(patch || {}).forEach(([key, value]) => {
    if (value === null) {
      delete target[key];
    } else if (isPlainObject(value) && isPlainObject(target[key])) {
      mergeReportJSON(target[key], value);
    } else {
      target[key] = cloneJSON(value);
    }
  });
  return target;
}

function walkReportBlocks(value, visitor) {
  if (!Array.isArray(value)) return null;
  for (const entry of value) {
    if (!isPlainObject(entry)) continue;
    const found = visitor(entry);
    if (found) return found;
  }
  return null;
}

function findReportBlock(blocks, id) {
  const target = String(id || '').trim();
  return walkReportBlocks(blocks, (block) => String(block.id || '').trim() === target ? block : null);
}

function validateStableReportBlockIds(blocks = []) {
  const ids = new Set();
  let error = '';
  walkReportBlocks(blocks, (block) => {
    if (!block.kind) return null;
    const id = String(block.id || '').trim();
    if (!id) error = 'Every progressive block requires a stable id.';
    else if (!isSafeReportSegment(id)) error = `Block id "${id}" must use letters, numbers, dots, underscores, or hyphens.`;
    else if (ids.has(id)) error = `Duplicate block id "${id}".`;
    ids.add(id);
    return null;
  });
  if (error) throw new Error(error);
}

function reportJSONDepth(value, depth = 0) {
  if (Array.isArray(value)) return value.reduce((max, item) => Math.max(max, reportJSONDepth(item, depth + 1)), depth);
  if (isPlainObject(value)) return Object.values(value).reduce((max, item) => Math.max(max, reportJSONDepth(item, depth + 1)), depth);
  return depth;
}

function forbiddenReportSourcePath(value, path = '$') {
  const forbidden = new Set([
    'ownerid', 'userid', 'authheader', 'authorization', 'authtoken', 'accesstoken',
    'dsn', 'dbdsn', 'secret', 'secrets', 'secretref', 'password', 'credentials',
  ]);
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index += 1) {
      const found = forbiddenReportSourcePath(value[index], `${path}[${index}]`);
      if (found) return found;
    }
    return '';
  }
  if (!isPlainObject(value)) return '';
  for (const [key, child] of Object.entries(value)) {
    const childPath = `${path}.${key}`;
    if (forbidden.has(String(key).trim().toLowerCase().replaceAll('_', '').replaceAll('-', ''))) return childPath;
    const found = forbiddenReportSourcePath(child, childPath);
    if (found) return found;
  }
  return '';
}

function csvRecordCount(value = '') {
  const text = String(value || '');
  if (!text.trim()) return 0;
  let records = 1;
  let inQuotes = false;
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (char === '"') {
      if (inQuotes && text[index + 1] === '"') index += 1;
      else inQuotes = !inQuotes;
    } else if (char === '\n' && !inQuotes && index < text.length - 1) {
      records += 1;
    }
  }
  if (inQuotes) throw new Error('CSV contains an unterminated quoted field.');
  return records;
}

function collectProgressiveReportBlocks(blocks = [], grammar = '') {
  const result = [];
  const walkContainers = (value) => {
    if (!Array.isArray(value)) return;
    value.forEach((block) => {
      if (!isPlainObject(block)) return;
      result.push(block);
      if (grammar !== 'dashboard-v1') return;
      if (Array.isArray(block.containers)) walkContainers(block.containers);
      const dashboard = isPlainObject(block.dashboard) ? block.dashboard : {};
      Object.values(dashboard).forEach((config) => {
        if (isPlainObject(config) && Array.isArray(config.containers)) walkContainers(config.containers);
      });
    });
  };
  walkContainers(blocks);
  return result;
}

function validateProgressiveReportState(state) {
  const blocks = Array.isArray(state?.source?.blocks) ? state.source.blocks : [];
  const allBlocks = collectProgressiveReportBlocks(blocks, state?.grammar);
  if (allBlocks.length > FORGE_REPORT_LIMITS.blocks) throw new Error(`An inline report may contain at most ${FORGE_REPORT_LIMITS.blocks} blocks.`);
  if (reportJSONDepth(blocks) > FORGE_REPORT_LIMITS.depth) throw new Error(`Inline report nesting may not exceed depth ${FORGE_REPORT_LIMITS.depth}.`);
  const forbiddenPath = forbiddenReportSourcePath(state?.source || {});
  if (forbiddenPath) throw new Error(`Report source must not declare credentials, ownership, or connection secrets at ${forbiddenPath}.`);
  validateStableReportBlockIds(blocks);
  const blockIds = new Set();
  const blockKinds = new Map();
  allBlocks.forEach((block) => {
    const id = String(block?.id || '').trim();
    const kind = String(block?.kind || '').trim();
    if (blockIds.has(id)) throw new Error(`Duplicate block id "${id}".`);
    if (state?.grammar === 'dashboard-v1' && !DASHBOARD_REPORT_KINDS.has(kind)) throw new Error(`Unsupported dashboard block kind "${kind}".`);
    if (state?.grammar === 'report-document-v1' && !CANONICAL_REPORT_KINDS.has(kind)) throw new Error(`Unsupported canonical report block kind "${kind}".`);
    blockIds.add(id);
    blockKinds.set(id, kind);
  });
  (Array.isArray(state?.source?.layout?.items) ? state.source.layout.items : []).forEach((item) => {
    const blockId = String(item?.blockId || '').trim();
    if (blockId && !blockIds.has(blockId)) throw new Error(`Layout references unknown block "${blockId}".`);
  });
  const available = new Set(Object.keys(state?.dataSources || {}));
  ['datasets', 'dataSources'].forEach((key) => {
    const declarations = state?.source?.[key];
    if (Array.isArray(declarations)) declarations.forEach((entry) => { if (entry?.id) available.add(String(entry.id)); });
    else if (isPlainObject(declarations)) Object.keys(declarations).forEach((id) => available.add(id));
  });
  allBlocks.forEach((block) => {
    ['dataSourceRef', 'dataSource', 'datasetRef'].forEach((key) => {
      const ref = String(block?.[key] || '').trim();
      if (ref && !available.has(ref)) throw new Error(`Block "${block?.id || ''}" references unavailable datasource "${ref}".`);
    });
    ['childBlockIds', 'sectionIds'].forEach((key) => {
      (Array.isArray(block?.[key]) ? block[key] : []).forEach((refValue) => {
        const ref = String(refValue || '').trim();
        if (!ref || !blockIds.has(ref)) throw new Error(`Block "${block?.id || ''}" references unknown block "${ref}" in ${key}.`);
        if (key === 'sectionIds' && blockKinds.get(ref) !== 'sectionBlock') throw new Error(`Block "${block?.id || ''}" sectionIds references non-section block "${ref}".`);
      });
    });
  });
}

function forgeDataByteLength(value) {
  return new TextEncoder().encode(typeof value === 'string' ? value : JSON.stringify(value ?? null)).length;
}

function validateProgressiveDataLimits(states, candidate, candidateKey) {
  let messageBytes = 0;
  for (const [key, current] of states.entries()) {
    const state = key === candidateKey ? candidate : current;
    const sources = Object.values(state?.dataSources || {});
    if (sources.length > FORGE_REPORT_LIMITS.dataSources) throw new Error(`An inline report may contain at most ${FORGE_REPORT_LIMITS.dataSources} datasources.`);
    let reportBytes = 0;
    sources.forEach((source) => {
      reportBytes += forgeDataByteLength(source?.data);
      if (Array.isArray(source?.data) && source.data.length > FORGE_REPORT_LIMITS.rows) throw new Error(`A static datasource may contain at most ${FORGE_REPORT_LIMITS.rows} rows.`);
      if (String(source?.format || '').toLowerCase() === 'csv') {
        const text = String(source?.data || '').replace(/[\r\n]+$/, '');
        const rows = text ? Math.max(0, csvRecordCount(text) - 1) : 0;
        if (rows > FORGE_REPORT_LIMITS.rows) throw new Error(`A static datasource may contain at most ${FORGE_REPORT_LIMITS.rows} rows.`);
      }
    });
    if (reportBytes > FORGE_REPORT_LIMITS.reportBytes) throw new Error('Static data for one report may not exceed 5 MB.');
    messageBytes += reportBytes;
  }
  if (messageBytes > FORGE_REPORT_LIMITS.messageBytes) throw new Error('Static report data in one assistant message may not exceed 10 MB.');
}

function removeReportBlock(blocks = [], id = '') {
  return (Array.isArray(blocks) ? blocks : []).flatMap((entry) => {
    if (!isPlainObject(entry) || String(entry.id || '').trim() === String(id || '').trim()) return [];
    return [{ ...entry }];
  });
}

function reportDiagnostic(code, message, state, event, path = '') {
  const payload = isPlainObject(event?.payload) ? event.payload : {};
  const firstBlock = Array.isArray(payload.blocks) && isPlainObject(payload.blocks[0]) ? payload.blocks[0] : {};
  const suggestedFix = code === 'REPORT_ALREADY_COMMITTED'
    ? 'Start a new report instance or save and update the committed report through the report API.'
    : code.includes('SEQUENCE')
      ? 'Emit a new transaction with the next sequence number; do not reuse a rejected sequence.'
      : event?.kind === 'data'
        ? 'Correct this datasource transaction and emit it with the next sequence number.'
        : 'Correct this report fragment and emit it with the next sequence number.';
  return {
    code,
    message,
    reportId: state?.id || payload.id || payload.reportRef || '',
    blockId: String(firstBlock.id || '').trim() || undefined,
    dataSourceId: event?.kind === 'data' ? (String(payload.id || '').trim() || undefined) : undefined,
    sequence: Number(payload.sequence || 0) || undefined,
    fence: event?.kind === 'data' ? FORGE_DATA_FENCE : FORGE_REPORT_FENCE,
    path: path || undefined,
    suggestedFix,
  };
}

function applyProgressiveData(state, payload) {
  validateProgressiveEnvelopeFields(payload, DATA_ENVELOPE_FIELDS, FORGE_DATA_FENCE);
  if (Number(payload.version) !== 2) throw new Error('Progressive forge-data requires version 2.');
  const id = String(payload.id || '').trim();
  if (!id) throw new Error('forge-data.id is required.');
  if (!isSafeReportSegment(id)) throw new Error(`Datasource id "${id}" must use letters, numbers, dots, underscores, or hyphens.`);
  const format = String(payload.format || 'json').trim().toLowerCase();
  const mode = String(payload.mode || 'replace').trim().toLowerCase();
  if (!['json', 'csv'].includes(format)) throw new Error(`Unsupported forge-data format: ${format}`);
  if (!['replace', 'append', 'patch'].includes(mode)) throw new Error(`Unsupported forge-data mode: ${mode}`);
  const next = { ...cloneJSON(payload), format, mode };
  const existing = state.dataSources[id];
  if (!existing || mode === 'replace') {
    state.dataSources[id] = next;
    return;
  }
  if (format !== 'json' || existing.format === 'csv') throw new Error(`${mode} requires JSON data.`);
  if (mode === 'append') {
    if (!Array.isArray(existing.data) || !Array.isArray(next.data)) throw new Error('append requires row arrays.');
    next.data = [...existing.data, ...next.data];
  } else {
    if (!isPlainObject(existing.data) || !isPlainObject(next.data)) throw new Error('patch requires JSON objects.');
    next.data = mergeReportJSON(cloneJSON(existing.data), next.data);
  }
  state.dataSources[id] = next;
}

function appendProgressiveReport(state, payload) {
  const target = {
    kind: String(payload?.target?.kind || 'report').trim(),
    ref: String(payload?.target?.ref || 'root').trim(),
    slot: String(payload?.target?.slot || '').trim(),
    position: String(payload?.target?.position || 'append').trim(),
  };
  const incoming = cloneJSON(Array.isArray(payload.blocks) ? payload.blocks : []);
  if (target.kind === 'report') {
    if (target.ref !== 'root' || target.position !== 'append') throw new Error('The report root supports append only.');
    for (const block of incoming) {
      const id = String(block?.id || '').trim();
      if (!id) throw new Error('Every progressive block requires a stable id.');
      if (findReportBlock(state.source.blocks, id)) throw new Error(`Duplicate block id "${id}".`);
      state.source.blocks.push(block);
    }
  } else {
    if (state.grammar !== 'report-document-v1') throw new Error('Block targets require report-document-v1.');
    if (!['childBlockIds', 'sectionIds'].includes(target.slot)) throw new Error(`Unsupported target slot: ${target.slot}`);
    const parent = findReportBlock(state.source.blocks, target.ref);
    if (!parent) throw new Error(`Target block "${target.ref}" does not exist.`);
    if (target.slot === 'childBlockIds' && parent.kind !== 'compositeBlock') throw new Error('childBlockIds requires a compositeBlock target.');
    if (target.slot === 'sectionIds' && parent.kind !== 'tabGroupBlock') throw new Error('sectionIds requires a tabGroupBlock target.');
    if (target.position !== 'append') throw new Error(`Unsupported target position: ${target.position}`);
    if (target.slot === 'sectionIds' && incoming.some((block) => block?.kind !== 'sectionBlock')) throw new Error('sectionIds accepts sectionBlock entries only.');
    for (const block of incoming) {
      const id = String(block?.id || '').trim();
      if (!id) throw new Error('Every progressive block requires a stable id.');
      if (findReportBlock(state.source.blocks, id)) throw new Error(`Duplicate block id "${id}".`);
    }
    parent[target.slot] = [...(Array.isArray(parent[target.slot]) ? parent[target.slot] : []), ...incoming.map((block) => block.id)];
    state.source.blocks.push(...incoming);
  }
  validateStableReportBlockIds(state.source.blocks);
}

function applyProgressiveReport(state, payload) {
  validateProgressiveEnvelopeFields(payload, REPORT_ENVELOPE_FIELDS, FORGE_REPORT_FENCE);
  if (Number(payload.version) !== 1) throw new Error(`Unsupported forge-report version: ${payload.version}`);
  const mode = String(payload.mode || '').trim().toLowerCase();
  const requestedGrammar = String(payload.grammar || '').trim().toLowerCase();
  const grammar = requestedGrammar || DEFAULT_FORGE_REPORT_GRAMMAR;
  if (!['dashboard-v1', 'report-document-v1'].includes(grammar)) throw new Error(`Unsupported report grammar: ${grammar}`);
  if (mode === 'start') {
    if (state.started) throw new Error('Report start was already accepted.');
    state.started = true;
    state.grammar = grammar;
    state.status = 'rendering';
    state.source = reportSource(payload);
    validateStableReportBlockIds(state.source.blocks);
    return;
  }
  if (!state.started) throw new Error(`Report ${mode || 'transaction'} requires an accepted start transaction.`);
  if (requestedGrammar && requestedGrammar !== state.grammar) throw new Error('Report grammar is immutable after start.');
  if (mode === 'append') appendProgressiveReport(state, payload);
  else if (mode === 'patch') {
    const patch = reportSource(payload);
    const blockPatches = Array.isArray(patch.blocks) ? patch.blocks : [];
    delete patch.blocks;
    blockPatches.forEach((blockPatch) => {
      const block = findReportBlock(state.source.blocks, blockPatch?.id);
      if (!block) throw new Error(`Patch references unknown block "${blockPatch?.id || ''}".`);
      mergeReportJSON(block, blockPatch);
    });
    mergeReportJSON(state.source, patch);
    (Array.isArray(payload.removeBlockIds) ? payload.removeBlockIds : []).forEach((id) => {
      state.source.blocks = removeReportBlock(state.source.blocks, id);
    });
    validateStableReportBlockIds(state.source.blocks);
  } else if (mode === 'replace') {
    if (!requestedGrammar) throw new Error('Report replace must restate the established grammar.');
    state.source = reportSource(payload);
    state.resetVersion += 1;
    validateStableReportBlockIds(state.source.blocks);
  } else if (mode === 'commit') {
    const missing = Array.from({ length: state.maxSequence }, (_, index) => index + 1).find((sequence) => !state.seen.has(sequence));
    if (missing) throw new Error(`Cannot commit with missing sequence ${missing}.`);
    state.committed = true;
    state.status = 'committed';
  } else throw new Error(`Unsupported report mode: ${mode}`);
}

function validateProgressiveEnvelopeFields(payload, allowed, label) {
  if (!isPlainObject(payload)) throw new Error(`${label} payload must be a JSON object.`);
  const unknown = Object.keys(payload).find((key) => !allowed.has(key));
  if (unknown) {
    throw new Error(`${label} contains unknown field "${unknown}"; future fields belong under metadata.extensions.`);
  }
}

// assembleForgeReportEvents is the web counterpart of Agently Core's
// platform-neutral assembler. Events retain descriptor indexes so one final
// report replaces its progressive source fences in transcript order.
export function assembleForgeReportEvents(events = []) {
  const states = new Map();
  const diagnostics = [];
  const getState = (scope, id) => {
    const normalizedScope = normalizeReportSegment(scope, 'message');
    const normalizedId = normalizeReportSegment(id);
    const key = `${normalizedScope}:${normalizedId}`;
    if (!states.has(key)) {
      if (states.size >= 4) return null;
      states.set(key, {
        key, scope: normalizedScope, id: normalizedId, grammar: '', status: 'pending',
        sequence: 0, maxSequence: 0, source: {}, dataSources: {}, seen: new Map(),
        started: false, committed: false, lastIndex: -1, fragments: 0, resetVersion: 0,
      });
    }
    return states.get(key);
  };

  (Array.isArray(events) ? events : []).forEach((event) => {
    const payload = event?.payload;
    if (!isPlainObject(payload)) return;
    const reportId = event.kind === 'data' ? payload.reportRef : payload.id;
    if (event.kind === 'data' && Number(payload.version) !== 2) return;
    if (!String(reportId || '').trim()) {
      const isData = event.kind === 'data';
      diagnostics.push(reportDiagnostic(isData ? 'REPORT_DATA_REF_REQUIRED' : 'REPORT_ID_REQUIRED', isData ? 'forge-data version 2 requires reportRef.' : 'forge-report requires id.', null, event, isData ? 'reportRef' : 'id'));
      return;
    }
    if (!isSafeReportSegment(reportId) || (String(payload.scope || '').trim() && !isSafeReportSegment(payload.scope))) {
      diagnostics.push(reportDiagnostic('REPORT_ID_INVALID', 'scope and report id must use letters, numbers, dots, underscores, or hyphens.', null, event, event.kind === 'data' ? 'reportRef' : 'id'));
      return;
    }
    const state = getState(payload.scope, reportId);
    if (!state) {
      diagnostics.push(reportDiagnostic('REPORT_LIMIT_EXCEEDED', 'No more than four inline reports may be assembled in one assistant message.', null, event));
      return;
    }
    state.lastIndex = Math.max(state.lastIndex, Number(event.index || 0));
    const sequence = Number(payload.sequence || 0);
    if (!Number.isInteger(sequence) || sequence <= 0) {
      diagnostics.push(reportDiagnostic('REPORT_SEQUENCE_REQUIRED', 'Progressive transactions require a positive sequence.', state, event, 'sequence'));
      return;
    }
    const canonical = canonicalJSON(payload);
    if (state.committed) {
      if (state.seen.has(sequence)) {
        if (state.seen.get(sequence) !== canonical) diagnostics.push(reportDiagnostic('REPORT_SEQUENCE_CONFLICT', 'The sequence was replayed with different content.', state, event, 'sequence'));
      } else {
        diagnostics.push(reportDiagnostic('REPORT_ALREADY_COMMITTED', 'The report assembly is already committed.', state, event, 'sequence'));
      }
      return;
    }
    if (state.seen.has(sequence)) {
      if (state.seen.get(sequence) !== canonical) diagnostics.push(reportDiagnostic('REPORT_SEQUENCE_CONFLICT', 'The sequence was replayed with different content.', state, event, 'sequence'));
      return;
    }
    if (sequence < state.maxSequence) {
      state.seen.set(sequence, canonical);
      diagnostics.push(reportDiagnostic('REPORT_SEQUENCE_STALE', 'A lower sequence arrived after a newer transaction and was ignored.', state, event, 'sequence'));
      return;
    }
    state.seen.set(sequence, canonical);
    state.maxSequence = Math.max(state.maxSequence, sequence);
    state.sequence = state.maxSequence;
    state.fragments += 1;
    if (state.fragments > FORGE_REPORT_LIMITS.fragments) {
      state.seen.delete(sequence);
      state.fragments -= 1;
      diagnostics.push(reportDiagnostic('REPORT_FRAGMENT_LIMIT_EXCEEDED', `An inline report may contain at most ${FORGE_REPORT_LIMITS.fragments} transactions.`, state, event, 'sequence'));
      return;
    }
    const candidate = {
      ...state,
      source: cloneJSON(state.source),
      dataSources: cloneJSON(state.dataSources),
      seen: state.seen,
    };
    try {
      if (event.kind === 'data') {
        applyProgressiveData(candidate, payload);
        validateProgressiveDataLimits(states, candidate, state.key);
      } else {
        applyProgressiveReport(candidate, payload);
        validateProgressiveReportState(candidate);
      }
      Object.assign(state, candidate);
    } catch (error) {
      state.seen.delete(sequence);
      state.fragments = Math.max(0, state.fragments - 1);
      if (event.kind === 'report' && String(payload.mode || '').toLowerCase() === 'commit') state.status = 'incomplete';
      diagnostics.push(reportDiagnostic(event.kind === 'data' ? 'REPORT_DATA_INVALID' : 'REPORT_TRANSACTION_INVALID', error?.message || 'Invalid progressive report transaction.', state, event));
    }
  });

  const assemblies = [...states.values()].map((state) => {
    if (!state.started) {
      state.status = 'orphaned';
      diagnostics.push({ code: 'REPORT_DATA_ORPHANED', message: 'Progressive report data has no matching forge-report start transaction.', reportId: state.id, fence: FORGE_DATA_FENCE, path: 'reportRef', suggestedFix: 'Add a matching forge-report start transaction in this assistant message.' });
    } else if (!state.committed) {
      const missing = Array.from({ length: state.maxSequence }, (_, index) => index + 1).find((sequence) => !state.seen.has(sequence));
      state.status = missing ? 'incomplete' : 'committed';
      if (missing) diagnostics.push({ code: 'REPORT_SEQUENCE_GAP', message: `Report sequence is missing transaction ${missing}.`, reportId: state.id, sequence: missing, fence: FORGE_REPORT_FENCE, path: 'sequence', suggestedFix: `Replay the missing transaction ${missing} before committing the report.` });
    }
    return {
      scope: state.scope,
      id: state.id,
      grammar: state.grammar,
      status: state.status,
      sequence: state.sequence,
      resetVersion: state.resetVersion,
      source: cloneJSON(state.source),
      dataSources: cloneJSON(state.dataSources),
      lastIndex: state.lastIndex,
    };
  });
  return { assemblies, diagnostics };
}

// Backend execution must call the equivalent server gate with an allowlist
// derived from the effective authenticated user's workspace catalog. This
// client helper provides the same deterministic diagnostics for preflight UX.
export function validateForgeReportWorkspaceReferences(assembly = {}, allowedDataSourceIds = []) {
  const allowed = new Set((Array.isArray(allowedDataSourceIds) ? allowedDataSourceIds : [])
    .map((value) => String(value || '').trim()).filter(Boolean));
  const diagnostics = [];
  ['datasets', 'dataSources'].forEach((key) => {
    const declarations = assembly?.source?.[key];
    const entries = Array.isArray(declarations)
      ? declarations.map((entry, index) => ({ entry, path: `$.${key}[${index}]` }))
      : isPlainObject(declarations)
        ? Object.entries(declarations).map(([id, entry]) => ({ entry: { id, ...(isPlainObject(entry) ? entry : {}) }, path: `$.${key}.${id}` }))
        : [];
    entries.forEach(({ entry, path }) => {
      if (!isPlainObject(entry) || String(entry.kind || '').trim().toLowerCase() !== 'workspaceref') return;
      const ref = String(entry.dataSourceRef || '').trim();
      if (ref && allowed.has(ref)) return;
      diagnostics.push({
        code: 'REPORT_WORKSPACE_REF_DENIED',
        message: `Workspace datasource "${ref}" is not available to the effective authenticated user.`,
        reportId: String(assembly?.id || '').trim(),
        dataSourceId: String(entry.id || '').trim() || undefined,
        sequence: Number(assembly?.sequence || 0) || undefined,
        fence: FORGE_REPORT_FENCE,
        path: `${path}.dataSourceRef`,
        suggestedFix: 'Use a datasource id from the current workspace catalog or remove the live datasource declaration.',
      });
    });
  });
  return diagnostics;
}

// validateForgeUIBlock normalizes a forge-ui payload with permissive defaults
// for backward compatibility. Missing `version` defaults to 1, missing `title`
// defaults to empty string, and missing `blocks` defaults to []. Only a
// fundamentally malformed (non-object) payload throws.
export function validateForgeUIBlock(block = {}) {
  if (!isPlainObject(block)) throw new Error('forge-ui block must be an object');
  const version = String(block.version || '').trim() === '' ? 1 : block.version;
  const title = String(block.title || '').trim();
  const blocks = Array.isArray(block.blocks) ? block.blocks : [];
  return {
    ...block,
    version,
    title,
    blocks: blocks.map((entry, index) => ({
      id: String(entry?.id || `block-${index + 1}`),
      ...entry,
    })),
  };
}

function normalizePlannerSubmitContract(block = {}) {
  const callback = block?.actions?.[0]?.callback || null;
  const context = callback?.context;
  if (!context || typeof context !== 'object' || Array.isArray(context)) {
    return null;
  }
  const domain = String(context.domain || '').trim();
  const submitIntent = String(context.submitIntent || '').trim();
  const selectedKeys = Array.isArray(context.selectedKeys)
    ? context.selectedKeys.map((entry) => String(entry || '').trim()).filter(Boolean)
    : [];
  const toolGuidance = context.toolGuidance && typeof context.toolGuidance === 'object' && !Array.isArray(context.toolGuidance)
    ? context.toolGuidance
    : null;
  const allowedSubmitIntents = Array.isArray(context.allowedSubmitIntents)
    ? context.allowedSubmitIntents.map((entry) => String(entry || '').trim()).filter(Boolean)
    : Array.isArray(context.submitIntentOptions)
      ? context.submitIntentOptions.map((entry) => String(entry || '').trim()).filter(Boolean)
    : [];
  if (!domain && !submitIntent && selectedKeys.length === 0 && !toolGuidance && allowedSubmitIntents.length === 0) {
    return null;
  }
  return {
    domain: domain || undefined,
    submitIntent: submitIntent || undefined,
    allowedSubmitIntents: allowedSubmitIntents.length > 0 ? allowedSubmitIntents : undefined,
    selectedKeys: selectedKeys.length > 0 ? selectedKeys : undefined,
    toolGuidance: toolGuidance || undefined,
  };
}

function filterPlannerRowsForSubmit(rows = [], selectedKeys = []) {
  const keys = Array.isArray(selectedKeys) ? selectedKeys.map((entry) => String(entry || '').trim()).filter(Boolean) : [];
  if (keys.length === 0) {
    return Array.isArray(rows) ? rows : [];
  }
  return (Array.isArray(rows) ? rows : []).map((row) => {
    const next = {};
    keys.forEach((key) => {
      if (row && Object.prototype.hasOwnProperty.call(row, key)) {
        next[key] = row[key];
      }
    });
    return next;
  });
}

export function createPlannerTableSubmitPayload(ui, block, currentRows = [], originalRows = []) {
  const selectionField = String(block?.selection?.field || 'selected').trim();
  const plannerSubmit = normalizePlannerSubmitContract(block);
  const selectedRowsRaw = currentRows.filter((row) => !!row?.[selectionField]);
  const selectedRows = filterPlannerRowsForSubmit(selectedRowsRaw, plannerSubmit?.selectedKeys);
  const unselectedRows = currentRows.filter((row) => !row?.[selectionField]);
  const changedRows = currentRows.filter((row, index) => {
    const before = originalRows[index] || {};
    return Boolean(before?.[selectionField]) !== Boolean(row?.[selectionField]);
  });
  return {
    eventName: String(block?.actions?.[0]?.callback?.eventName || 'planner_table_submit').trim(),
    tableId: String(block?.id || '').trim(),
    dataSourceRef: String(block?.dataSourceRef || '').trim(),
    selectionField,
    columns: (Array.isArray(block?.columns) ? block.columns : []).map((column) => ({
      key: String(column?.key || '').trim(),
      label: String(column?.label || column?.key || '').trim(),
    })).filter((column) => column.key),
    selectedRows,
    selectedRowsRaw,
    unselectedRows,
    changedRows,
    finalDataSourceSnapshot: currentRows,
    callback: block?.actions?.[0]?.callback || null,
    plannerSubmit,
    uiTitle: String(ui?.title || '').trim(),
  };
}

export function createPlannerTableActionPayload(ui, block, action, currentRows = [], originalRows = []) {
  const base = createPlannerTableSubmitPayload(ui, block, currentRows, originalRows);
  const callback = action?.callback || null;
  const eventName = String(callback?.eventName || action?.id || base.eventName || 'planner_table_submit').trim();
  return {
    ...base,
    eventName,
    callback,
    actionId: String(action?.id || '').trim(),
    actionKind: String(action?.kind || '').trim(),
    actionLabel: String(action?.label || '').trim(),
  };
}

export const forgeFenceSample = {
  ui: {
    version: 1,
    title: 'Recommended sites',
    subtitle: 'Review recommendations before submitting',
    blocks: [
      {
        id: 'site-review',
        kind: 'planner.table',
        title: 'Site review',
        dataSourceRef: 'recommended_sites',
        selection: {
          mode: 'checkbox',
          field: 'selected',
        },
        columns: [
          { key: 'site_id', label: 'Site ID' },
          { key: 'site_name', label: 'Site name' },
          { key: 'reason', label: 'Why recommended' },
        ],
        actions: [
          {
            id: 'submit-sites',
            kind: 'submit',
            label: 'Submit changes',
            callback: {
              type: 'llm_event',
              eventName: 'planner_table_submit',
            },
          },
        ],
      },
    ],
  },
  data: [
    {
      version: 1,
      id: 'recommended_sites',
      format: 'csv',
      mode: 'replace',
      data: [
        'site_id,site_name,reason,selected',
        '101,example.com,Strong overlap with converting audience,true',
        '202,publisher.net,High historical click-through on adjacent order,true',
        '303,news-site.org,Relevant content adjacency and scalable native supply,true',
      ].join('\n'),
    },
  ],
};
