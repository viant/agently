// RichContent – renders assistant message content with syntax-highlighted code,
// Blueprint tables, Recharts charts, Mermaid diagrams, and inline markdown.
// Parsing and classification logic comes from the SDK's pluggable richContent module.

import React from 'react';
import { autoType, csvParse } from 'd3-dsv';
import CodeBlock from './CodeBlock.jsx';
import Mermaid from './Mermaid';
import { Button, Dialog, Tooltip, Spinner } from '@blueprintjs/core';
import { Table as BpTable, Column as BpColumn, Cell as BpCell, ColumnHeaderCell as BpColumnHeaderCell } from '@blueprintjs/table';
import {
  ResponsiveContainer,
  LineChart,
  Line,
  AreaChart,
  Area,
  BarChart,
  Bar,
  ScatterChart,
  Scatter,
  PieChart,
  Pie,
  Cell,
  CartesianGrid,
  XAxis,
  YAxis,
  Tooltip as RcTooltip,
  Legend as RcLegend,
} from 'recharts';

// Import all parsing/rendering utilities from SDK
import {
  describeContent,
  normalizeChartSpec,
  buildChartSeries,
  renderMarkdownBlock,
  renderMarkdownCellHTML,
  describeFence,
} from 'agently-core-ui-sdk';
import { DashboardBlock } from 'forge/components';
import {
  FORGE_UI_FENCE,
  FORGE_DATA_FENCE,
  applyForgeDataBlocks,
  createPlannerTableSubmitPayload,
  parseForgeFenceBody,
  rowsToCsv,
  validateForgeDataBlock,
  validateForgeUIBlock,
} from '../../services/forgeFenceContract.js';
import { dispatchForgeUIAction } from '../../services/forgeUIActions.js';

const DASHBOARD_BLOCK_KINDS = [
  'dashboard.summary',
  'dashboard.compare',
  'dashboard.kpiTable',
  'dashboard.filters',
  'dashboard.timeline',
  'dashboard.dimensions',
  'dashboard.messages',
  'dashboard.status',
  'dashboard.feed',
  'dashboard.report',
  'dashboard.detail',
];

function isForgeUIFence(fence = {}) {
  return String(fence?.lang || '').trim().toLowerCase() === FORGE_UI_FENCE;
}

function isForgeDataFence(fence = {}) {
  return String(fence?.lang || '').trim().toLowerCase() === FORGE_DATA_FENCE;
}

function hasTrailingOpenForgeFence(content = '', lang = '') {
  const source = String(content || '');
  const normalizedLang = String(lang || '').trim().toLowerCase();
  if (!source || !normalizedLang) return false;
  const openPattern = new RegExp('```(?:' + normalizedLang + ')(?:\\r?\\n|(?=[\\[{]))[\\s\\S]*$', 'i');
  const closedPattern = new RegExp('```(?:' + normalizedLang + ')(?:\\r?\\n|(?=[\\[{]))[\\s\\S]*?```', 'ig');
  const matches = [...source.matchAll(closedPattern)];
  const lastClosedEnd = matches.length > 0 ? matches[matches.length - 1].index + matches[matches.length - 1][0].length : 0;
  const tail = source.slice(lastClosedEnd);
  return openPattern.test(tail);
}

function detectStreamingForgeFenceText(text = '') {
  const source = String(text || '');
  if (hasTrailingOpenForgeFence(source, FORGE_UI_FENCE)) return FORGE_UI_FENCE;
  if (hasTrailingOpenForgeFence(source, FORGE_DATA_FENCE)) return FORGE_DATA_FENCE;
  return '';
}

function ForgeFenceLoading({ label = 'Forge content' }) {
  return (
    <div
      className="app-forge-fence-loading"
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 8,
        padding: '10px 12px',
        borderRadius: 12,
        border: '1px solid #d8e1ee',
        background: '#f7faff',
        color: '#48607a',
        fontSize: 13,
        fontWeight: 600,
        margin: '6px 0'
      }}
    >
      <Spinner size={14} />
      <span>{label}</span>
    </div>
  );
}

function titleizeDashboardKey(value = '') {
  return String(value || '')
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

function createStaticSignal(value) {
  return {
    value,
    peek: () => value,
  };
}

function normalizeDashboardDataSources(dataSources = []) {
  return (Array.isArray(dataSources) ? dataSources : []).map((entry) => {
    if (!entry || typeof entry !== 'object') return entry;
    if (Array.isArray(entry.collection)) return entry;
    const csv = String(entry.csv || '').trim();
    if (!csv) return entry;
    try {
      return { ...entry, collection: csvParse(csv, autoType) };
    } catch (_) {
      return entry;
    }
  });
}

function aggregateDashboardMetric(collection = [], key = '') {
  const values = (Array.isArray(collection) ? collection : [])
    .map((row) => row?.[key])
    .filter((value) => value !== undefined && value !== null && value !== '');
  if (!values.length) return null;
  if (values.every((value) => typeof value === 'number')) {
    if (key === 'ctr' || key === 'vtr') {
      return values.reduce((sum, value) => sum + value, 0) / values.length;
    }
    return values.reduce((sum, value) => sum + value, 0);
  }
  return values[0];
}

function inferMetricFormat(key = '', value) {
  const name = String(key || '').trim().toLowerCase();
  if (name === 'ctr' || name === 'vtr' || name.endsWith('_ctr') || name.endsWith('_vtr')) {
    return 'percent';
  }
  if (typeof value === 'number' && value > 0 && value < 1) {
    return 'percent';
  }
  return undefined;
}

const DEFAULT_DASHBOARD_PALETTE = ['#137cbd', '#0f9960', '#d9822b', '#8f3985', '#c23030'];

function normalizeMetricValue(value) {
  if (Array.isArray(value)) {
    return value.map((entry) => String(entry ?? '').trim()).filter(Boolean).join(', ');
  }
  return value;
}

function dashboardColumnKey(column) {
  if (typeof column === 'string') {
    return String(column).trim();
  }
  return String(column?.key || column?.id || '').trim();
}

function dashboardSeriesKey(entry) {
  if (typeof entry === 'string') {
    return String(entry).trim();
  }
  return String(entry?.key || entry?.value || entry?.id || '').trim();
}

function aggregateRowsByDimension(collection = [], dimension = '', metrics = []) {
  const grouped = new Map();
  for (const row of Array.isArray(collection) ? collection : []) {
    const groupValue = row?.[dimension];
    if (groupValue === undefined || groupValue === null || groupValue === '') {
      continue;
    }
    const key = String(groupValue);
    if (!grouped.has(key)) {
      grouped.set(key, { __count: 0 });
    }
    const bucket = grouped.get(key);
    bucket.__count += 1;
    for (const metric of metrics) {
      const value = row?.[metric];
      if (typeof value !== 'number') {
        if (bucket[metric] == null && value != null) {
          bucket[metric] = value;
        }
        continue;
      }
      if (metric === 'ctr' || metric === 'vtr') {
        bucket[metric] = (bucket[metric] || 0) + value;
      } else {
        bucket[metric] = (bucket[metric] || 0) + value;
      }
    }
  }
  const rows = [];
  for (const [groupValue, bucket] of grouped.entries()) {
    const row = { [dimension]: groupValue };
    for (const metric of metrics) {
      const value = bucket[metric];
      if (typeof value === 'number' && (metric === 'ctr' || metric === 'vtr') && bucket.__count > 0) {
        row[metric] = value / bucket.__count;
      } else {
        row[metric] = value ?? null;
      }
    }
    rows.push(row);
  }
  return rows;
}

export function normalizeDashboardPayload(payload) {
  if (!isDashboardPayload(payload)) return payload;
  const dataSources = normalizeDashboardDataSources(payload?.dataSources);
  const metrics = { ...(payload?.metrics || {}) };
  const normalizedBlocks = (Array.isArray(payload?.blocks) ? payload.blocks : []).map((block, index) => {
    const sourceID = block?.dataSourceRef || block?.dataSource;
    const source = dataSources.find((entry) => entry && (entry.id === sourceID || entry.name === sourceID)) || null;
    const collection = Array.isArray(source?.collection) ? source.collection : [];
    const keyBase = `block_${index}`;

    if (block?.kind === 'dashboard.summary' && Array.isArray(block.metrics) && collection.length) {
      const target = {};
      for (const metric of block.metrics) {
        if (typeof metric !== 'string') continue;
        target[metric] = aggregateDashboardMetric(collection, metric);
      }
      metrics[keyBase] = target;
      return {
        ...block,
        metrics: Object.keys(target).map((metric) => ({
          id: metric,
          label: metric.toUpperCase(),
          selector: `${keyBase}.${metric}`,
          format: inferMetricFormat(metric, target[metric]),
        })),
      };
    }

    if (block?.kind === 'dashboard.summary' && Array.isArray(block.items)) {
      const target = {};
      for (const item of block.items) {
        const metricKey = String(item?.metricKey || item?.id || item?.label || '').trim();
        if (!metricKey) continue;
        target[metricKey] = normalizeMetricValue(item?.value ?? null);
      }
      metrics[keyBase] = target;
      return {
        ...block,
        metrics: Object.keys(target).map((metric) => ({
          id: metric,
          label: titleizeDashboardKey(metric),
          selector: `${keyBase}.${metric}`,
          format: inferMetricFormat(metric, target[metric]),
        })),
      };
    }

    if (block?.kind === 'dashboard.kpiTable' && Array.isArray(block.columns)) {
      const columnKeys = block.columns.map(dashboardColumnKey).filter(Boolean);
      if (Array.isArray(block.rows)) {
        return {
          ...block,
          columns: columnKeys,
          rows: block.rows.map((row) =>
            Array.isArray(row) ? row : columnKeys.map((column) => row?.[column])
          ),
        };
      }
      if (!collection.length) {
        return {
          ...block,
          columns: columnKeys,
        };
      }
      return {
        ...block,
        columns: columnKeys,
        rows: collection.map((row) => columnKeys.map((column) => row?.[column])),
      };
    }

    if (block?.kind === 'dashboard.compare' && Array.isArray(block.metrics) && collection.length) {
      const compareMetrics = {};
      const compareItems = [];
      const rowField = block.groupBy || block.dimension || 'order';
      const rows = Array.isArray(block.rows) && block.rows.length > 0
        ? block.rows
        : Array.from(new Set(collection.map((row) => row?.[rowField]).filter((value) => value !== undefined && value !== null))).slice(0, 2);
      if (rows.length < 2) {
        return block;
      }
      const aggregatedRows = aggregateRowsByDimension(collection, rowField, block.metrics);
      for (const metric of block.metrics) {
        const currentRow = aggregatedRows.find((row) => String(row?.[rowField]) === String(rows[0]));
        const previousRow = aggregatedRows.find((row) => String(row?.[rowField]) === String(rows[1]));
        compareMetrics[metric] = {
          current: currentRow?.[metric] ?? null,
          previous: previousRow?.[metric] ?? null,
        };
        compareItems.push({
          id: metric,
          label: titleizeDashboardKey(metric),
          current: `${keyBase}.${metric}.current`,
          previous: `${keyBase}.${metric}.previous`,
          currentLabel: String(rows[0] ?? '').trim(),
          previousLabel: String(rows[1] ?? '').trim(),
          deltaLabel: String(rows[1] ?? '').trim() ? `vs ${String(rows[1]).trim()}` : 'vs previous',
          format: metric === 'ctr' || metric === 'vtr' ? 'percent' : undefined,
        });
      }
      metrics[keyBase] = compareMetrics;
      return {
        ...block,
        groupBy: rowField,
        items: compareItems,
      };
    }

    if (block?.kind === 'dashboard.timeline' && !block.chart && sourceID) {
      const chartType = String(block.chartType || 'line').trim().toLowerCase() || 'line';
      if (block.timeColumn && block.seriesColumn && block.valueColumn && collection.length) {
        const transformedCollection = collection.map((row) => ({
          ...row,
          series: [row?.[block.groupBy], row?.[block.seriesColumn]].filter(Boolean).join(' · '),
        }));
        return {
          ...block,
          dataSourceRef: sourceID,
          __collection: transformedCollection,
          chart: {
            type: chartType,
            xAxis: {
              dataKey: block.timeColumn,
              label: titleizeDashboardKey(block.timeColumn),
              tickFormat: 'MM/dd',
            },
            yAxis: {
              label: titleizeDashboardKey(block.valueColumn),
            },
            cartesianGrid: {
              strokeDasharray: '3 3',
            },
            series: {
              nameKey: 'series',
              valueKey: block.valueColumn,
              values: [{ label: titleizeDashboardKey(block.valueColumn), name: titleizeDashboardKey(block.valueColumn), value: block.valueColumn }],
              palette: DEFAULT_DASHBOARD_PALETTE,
            },
          },
        };
      }
      const seriesValues = (Array.isArray(block.series) ? block.series : (block.valueColumn ? [block.valueColumn] : ['value']))
        .map(dashboardSeriesKey)
        .filter(Boolean);
      if ((block.dateField || block.timeColumn) && seriesValues.length > 0 && !block.groupBy && !block.seriesColumn) {
        const dateKey = block.dateField || block.timeColumn || 'date';
        const transformedCollection = collection.flatMap((row) =>
          seriesValues.map((entry) => ({
            [dateKey]: row?.[dateKey],
            series: titleizeDashboardKey(entry),
            value: row?.[entry],
          }))
        );
        return {
          ...block,
          dataSourceRef: sourceID,
          __collection: transformedCollection,
          chart: {
            type: chartType,
            xAxis: {
              dataKey: dateKey,
              label: titleizeDashboardKey(dateKey),
              tickFormat: 'MM/dd',
            },
            yAxis: {
              label: titleizeDashboardKey(seriesValues[0] || 'value'),
            },
            cartesianGrid: {
              strokeDasharray: '3 3',
            },
            series: {
              nameKey: 'series',
              valueKey: 'value',
              values: seriesValues.map((entry) => ({ label: titleizeDashboardKey(entry), name: titleizeDashboardKey(entry), value: entry })),
              palette: DEFAULT_DASHBOARD_PALETTE,
            },
          },
        };
      }
      return {
        ...block,
        dataSourceRef: sourceID,
        mapping: {
          dateColumn: block.dateField || block.timeColumn || 'date',
          seriesColumns: [block.groupBy || block.seriesColumn || 'order', ...seriesValues],
        },
        chart: {
          type: chartType,
          xAxis: {
            dataKey: block.dateField || block.timeColumn || 'date',
            label: titleizeDashboardKey(block.dateField || block.timeColumn || 'date'),
            tickFormat: 'MM/dd',
          },
          yAxis: {
            label: titleizeDashboardKey(seriesValues[0] || 'value'),
          },
          cartesianGrid: {
            strokeDasharray: '3 3',
          },
          series: {
            nameKey: block.groupBy || block.seriesColumn || 'order',
            valueKey: seriesValues[0] || 'value',
            values: seriesValues.map((entry) => ({ label: titleizeDashboardKey(entry), name: titleizeDashboardKey(entry), value: entry })),
            palette: DEFAULT_DASHBOARD_PALETTE,
          },
        },
      };
    }

    if (block?.kind === 'dashboard.messages' && Array.isArray(block.messages) && !Array.isArray(block.items)) {
      return {
        ...block,
        items: block.messages.map((message, messageIndex) => {
          if (typeof message === 'string') {
            return { severity: 'info', title: `Note ${messageIndex + 1}`, body: message };
          }
          return {
            severity: message?.severity || message?.type || 'info',
            title: message?.title || `Note ${messageIndex + 1}`,
            body: message?.body || message?.text || '',
          };
        }),
      };
    }

    return block;
  });

  return {
    ...payload,
    metrics,
    dataSources,
    blocks: normalizedBlocks,
  };
}

function isDashboardPayload(value) {
  if (!value || typeof value !== 'object') return false;
  const blocks = Array.isArray(value.blocks) ? value.blocks : [];
  if (!blocks.length) return false;
  return blocks.some((block) => DASHBOARD_BLOCK_KINDS.includes(String(block?.kind || '').trim()));
}

function buildForgeDataSources(store = {}) {
  return Object.values(store).map((entry) => ({
    id: entry?.id,
    name: entry?.id,
    collection: Array.isArray(entry?.rows) ? entry.rows : [],
  }));
}

function buildForgeDashboardContext(payload, dataStore, block) {
  const sourceID = block?.dataSourceRef || block?.dataSource;
  const source = sourceID ? dataStore?.[sourceID] : null;
  const collection = Array.isArray(block?.__collection)
    ? block.__collection
    : Array.isArray(source?.rows) ? source.rows : [];
  const dashboardKey = `forge-ui:${String(payload?.title || 'message')}`;
  return {
    dashboardKey,
    identity: { windowId: dashboardKey, dashboardKey },
    locale: 'en-US',
    signals: {
      metrics: createStaticSignal(payload?.metrics || {}),
      collection: createStaticSignal(collection),
      control: createStaticSignal({}),
      selection: createStaticSignal({}),
    },
    handlers: {
      dataSource: {
        setSelected: () => {},
        getSelection: () => ({ selected: null }),
      },
    },
  };
}

function PlannerTableBlock({ ui, block, dataStore }) {
  const sourceID = String(block?.dataSourceRef || '').trim();
  const source = sourceID ? dataStore?.[sourceID] : null;
  const originalRows = React.useMemo(
    () => (Array.isArray(source?.rows) ? source.rows : []).map((row) => ({ ...row })),
    [source]
  );
  const [rows, setRows] = React.useState(originalRows);

  React.useEffect(() => {
    setRows(originalRows);
  }, [originalRows]);

  const selectionField = String(block?.selection?.field || 'selected').trim() || 'selected';
  const plannerKindLabel = String(block?.kind || '').trim() === 'planner.table' ? 'planner' : String(block?.kind || '').trim();
  const changedCount = React.useMemo(
    () => rows.filter((row, index) => Boolean(row?.[selectionField]) !== Boolean(originalRows[index]?.[selectionField])).length,
    [rows, originalRows, selectionField]
  );

  const payload = React.useMemo(
    () => createPlannerTableSubmitPayload(ui, block, rows, originalRows),
    [ui, block, rows, originalRows]
  );

  const toggleRow = (rowIndex) => {
    setRows((current) => current.map((row, index) => (
      index === rowIndex ? { ...row, [selectionField]: !row?.[selectionField] } : row
    )));
  };

  const handleSubmit = () => {
    const callback = block?.actions?.[0]?.callback || {};
    const eventName = String(callback?.eventName || 'planner_table_submit').trim() || 'planner_table_submit';
    const detail = {
      ...payload,
      callback,
      eventName,
    };
    dispatchForgeUIAction(detail);
  };

  const downloadCsv = () => {
    if (typeof window === 'undefined') return;
    const csv = rowsToCsv(rows, block?.columns || []);
    const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${String(block?.id || 'planner-table').trim() || 'planner-table'}.csv`;
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
    URL.revokeObjectURL(url);
  };

  return (
    <div style={{
      display: 'grid',
      gap: 14,
      border: '1px solid #d8e1ee',
      borderRadius: 18,
      background: '#ffffff',
      padding: '18px 20px',
      boxShadow: '0 18px 38px rgba(17, 29, 53, 0.08)',
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start' }}>
        <div>
          <div style={{ fontSize: 18, fontWeight: 700, lineHeight: 1.2 }}>{block?.title || 'Planner table'}</div>
          {block?.subtitle ? <div style={{ marginTop: 6, color: 'var(--dark-gray3)', fontSize: 13 }}>{block.subtitle}</div> : null}
        </div>
        <div style={{
          borderRadius: 999,
          padding: '6px 10px',
          background: '#eef4ff',
          color: '#2d67c7',
          fontSize: 11,
          fontWeight: 700,
          textTransform: 'uppercase',
          letterSpacing: '0.04em',
        }}>{plannerKindLabel}</div>
      </div>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={plannerHeaderStyle}>Review</th>
              {(Array.isArray(block?.columns) ? block.columns : []).map((column) => (
                <th key={column?.key || column?.label} style={plannerHeaderStyle}>{column?.label || column?.key}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, rowIndex) => (
              <tr key={String(row?.id || row?.site_id || rowIndex)}>
                <td style={plannerCellStyle}>
                  <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontWeight: 600 }}>
                    <input
                      type="checkbox"
                      data-site-toggle={rowIndex}
                      checked={Boolean(row?.[selectionField])}
                      onChange={() => toggleRow(rowIndex)}
                    />
                    <span>{row?.[selectionField] ? 'Keep' : 'Drop'}</span>
                  </label>
                </td>
                {(Array.isArray(block?.columns) ? block.columns : []).map((column) => (
                  <td key={`${column?.key || column?.label}-${rowIndex}`} style={plannerCellStyle}>
                    {String(row?.[column?.key] ?? '')}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
        <div style={{ fontSize: 13, color: 'var(--dark-gray3)' }}>
          {changedCount > 0 ? `${changedCount} row${changedCount === 1 ? '' : 's'} changed` : 'No changes yet'}
        </div>
        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
          <button
            type="button"
            onClick={downloadCsv}
            data-forge-download={String(block?.id || '').trim() || 'planner-table'}
            style={{
              border: '1px solid #d8e1ee',
              borderRadius: 999,
              padding: '12px 18px',
              fontSize: 14,
              fontWeight: 700,
              cursor: 'pointer',
              background: '#ffffff',
              color: '#2d67c7',
            }}
          >
            Download CSV
          </button>
          {Array.isArray(block?.actions) && block.actions.length > 0 ? (
            <button
              type="button"
              onClick={handleSubmit}
              id={`submit-${String(block?.id || '').trim() || 'planner-table'}`}
              data-forge-submit={String(block?.id || '').trim() || 'planner-table'}
              disabled={changedCount === 0}
              style={{
                border: 'none',
                borderRadius: 999,
                padding: '12px 18px',
                fontSize: 14,
                fontWeight: 700,
                cursor: changedCount === 0 ? 'not-allowed' : 'pointer',
                opacity: changedCount === 0 ? 0.5 : 1,
                background: '#2d67c7',
                color: '#ffffff',
                boxShadow: changedCount === 0 ? 'none' : '0 8px 18px rgba(45, 103, 199, 0.24)',
              }}
            >
              {block.actions[0]?.label || 'Submit'}
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

const plannerHeaderStyle = {
  textAlign: 'left',
  color: '#6b7688',
  fontSize: 12,
  letterSpacing: '0.06em',
  textTransform: 'uppercase',
  padding: '0 0 10px',
  borderBottom: '1px solid #d8e1ee',
};

const plannerCellStyle = {
  padding: '14px 0',
  borderBottom: '1px solid #eef2f8',
  verticalAlign: 'top',
  fontSize: 14,
};

// ForgeRenderErrorBoundary isolates a forge-ui fence render failure so a single
// malformed block can't crash the whole chat transcript. It shows a friendly
// inline message and logs details to the console for debugging.
class ForgeRenderErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }
  static getDerivedStateFromError(error) {
    return { error };
  }
  componentDidCatch(error, info) {
    // eslint-disable-next-line no-console
    console.warn('forge-ui render failed', error?.message || error, info);
  }
  render() {
    if (this.state.error) {
      const msg = this.state.error?.message || String(this.state.error);
      return (
        <div
          role="note"
          style={{
            padding: '10px 12px',
            borderRadius: 8,
            border: '1px solid #f5c542',
            background: '#fff5d6',
            color: '#8a5d00',
            fontSize: 13,
            lineHeight: 1.45,
          }}
        >
          <div style={{ fontWeight: 600, marginBottom: 2 }}>
            Component cannot be loaded
          </div>
          <div style={{ fontSize: 12 }}>
            The assistant generated a dashboard payload that couldn't be rendered
            ({msg || 'unknown error'}). This usually means a required field is
            missing. The rest of the message is still shown below.
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

function ForgeUIFenceInner({ payload, dataBlocks = [] }) {
  const ui = validateForgeUIBlock(payload);
  const dataStore = React.useMemo(() => applyForgeDataBlocks(dataBlocks), [dataBlocks]);
  const forgePayload = React.useMemo(() => ({
    title: ui.title,
    subtitle: ui.subtitle,
    dataSources: buildForgeDataSources(dataStore),
    blocks: ui.blocks,
  }), [ui, dataStore]);

  return (
    <div className="app-rich-dashboard" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        {ui.title ? <div className="app-rich-chart-title">{ui.title}</div> : null}
        {ui.subtitle ? <div style={{ fontSize: 12, color: 'var(--dark-gray3)' }}>{ui.subtitle}</div> : null}
      </div>
      {ui.blocks.map((block, index) => {
        const kind = String(block?.kind || '').trim();
        if (kind.startsWith('dashboard.')) {
          try {
            const normalizedPayload = normalizeDashboardPayload(forgePayload);
            const normalizedBlock = normalizedPayload.blocks[index] || block;
            return (
              <DashboardBlock
                key={String(block?.id || `${block?.kind || 'dashboard'}-${index}`)}
                container={normalizedBlock}
                context={buildForgeDashboardContext(normalizedPayload, dataStore, normalizedBlock)}
                isActive={true}
              />
            );
          } catch (e) {
            // eslint-disable-next-line no-console
            console.warn('dashboard block render prep failed', kind, e);
            return (
              <div key={String(block?.id || index)} style={{ color: '#8a5d00', fontSize: 12 }}>
                Skipped block “{kind}”: {e?.message || 'render error'}
              </div>
            );
          }
        }
        if (kind === 'planner.table') {
          return <PlannerTableBlock key={String(block?.id || index)} ui={ui} block={block} dataStore={dataStore} />;
        }
        return (
          <div key={String(block?.id || index)} style={{ color: 'var(--dark-gray3)', fontSize: 13 }}>
            Unsupported forge-ui block kind: {kind}
          </div>
        );
      })}
    </div>
  );
}

function ForgeUIFence({ payload, dataBlocks = [] }) {
  return (
    <ForgeRenderErrorBoundary>
      <ForgeUIFenceInner payload={payload} dataBlocks={dataBlocks} />
    </ForgeRenderErrorBoundary>
  );
}

function escapeHTMLAttr(value = '') {
  return String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function generatedFileKey(files = []) {
  if (!Array.isArray(files) || !files.length) return '';
  return files
    .map((file) => {
      const id = String(file?.id || '').trim();
      const filename = String(file?.filename || '').trim().toLowerCase();
      const status = String(file?.status || '').trim().toLowerCase();
      return `${id}|${filename}|${status}`;
    })
    .filter(Boolean)
    .join(',');
}

function normalizeSandboxFilename(url = '') {
  let raw = String(url || '').trim();
  if (!raw || !/^sandbox:\//i.test(raw)) return '';
  raw = raw.replace(/^sandbox:\/*/i, '');
  const parts = raw.split('/');
  const last = parts.length ? parts[parts.length - 1] : '';
  if (!last) return '';
  try {
    return decodeURIComponent(last).trim();
  } catch (_) {
    return last.trim();
  }
}

function resolveMarkdownHref(url = '', generatedFiles = []) {
  const href = String(url || '').trim();
  if (!href || !/^sandbox:\//i.test(href)) return href;
  const filename = normalizeSandboxFilename(href);
  if (!filename) return href;
  const want = filename.toLowerCase();
  const files = Array.isArray(generatedFiles) ? generatedFiles : [];
  const match = files.find((file) => {
    const id = String(file?.id || '').trim();
    const name = String(file?.filename || '').trim().toLowerCase();
    return !!id && name === want;
  });
  if (!match?.id) return href;
  return `/v1/api/generated-files/${encodeURIComponent(String(match.id).trim())}/download`;
}

function rewriteSandboxMarkdownLinks(text = '', generatedFiles = []) {
  return String(text || '').replace(/\[([^\]]+)\]\((sandbox:[^)]+)\)/gi, (match, label, url) => {
    const href = resolveMarkdownHref(url, generatedFiles);
    return `[${label}](${href})`;
  });
}

function rewriteSandboxHrefInHTML(html = '', generatedFiles = []) {
  return String(html || '').replace(/href=(["'])(sandbox:[^"']+)\1/gi, (match, quote, url) => {
    const href = resolveMarkdownHref(url, generatedFiles);
    return `href=${quote}${escapeHTMLAttr(href)}${quote}`;
  });
}

function normalizeBrokenMarkdownLayout(text = '') {
  let value = String(text || '');
  if (!value) return '';

  // Some streamed/finalized responses collapse a markdown heading and the
  // following pipe-table onto one line, e.g.:
  // - "### Daily Trend | Date | ..."
  // - "### Recommendation2| Publisher | ..."
  // - "**Raw Evidence**| entity | ..."
  // Split that back into a block line plus the table block.
  value = value.replace(/^((?:#{1,6}\s+.+?|\*\*[^*\n]+\*\*|__[^_\n]+__))(?:\s*)(\|.+)$/gm, '$1\n\n$2');

  // Some responses also collapse a heading and the first bullet onto one line,
  // e.g.:
  // - "## Highlights- Item"
  // - "**Evidence context**- **Metric window:** ..."
  // Preserve the block line and start the list below.
  value = value.replace(/^((?:#{1,6}\s+[^\n]+?|\*\*[^*\n]+\*\*|__[^_\n]+__))(?:\s*)([-*]\s+)/gm, '$1\n\n$2');

  // Forecasting responses sometimes collapse multiple bullets onto one line,
  // e.g.:
  // - "- Deal:141952- Best available day: Day1-3-day total inventory:..."
  // Split each bullet back onto its own line when a bullet line contains a
  // later "- " marker followed by a title-ish token.
  value = value
    .split('\n')
    .map((line) => {
      if (!/^\s*-\s+/.test(line)) return line;
      return line
        .replace(/(?<=\S)-\s(?=[A-Z0-9][^:\n]{0,60}:)/g, '\n- ')
        .replace(/(?<=\S)-\s(?=(?:Uniques|Completed days|Total uniques|Average clearing price|Best available day|3-day total inventory)\b)/g, '\n- ');
    })
    .join('\n');

  // Forecasting formatting occasionally glues common labels/numbers together.
  // Repair the user-visible text form before markdown render.
  value = value.replace(/\bDeal(?=\d)/g, 'Deal ');
  value = value.replace(/\bdeal(?=\d)/g, 'deal ');
  value = value.replace(/\bDay(?=\d)/g, 'Day ');
  value = value.replace(/(?<=:\s?)(\d)(?=day\b)/gi, '$1');
  value = value.replace(/(:)(\d)/g, '$1 $2');
  value = value.replace(/(\$)(\d)/g, '$1$2');
  value = value.replace(/\bthe(?=3-day\b)/gi, 'the ');
  value = value.replace(/\bcurrent(?=3-day\b)/gi, 'current ');
  value = value.replace(/\bacross(?=3-day\b)/gi, 'across ');
  value = value.replace(/\bthis(?=3-day\b)/gi, 'this ');
  value = value.replace(/\bDeal:\s*(\d+)/g, 'Deal: $1');
  value = value.replace(/\bDeal\s+(\d+)-\s+/g, 'Deal $1\n- ');

  // Keep common forecast day labels separated from following prose.
  value = value.replace(/\b(Day\s+\d)\s*\|\s*([^\n]+?)(?=Day\s+\d\s*\||$)/g, (match, day, rest) => {
    const cleaned = String(rest || '').trim();
    return `${day} | ${cleaned}`;
  });

  // Normalize compact chart fences like ```json{ to a standard fenced block.
  value = value.replace(/```json\s*\{/g, '```json\n{');

  return value;
}

function useMeasuredContainer() {
  const ref = React.useRef(null);
  const [size, setSize] = React.useState({ width: 0, height: 0 });

  React.useEffect(() => {
    const node = ref.current;
    if (!node || typeof ResizeObserver === 'undefined') return undefined;
    const update = () => {
      const nextWidth = Number(node.clientWidth || 0);
      const nextHeight = Number(node.clientHeight || 0);
      setSize((prev) => (
        prev.width === nextWidth && prev.height === nextHeight
          ? prev
          : { width: nextWidth, height: nextHeight }
      ));
    };
    update();
    const observer = new ResizeObserver(() => update());
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return [ref, size];
}

function MinimalText({ text = '', generatedFiles = [] }) {
  const cleaned = rewriteSandboxMarkdownLinks(
    normalizeBrokenMarkdownLayout(String(text || '').replace(/^\s*<!--\s*CHART_SPEC:v1\s*-->\s*$/gim, '').trim()),
    generatedFiles
  );
  const html = rewriteSandboxHrefInHTML(renderMarkdownBlock(cleaned), generatedFiles);
  return <div className="app-rich-prose" dangerouslySetInnerHTML={{ __html: html }} />;
}

function ChartSpecPanel({ spec = {} }) {
  const [mode, setMode] = React.useState('chart');
  const [canvasRef, canvasSize] = useMeasuredContainer();
  const [chartReady, setChartReady] = React.useState(false);
  const n = React.useMemo(() => normalizeChartSpec(spec), [spec]);
  const { rows, series } = React.useMemo(() => buildChartSeries(n), [n]);
  const { type, xKey, palette, title } = n;
  const headers = React.useMemo(() => {
    const keys = new Set();
    (n.data || []).forEach((r) => Object.keys(r || {}).forEach((k) => keys.add(k)));
    return Array.from(keys);
  }, [n.data]);

  React.useEffect(() => {
    if (mode !== 'chart' || canvasSize.width <= 0 || canvasSize.height <= 0) {
      setChartReady(false);
      return undefined;
    }
    const raf = window.requestAnimationFrame(() => setChartReady(true));
    return () => window.cancelAnimationFrame(raf);
  }, [mode, canvasSize.width, canvasSize.height]);

  const chartCommon = (
    <>
      <CartesianGrid strokeDasharray="3 3" />
      <XAxis dataKey={xKey} />
      <YAxis />
      <RcTooltip />
      <RcLegend />
    </>
  );

  const renderChart = () => {
    if (!rows.length || !series.length) return <div style={{ fontSize: 12, color: 'var(--dark-gray3)' }}>No chart data</div>;
    if (type === 'area') return <AreaChart data={rows}>{chartCommon}{series.map((s, i) => <Area key={s} type="monotone" dataKey={s} stroke={palette[i % palette.length]} fill={palette[i % palette.length]} fillOpacity={0.2} />)}</AreaChart>;
    if (type === 'bar' || type === 'stacked_bar') {
      const stacked = type === 'stacked_bar';
      return <BarChart data={rows}>{chartCommon}{series.map((s, i) => <Bar key={s} dataKey={s} fill={palette[i % palette.length]} stackId={stacked ? 'a' : undefined} />)}</BarChart>;
    }
    if (type === 'scatter') return <ScatterChart><CartesianGrid strokeDasharray="3 3" /><XAxis dataKey={xKey} /><YAxis dataKey={series[0]} /><RcTooltip /><RcLegend /><Scatter name={series[0]} data={rows} fill={palette[0]} /></ScatterChart>;
    if (type === 'pie' || type === 'donut') {
      const donut = type === 'donut';
      return (
        <PieChart>
          <RcTooltip />
          <RcLegend />
          <Pie
            data={n.data || []}
            dataKey={series[0] || 'value'}
            nameKey={n.seriesKey || xKey}
            outerRadius={110}
            innerRadius={donut ? 58 : 0}
            label
          >
            {(n.data || []).map((_, i) => <Cell key={`cell-${i}`} fill={palette[i % palette.length]} />)}
          </Pie>
        </PieChart>
      );
    }
    return <LineChart data={rows}>{chartCommon}{series.map((s, i) => <Line key={s} type="monotone" dataKey={s} stroke={palette[i % palette.length]} dot={false} />)}</LineChart>;
  };

  return (
    <div className="app-rich-chart">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <div className="app-rich-chart-title">{title || 'Chart'}</div>
        <div style={{ display: 'flex', gap: 6 }}>
          <Button small minimal={mode !== 'chart'} intent={mode === 'chart' ? 'primary' : 'none'} onClick={() => setMode('chart')}>Chart</Button>
          <Button small minimal={mode !== 'table'} intent={mode === 'table' ? 'primary' : 'none'} onClick={() => setMode('table')}>Table</Button>
        </div>
      </div>
      {mode === 'chart' ? (
        <div className="app-rich-chart-canvas" ref={canvasRef}>
          {chartReady && canvasSize.width > 0 && canvasSize.height > 0 ? (
            <ResponsiveContainer width="100%" height="100%">{renderChart()}</ResponsiveContainer>
          ) : null}
        </div>
      ) : (
        <FencedPipeTable
          headers={headers}
          rows={(n.data || []).map((r) => headers.map((h) => String(r?.[h] ?? '')))}
          aligns={headers.map(() => 'left')}
        />
      )}
    </div>
  );
}

// ── Pipe table rendering (Blueprint Table) ──

function FencedPipeTable({ headers = [], rows = [], aligns = [], generatedFiles = [] }) {
  const pageSize = 40;
  const [visible, setVisible] = React.useState(() => new Set(headers.map((_, i) => i)));
  const [page, setPage] = React.useState(0);
  const [showCols, setShowCols] = React.useState(false);
  const [expand, setExpand] = React.useState(null);
  const [truncateAt, setTruncateAt] = React.useState(100);
  const [widthByCol, setWidthByCol] = React.useState({});

  const total = Array.isArray(rows) ? rows.length : 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(0, page), pageCount - 1);
  const start = safePage * pageSize;
  const end = Math.min(total, start + pageSize);
  const pageRows = React.useMemo(() => rows.slice(start, end), [rows, start, end]);

  const visIdx = headers.map((_, i) => i).filter(i => visible.has(i));
  const toggleColumn = (i) => setVisible(prev => { const n = new Set(prev); if (n.has(i)) n.delete(i); else n.add(i); return n; });

  const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));
  const allMaxLens = headers.map((h, i) => {
    let m = String(h || '').length;
    for (const r of rows) { const c = String((r || [])[i] ?? '').length; if (c > m) m = c; }
    return clamp(m, 4, 48);
  });
  const baseWidthPx = 720;
  const totalLens = visIdx.reduce((acc, i) => acc + allMaxLens[i], 0) || visIdx.length;
  const computedColWidths = visIdx.map(i => Math.max(80, Math.round((allMaxLens[i] / totalLens) * baseWidthPx)));
  const colWidths = visIdx.map((i, j) => {
    const persisted = Number(widthByCol[i]);
    if (Number.isFinite(persisted) && persisted > 40) return persisted;
    return computedColWidths[j];
  });

  const downloadCSV = () => {
    try {
      const esc = (v = '') => { const s = String(v ?? ''); if (/[,"\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"'; return s; };
      const lines = [visIdx.map(i => esc(headers[i])).join(',')];
      for (const r of rows) lines.push(visIdx.map(i => esc((r || [])[i] ?? '')).join(','));
      const blob = new Blob(['\ufeff' + lines.join('\n')], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `table-${new Date().toISOString().replace(/[:.]/g, '-')}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (_) {}
  };

  const columns = visIdx.map((ci) => {
    const align = aligns[ci] || 'left';
    const cellRenderer = (rowIndex) => {
      const raw = pageRows[rowIndex]?.[ci] ?? '';
      const text = String(raw);
      const isLong = text.length > truncateAt;
      const display = isLong ? (text.slice(0, truncateAt) + '…') : text;
      const html = rewriteSandboxHrefInHTML(
        renderMarkdownCellHTML(rewriteSandboxMarkdownLinks(display, generatedFiles)),
        generatedFiles
      );
      const fullHtml = rewriteSandboxHrefInHTML(
        renderMarkdownCellHTML(rewriteSandboxMarkdownLinks(text, generatedFiles)),
        generatedFiles
      );
      const cellContent = <span dangerouslySetInnerHTML={{ __html: html }} />;
      const tooltipContent = text ? <div style={{ maxWidth: 640, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }} dangerouslySetInnerHTML={{ __html: fullHtml }} /> : null;
      return (
        <BpCell
          style={{ textAlign: align, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', cursor: isLong ? 'pointer' : 'default' }}
          onClick={() => { if (isLong) setExpand({ title: headers[ci], content: text }); }}
        >
          {tooltipContent ? <Tooltip content={tooltipContent} hoverOpenDelay={250} placement="auto">{cellContent}</Tooltip> : cellContent}
        </BpCell>
      );
    };
    const columnHeaderCellRenderer = () => <BpColumnHeaderCell name={headers[ci]} />;
    return <BpColumn key={`col-${ci}`} cellRenderer={cellRenderer} columnHeaderCellRenderer={columnHeaderCellRenderer} />;
  });

  return (
    <div style={{ overflowX: 'auto', margin: '6px 0' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Button small minimal icon="cog" onClick={() => setShowCols(true)} title="Columns & display" />
          {total > pageSize && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <Button small minimal icon="double-chevron-left" onClick={() => setPage(0)} disabled={safePage === 0} />
              <Button small minimal icon="chevron-left" onClick={() => setPage(p => Math.max(0, p - 1))} disabled={safePage === 0} />
              <span style={{ fontSize: 12 }}>{start + 1}–{end} of {total}</span>
              <Button small minimal icon="chevron-right" onClick={() => setPage(p => Math.min(pageCount - 1, p + 1))} disabled={safePage >= pageCount - 1} />
              <Button small minimal icon="double-chevron-right" onClick={() => setPage(pageCount - 1)} disabled={safePage >= pageCount - 1} />
            </div>
          )}
        </div>
        <Button small minimal icon="download" onClick={downloadCSV} text="CSV" />
      </div>
      <BpTable
        numRows={pageRows.length}
        columnWidths={colWidths}
        onColumnWidthChanged={(idx, next) => {
          if (typeof idx === 'number' && typeof next === 'number') {
            if (idx > 2000 && next < 200) { const t = idx; idx = next; next = t; }
            const col = visIdx[idx];
            if (col !== undefined && Number.isFinite(next) && next > 40) setWidthByCol((prev) => ({ ...prev, [col]: next }));
          }
        }}
        enableGhostCells={false}
        enableRowHeader={false}
        defaultRowHeight={28}
      >
        {columns}
      </BpTable>

      <Dialog isOpen={showCols} onClose={() => setShowCols(false)} title="Visible Columns">
        <div style={{ padding: 12, display: 'flex', gap: 10, flexDirection: 'column' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <label style={{ fontSize: 12 }}>Truncate at</label>
            <input type="number" className="bp5-input bp5-small" min={20} max={1000} value={truncateAt}
              onChange={e => setTruncateAt(Math.max(20, Math.min(1000, Number(e.target.value) || 120)))} style={{ width: 90 }} />
            <span style={{ fontSize: 12, color: '#6b7280' }}>(characters)</span>
          </div>
          {headers.map((h, i) => (
            <label key={`opt-${i}`} className="bp5-control bp5-checkbox">
              <input type="checkbox" checked={visible.has(i)} onChange={() => toggleColumn(i)} />
              <span className="bp5-control-indicator" />
              {h}
            </label>
          ))}
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Button small intent="primary" onClick={() => setShowCols(false)}>Close</Button>
          </div>
        </div>
      </Dialog>

      <Dialog isOpen={!!expand} onClose={() => setExpand(null)} title={expand?.title || 'Content'} style={{ width: '70vw', minWidth: 480 }}>
        <div
          style={{ padding: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
          dangerouslySetInnerHTML={{
            __html: rewriteSandboxHrefInHTML(
              renderMarkdownCellHTML(rewriteSandboxMarkdownLinks(expand ? expand.content : '', generatedFiles)),
              generatedFiles
            )
          }}
        />
      </Dialog>
    </div>
  );
}

function renderPipeTable(body = '', generatedFiles = []) {
  const fence = describeFence('markdown', body);
  const { headers, rows, aligns } = fence.table || { headers: [], rows: [], aligns: [] };
  return <FencedPipeTable headers={headers} rows={rows} aligns={aligns} generatedFiles={generatedFiles} />;
}

// ── Main component ──

function RichContent({ content = '', generatedFiles = [] }) {
  const textNorm = normalizeBrokenMarkdownLayout(String(content || '').replace(/\r\n/g, '\n'));
  const descriptors = React.useMemo(() => describeContent(textNorm), [textNorm]);
  const forgeDataBlocks = React.useMemo(() => {
    const blocks = [];
    for (const part of descriptors) {
      if (part?.kind !== 'fence' || !isForgeDataFence(part.fence)) continue;
      try {
        blocks.push(validateForgeDataBlock(parseForgeFenceBody(part.fence.body)));
      } catch (_) {}
    }
    return blocks;
  }, [descriptors]);

  if (!descriptors.length) return <span>&nbsp;</span>;

  const out = [];
  let idx = 0;
  for (const part of descriptors) {
    if (part.kind === 'text') {
      const chunk = normalizeBrokenMarkdownLayout(String(part.value || ''));
      if (chunk) {
        const streamingForgeFence = detectStreamingForgeFenceText(chunk);
        if (streamingForgeFence === FORGE_UI_FENCE) {
          out.push(<ForgeFenceLoading key={`forge-ui-loading-text-${idx++}`} label="Building UI" />);
        } else if (streamingForgeFence === FORGE_DATA_FENCE) {
          out.push(<ForgeFenceLoading key={`forge-data-loading-text-${idx++}`} label="Setting datasources" />);
        } else {
          out.push(<MinimalText key={`seg-${idx++}`} text={chunk} generatedFiles={generatedFiles} />);
        }
      }
      continue;
    }
    if (part.kind === 'table') {
      out.push(<div key={`table-${idx++}`} style={{ overflowX: 'auto', margin: '6px 0' }}>{renderPipeTable(part.raw, generatedFiles)}</div>);
      continue;
    }

    const fence = part.fence;
    const body = fence.body;
    if (isForgeDataFence(fence)) {
      if (hasTrailingOpenForgeFence(textNorm, FORGE_DATA_FENCE)) {
        out.push(<ForgeFenceLoading key={`forge-data-loading-${idx++}`} label="Setting datasources" />);
      }
      continue;
    }
    if (isForgeUIFence(fence)) {
      try {
        const payload = parseForgeFenceBody(body);
        out.push(<ForgeUIFence key={`forge-ui-${idx++}`} payload={payload} dataBlocks={forgeDataBlocks} />);
      } catch (_) {
        if (hasTrailingOpenForgeFence(textNorm, FORGE_UI_FENCE)) {
          out.push(<ForgeFenceLoading key={`forge-ui-loading-${idx++}`} label="Building UI" />);
        } else {
          out.push(
            <div key={`forge-ui-error-${idx++}`} style={{ color: '#c23030', fontSize: 13 }}>
              Invalid forge-ui block
            </div>
          );
        }
      }
      continue;
    }
    switch (fence.renderer) {
      case 'chart':
        out.push(<ChartSpecPanel key={`chart-${idx++}`} spec={fence.chartSpec} />);
        break;
      case 'pipeTable':
        out.push(<div key={`table-${idx++}`} style={{ overflowX: 'auto', margin: '6px 0' }}>{renderPipeTable(body, generatedFiles)}</div>);
        break;
      case 'mermaid':
        out.push(
          <div key={`mermaid-${idx++}`} className="app-rich-mermaid">
            <Mermaid code={body} />
          </div>
        );
        break;
      case 'markdown': {
        const html = rewriteSandboxHrefInHTML(
          renderMarkdownBlock(rewriteSandboxMarkdownLinks(body, generatedFiles)),
          generatedFiles
        );
        out.push(<div key={`md-${idx++}`} className="app-rich-markdown" dangerouslySetInnerHTML={{ __html: html }} />);
        break;
      }
      default:
        out.push(
          <div key={`code-${idx++}`} style={{ borderRadius: 8, overflow: 'hidden', margin: '6px 0' }}>
            <CodeBlock value={body} language={fence.lang} height={'auto'} />
          </div>
        );
        break;
    }
  }

  return <div className="app-rich-content">{out}</div>;
}

export default React.memo(RichContent, (a, b) => (
  (a.content || '') === (b.content || '')
  && generatedFileKey(a.generatedFiles) === generatedFileKey(b.generatedFiles)
));

// Re-export parseFences from SDK for consumers that import from this module.
export { parseFences, describeContent } from 'agently-core-ui-sdk';
