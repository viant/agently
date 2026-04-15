export const FORGE_UI_FENCE = 'forge-ui';
export const FORGE_DATA_FENCE = 'forge-data';

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

export function validateForgeDataBlock(block = {}) {
  if (!isPlainObject(block)) throw new Error('forge-data block must be an object');
  if (String(block.version || '').trim() === '') throw new Error('forge-data.version is required');
  if (String(block.id || '').trim() === '') throw new Error('forge-data.id is required');
  const format = String(block.format || '').trim().toLowerCase();
  if (!['json', 'csv'].includes(format)) throw new Error(`Unsupported forge-data format: ${format}`);
  const mode = String(block.mode || 'replace').trim().toLowerCase();
  if (!['replace', 'append', 'patch'].includes(mode)) throw new Error(`Unsupported forge-data mode: ${mode}`);
  return {
    ...block,
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

export function validateForgeUIBlock(block = {}) {
  if (!isPlainObject(block)) throw new Error('forge-ui block must be an object');
  if (String(block.version || '').trim() === '') throw new Error('forge-ui.version is required');
  if (String(block.title || '').trim() === '') throw new Error('forge-ui.title is required');
  if (!Array.isArray(block.blocks)) throw new Error('forge-ui.blocks must be an array');
  return {
    ...block,
    blocks: block.blocks.map((entry, index) => ({
      id: String(entry?.id || `block-${index + 1}`),
      ...entry,
    })),
  };
}

export function createPlannerTableSubmitPayload(ui, block, currentRows = [], originalRows = []) {
  const selectionField = String(block?.selection?.field || 'selected').trim();
  const selectedRows = currentRows.filter((row) => !!row?.[selectionField]);
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
    selectedRows,
    unselectedRows,
    changedRows,
    finalDataSourceSnapshot: currentRows,
    callback: block?.actions?.[0]?.callback || null,
    uiTitle: String(ui?.title || '').trim(),
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
