// CodeFenceRenderer – detects ```lang fenced code blocks and renders them
// with Forge's Editor in read-only, scrollable mode. Falls back to minimal
// inline formatting for non-code text.

import React from 'react';
import CodeBlock from './CodeBlock.jsx';
import Mermaid from './Mermaid.jsx';
import { Button, Dialog, Tooltip } from '@blueprintjs/core';
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
import { findNextPipeTableBlock } from './markdownTableUtils.js';

// Use Editor from forge/components directly (consistent with other imports like Chat).

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
    .map((f) => {
      const id = String(f?.id || '').trim();
      const filename = String(f?.filename || '').trim().toLowerCase();
      const status = String(f?.status || '').trim().toLowerCase();
      return `${id}|${filename}|${status}`;
    })
    .filter(Boolean)
    .join(',');
}

function normalizeSandboxFilename(url = '') {
  let raw = String(url || '').trim();
  if (!raw) return '';
  if (!/^sandbox:\//i.test(raw)) return '';
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
  if (!href) return href;
  if (!/^sandbox:\//i.test(href)) return href;
  const filename = normalizeSandboxFilename(href);
  if (!filename) return href;
  const want = filename.toLowerCase();
  const files = Array.isArray(generatedFiles) ? generatedFiles : [];
  const match = files.find((f) => {
    const id = String(f?.id || '').trim();
    const name = String(f?.filename || '').trim().toLowerCase();
    return !!id && name === want;
  });
  if (!match || !match.id) return href;
  return `/v1/api/generated-files/${encodeURIComponent(String(match.id).trim())}/download`;
}

function renderMarkdownLinks(input = '', generatedFiles = []) {
  return String(input || '').replace(/\[([^\]]+)\]\(([^\)]+)\)/g, (m, label, url) => {
    const href = resolveMarkdownHref(url, generatedFiles);
    return `<a href="${escapeHTMLAttr(href)}" target="_blank" rel="noopener noreferrer">${label}</a>`;
  });
}

function rewriteSandboxHrefInHTML(html = '', generatedFiles = []) {
  return String(html || '').replace(/href=(["'])(sandbox:[^"']+)\1/gi, (m, q, url) => {
    const href = resolveMarkdownHref(url, generatedFiles);
    return `href=${q}${escapeHTMLAttr(href)}${q}`;
  });
}

function MinimalText({ text = '', generatedFiles = [] }) {
  // Escape and apply minimal inline formatting: inline code, bold, italic, links, newlines
  const escaped = String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  const renderFractions = (input = '') => {
    // Render basic LaTeX-style \frac{a}{b} and \dfrac{a}{b} into HTML spans.
    return String(input).replace(/\\(?:d)?frac\{([^{}]+)\}\{([^{}]+)\}/g, (m, num, den) => {
      return `<span class="md-fraction"><span class="md-fraction-num">${num}</span><span class="md-fraction-den">${den}</span></span>`;
    });
  };
  const withHeadings = escaped
    .replace(/^######\s+(.+)$/gm, '<h6>$1</h6>')
    .replace(/^#####\s+(.+)$/gm, '<h5>$1</h5>')
    .replace(/^####\s+(.+)$/gm, '<h4>$1</h4>')
    .replace(/^###\s+(.+)$/gm, '<h3>$1</h3>')
    .replace(/^##\s+(.+)$/gm, '<h2>$1</h2>')
    .replace(/^#\s+(.+)$/gm, '<h1>$1</h1>');
  const withFractions = renderFractions(withHeadings);
  const withInline = withFractions.replace(/`([^`]+?)`/g, '<code>$1</code>');
  const withBold   = withInline.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
  const withStarItalic = withBold.replace(/\*(.*?)\*/g, '<em>$1</em>');
  // Underscore italics (avoid mid-word underscores)
  const withUnderscoreItalic = withStarItalic.replace(/(^|[^\w])_([^_\n]+)_/g, '$1<em>$2</em>');
  const withLinks  = renderMarkdownLinks(withUnderscoreItalic, generatedFiles);
  const html = withLinks.replace(/\n/g, '<br/>');
  // eslint-disable-next-line react/no-danger
  return <div className="prose max-w-full text-sm" dangerouslySetInnerHTML={{ __html: html }} />;
}

// Renders fenced 'markdown' code as preformatted HTML with clickable markdown links only.
// It preserves monospaced layout while converting [label](url) to <a> anchors.
function MarkdownLinksPre({ body = '', generatedFiles = [] }) {
  const escaped = String(body || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  const withLinks = renderMarkdownLinks(escaped, generatedFiles);
  // eslint-disable-next-line react/no-danger
  return <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', margin: 0 }}><code dangerouslySetInnerHTML={{ __html: withLinks }} /></pre>;
}

function languageHint(lang = '') {
  const v = String(lang || '').trim().toLowerCase();
  if (!v) return 'plaintext';
  // Normalize a few common aliases
  if (v === 'js') return 'javascript';
  if (v === 'ts') return 'typescript';
  if (v === 'yml') return 'yaml';
  if (v === 'sequence' || v === 'sequencediagram') return 'mermaid';
  return v;
}

function isObject(v) {
  return !!v && typeof v === 'object' && !Array.isArray(v);
}

function parseChartSpecFromFence(lang = '', body = '') {
  const v = String(lang || '').toLowerCase();
  const raw = String(body || '').trim();
  if (!(v === 'json' || v === 'javascript' || v === 'js' || v === 'plaintext' || v === 'md' || v === 'markdown')) return null;
  if (!raw.startsWith('{') && !raw.startsWith('[')) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!isObject(parsed)) return null;
    const chart = parsed.chart;
    const data = Array.isArray(parsed.data) ? parsed.data : (Array.isArray(parsed.rows) ? parsed.rows : null);
    if (!isObject(chart) || !Array.isArray(data)) return null;
    const type = String(chart.type || '').trim().toLowerCase();
    if (!type) return null;
    return parsed;
  } catch (_) {
    return null;
  }
}

function normalizeChartSpec(spec = {}) {
  const chart = isObject(spec.chart) ? spec.chart : {};
  const type = String(chart.type || 'line').toLowerCase();
  const data = Array.isArray(spec.data) ? spec.data : (Array.isArray(spec.rows) ? spec.rows : []);
  const xKey = String(chart?.x?.key || spec.xKey || 'x');
  const seriesKey = chart?.series?.key ? String(chart.series.key) : '';
  const yArr = Array.isArray(chart?.y) ? chart.y : [];
  const yKeys = yArr.map((v) => String(v?.key || '')).filter(Boolean);
  const valueKey = String(chart?.valueKey || spec.valueKey || yKeys[0] || 'value');
  const palette = Array.isArray(spec?.options?.palette) ? spec.options.palette : ['#2563eb', '#ef4444', '#16a34a', '#f59e0b', '#9333ea', '#06b6d4'];
  return { type, data, xKey, seriesKey, yKeys, valueKey, palette, title: String(spec.title || '') };
}

function buildChartSeries(normalized) {
  const { data, xKey, seriesKey, yKeys, valueKey } = normalized;
  if (!Array.isArray(data) || data.length === 0) return { rows: [], series: [] };

  // Long form: x + series + single value
  if (seriesKey) {
    const map = new Map();
    const order = [];
    for (const row of data) {
      const x = row?.[xKey];
      const s = String(row?.[seriesKey] ?? '');
      if (!s) continue;
      if (!order.includes(s)) order.push(s);
      const k = String(x);
      if (!map.has(k)) map.set(k, { [xKey]: x });
      map.get(k)[s] = Number(row?.[valueKey] ?? 0);
    }
    return { rows: Array.from(map.values()), series: order };
  }

  // Wide form: x + multiple y keys
  const keys = (yKeys.length ? yKeys : Object.keys(data[0] || {}).filter((k) => k !== xKey && typeof data[0][k] === 'number'));
  return { rows: data, series: keys };
}

function ChartSpecTable({ rows = [] }) {
  const headers = React.useMemo(() => {
    const keys = new Set();
    for (const r of rows) Object.keys(r || {}).forEach((k) => keys.add(k));
    return Array.from(keys);
  }, [rows]);

  const columns = headers.map((h, ci) => {
    const cellRenderer = (rowIndex) => {
      const raw = rows[rowIndex]?.[h];
      return <BpCell>{String(raw ?? '')}</BpCell>;
    };
    const columnHeaderCellRenderer = () => <BpColumnHeaderCell name={h} />;
    return <BpColumn key={`c-${ci}`} cellRenderer={cellRenderer} columnHeaderCellRenderer={columnHeaderCellRenderer} />;
  });

  if (!rows.length || !headers.length) {
    return <div style={{ fontSize: 12, color: 'var(--dark-gray3)' }}>No data</div>;
  }
  const widths = headers.map(() => 160);
  return (
    <div style={{ width: '60vw', overflowX: 'auto' }}>
      <BpTable numRows={rows.length} columnWidths={widths} enableGhostCells={false} enableRowHeader={false} defaultRowHeight={28}>
        {columns}
      </BpTable>
    </div>
  );
}

function ChartSpecPanel({ spec = {} }) {
  const [mode, setMode] = React.useState('chart');
  const n = React.useMemo(() => normalizeChartSpec(spec), [spec]);
  const { rows, series } = React.useMemo(() => buildChartSeries(n), [n]);
  const { type, xKey, palette, title } = n;

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
    if (type === 'area') {
      return (
        <AreaChart data={rows}>
          {chartCommon}
          {series.map((s, i) => <Area key={s} type="monotone" dataKey={s} stroke={palette[i % palette.length]} fill={palette[i % palette.length]} fillOpacity={0.2} />)}
        </AreaChart>
      );
    }
    if (type === 'bar' || type === 'stacked_bar') {
      const stacked = type === 'stacked_bar';
      return (
        <BarChart data={rows}>
          {chartCommon}
          {series.map((s, i) => <Bar key={s} dataKey={s} fill={palette[i % palette.length]} stackId={stacked ? 'a' : undefined} />)}
        </BarChart>
      );
    }
    if (type === 'scatter') {
      const y = series[0];
      return (
        <ScatterChart>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey={xKey} />
          <YAxis dataKey={y} />
          <RcTooltip />
          <RcLegend />
          <Scatter name={y} data={rows} fill={palette[0]} />
        </ScatterChart>
      );
    }
    if (type === 'pie') {
      const nameKey = n.seriesKey || xKey;
      const valueKey = series[0];
      return (
        <PieChart>
          <RcTooltip />
          <RcLegend />
          <Pie data={n.data || []} dataKey={valueKey} nameKey={nameKey} outerRadius={110} label>
            {(n.data || []).map((_, i) => <Cell key={`cell-${i}`} fill={palette[i % palette.length]} />)}
          </Pie>
        </PieChart>
      );
    }
    return (
      <LineChart data={rows}>
        {chartCommon}
        {series.map((s, i) => <Line key={s} type="monotone" dataKey={s} stroke={palette[i % palette.length]} dot={false} />)}
      </LineChart>
    );
  };

  return (
    <div style={{ width: '60vw', border: '1px solid var(--light-gray2)', borderRadius: 6, padding: 8, margin: '6px 0' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <div style={{ fontSize: 12, color: 'var(--dark-gray3)' }}>{title || 'Chart'}</div>
        <div style={{ display: 'flex', gap: 6 }}>
          <Button small minimal={mode !== 'chart'} intent={mode === 'chart' ? 'primary' : 'none'} onClick={() => setMode('chart')}>Chart</Button>
          <Button small minimal={mode !== 'table'} intent={mode === 'table' ? 'primary' : 'none'} onClick={() => setMode('table')}>Table</Button>
        </div>
      </div>
      {mode === 'chart' ? (
        <div style={{ width: '100%', height: 320 }}>
          <ResponsiveContainer width="100%" height="100%">
            {renderChart()}
          </ResponsiveContainer>
        </div>
      ) : (
        <ChartSpecTable rows={n.data || []} />
      )}
    </div>
  );
}

// Detects GitHub-flavored Markdown pipe table in a fenced block body.
function looksLikePipeTable(body = '') {
  const lines = String(body || '').split('\n').map(l => l.trim()).filter(l => l.length > 0);
  if (lines.length < 2) return false;
  const header = lines[0];
  const sep = lines[1];
  if (!header.includes('|')) return false;
  // Header row should have pipes separating columns
  // Separator row should be composed of pipes, dashes and optional colons for alignment
  const sepOk = /^\|?\s*[:\-]+(\s*\|\s*[:\-]+)+\s*\|?$/.test(sep);
  if (!sepOk) return false;
  return true;
}

// Parses a simple Markdown pipe table into { headers, rows }
function parsePipeTable(body = '') {
  const lines = String(body || '').split('\n').map(l => l.trim()).filter(l => l.length > 0);
  const headerLine = lines[0];
  const sepLine = lines[1] || '';
  const dataLines = lines.slice(2); // skip separator
  const toCells = (line) => {
    let s = line;
    if (s.startsWith('|')) s = s.slice(1);
    if (s.endsWith('|')) s = s.slice(0, -1);
    return s.split('|').map(c => c.trim());
  };
  const headers = toCells(headerLine);
  // Alignment detection from separator line per GFM tables
  const parseAlign = (seg) => {
    const x = (seg || '').trim();
    const left = x.startsWith(':');
    const right = x.endsWith(':');
    if (left && right) return 'center';
    if (right) return 'right';
    if (left) return 'left';
    return 'left';
  };
  const aligns = (() => {
    let s = sepLine;
    if (s.startsWith('|')) s = s.slice(1);
    if (s.endsWith('|')) s = s.slice(0, -1);
    const segs = s.split('|');
    const arr = segs.map(parseAlign);
    while (arr.length < headers.length) arr.push('left');
    if (arr.length > headers.length) arr.length = headers.length;
    return arr;
  })();
  const rows = dataLines.map(toCells).map(r => {
    if (r.length < headers.length) {
      return r.concat(Array(headers.length - r.length).fill(''));
    } else if (r.length > headers.length) {
      return r.slice(0, headers.length);
    }
    return r;
  });
  return { headers, rows, aligns };
}

function escapeHTMLCell(str = '') {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderMarkdownCellHTML(md = '') {
  const escaped = String(md || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  const withCodeBlocks = escaped.replace(/```([\s\S]*?)```/g, (match, p1) => `<pre><code>${p1}</code></pre>`);
  const withInlineCode = withCodeBlocks.replace(/`([^`]+?)`/g, '<code>$1</code>');
  const withBold = withInlineCode.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
  const withItalic = withBold.replace(/\*(.*?)\*/g, '<em>$1</em>');
  const withStrike = withItalic.replace(/~~(.*?)~~/g, '<del>$1</del>');
  const withLinks = withStrike.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  return withLinks.replace(/\n/g, '<br/>');
}

// Enhanced fenced table with toolbar, column chooser, pagination and expandable cells
function FencedPipeTable({ headers = [], rows = [], aligns = [] }) {
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

  // Estimate column widths (based on all rows for stable layout)
  const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));
  const allMaxLens = headers.map((h, i) => {
    let m = String(h || '').length;
    for (const r of rows) {
      const c = String((r || [])[i] ?? '').length;
      if (c > m) m = c;
    }
    return clamp(m, 4, 48);
  });
  const baseWidthPx = 720;
  const totalLens = visIdx.reduce((acc, i) => acc + allMaxLens[i], 0) || visIdx.length;
  const computedColWidths = visIdx.map(i => {
    const p = allMaxLens[i] / totalLens;
    return Math.max(80, Math.round(p * baseWidthPx));
  });
  const colWidths = visIdx.map((i, j) => {
    const persisted = Number(widthByCol[i]);
    if (Number.isFinite(persisted) && persisted > 40) return persisted;
    return computedColWidths[j];
  });

  const downloadCSV = () => {
    try {
      const escapeCell = (s = '') => {
        const v = String(s ?? '');
        if (/[,"\n]/.test(v)) return '"' + v.replace(/"/g, '""') + '"';
        return v;
      };
      const lines = [];
      lines.push(visIdx.map(i => escapeCell(headers[i])).join(','));
      for (const r of rows) {
        lines.push(visIdx.map(i => escapeCell((r || [])[i] ?? '')).join(','));
      }
      const bom = '\ufeff';
      const csv = bom + lines.join('\n');
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      const ts = new Date().toISOString().replace(/[:.]/g, '-');
      a.href = url;
      a.download = `table-${ts}.csv`;
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
      const html = renderMarkdownCellHTML(display);
      const fullHtml = renderMarkdownCellHTML(text);
      const cellContent = (
        <span dangerouslySetInnerHTML={{ __html: html }} />
      );
      return (
        <BpCell
          style={{ textAlign: align, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', cursor: isLong ? 'pointer' : 'default' }}
          onClick={() => { if (isLong) setExpand({ title: headers[ci], content: text }); }}
        >
          {isLong ? (
            <Tooltip
              content={<div style={{ maxWidth: 640, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }} dangerouslySetInnerHTML={{ __html: fullHtml }} />}
              hoverOpenDelay={250}
              placement="auto"
              interactionKind="hover-target"
            >
              {cellContent}
            </Tooltip>
          ) : cellContent}
        </BpCell>
      );
    };
    const columnHeaderCellRenderer = () => (
      <BpColumnHeaderCell name={headers[ci]} />
    );
    return (
      <BpColumn key={`col-${ci}`} cellRenderer={cellRenderer} columnHeaderCellRenderer={columnHeaderCellRenderer} />
    );
  });

  return (
    <div style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
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
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Button small minimal icon="download" onClick={downloadCSV} text="CSV" />
        </div>
      </div>
      <BpTable
        numRows={pageRows.length}
        columnWidths={colWidths}
        onColumnWidthChanged={(indexOrSize, sizeOrIndex) => {
          let idx = indexOrSize;
          let next = sizeOrIndex;
          if (typeof idx === 'number' && typeof next === 'number') {
            // Blueprint may pass (index, size) in current versions.
            // If values appear reversed, normalize by choosing the small value as index.
            if (idx > 2000 && next < 200) {
              const t = idx;
              idx = next;
              next = t;
            }
            const col = visIdx[idx];
            if (col !== undefined && Number.isFinite(next) && next > 40) {
              setWidthByCol((prev) => ({ ...prev, [col]: next }));
            }
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
            <label style={{ fontSize: 12, color: 'var(--dark-gray3)' }}>Truncate at</label>
            <input type="number" className="bp4-input bp4-small" min={20} max={1000} value={truncateAt}
              onChange={e => setTruncateAt(Math.max(20, Math.min(1000, Number(e.target.value)||120)))} style={{ width: 90 }} />
            <span style={{ fontSize: 12, color: 'var(--dark-gray3)' }}>(characters)</span>
          </div>
          {headers.map((h, i) => (
            <label key={`opt-${i}`} className="bp4-control bp4-checkbox">
              <input type="checkbox" checked={visible.has(i)} onChange={() => toggleColumn(i)} />
              <span className="bp4-control-indicator" />
              {h}
            </label>
          ))}
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Button small intent="primary" onClick={() => setShowCols(false)}>Close</Button>
          </div>
        </div>
      </Dialog>

      <Dialog isOpen={!!expand} onClose={() => setExpand(null)} title={expand?.title || 'Content'} style={{ width: '70vw', minWidth: 480 }}>
        <div style={{ padding: 12 }}>
          <div style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }} dangerouslySetInnerHTML={{ __html: renderMarkdownCellHTML(expand ? expand.content : '') }} />
        </div>
      </Dialog>
    </div>
  );
}

// Wrapper to render the enhanced table from a fenced block body
function renderPipeTable(body = '') {
  const { headers, rows, aligns } = parsePipeTable(body);
  return <FencedPipeTable headers={headers} rows={rows} aligns={aligns} />;
}

// Renders a Markdown pipe table as a scrollable HTML table with basic styling.
function legacyRenderPipeTable(body = '') {
  if (!looksLikePipeTable(body)) return null;
  const { headers, rows, aligns } = parsePipeTable(body);
  // Estimate column widths by max cell length per column (clamped and normalized)
  const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));
  const maxLens = headers.map((h, i) => {
    let m = (h || '').length;
    for (const r of rows) {
      const c = (r[i] || '').length;
      if (c > m) m = c;
    }
    return clamp(m, 4, 48);
  });
  const sum = maxLens.reduce((a, b) => a + b, 0) || headers.length;
  const percents = maxLens.map(v => (v / sum) * 100);
  const baseWidthPx = 720; // approx 60vw target
  const colWidths = percents.map(p => Math.max(80, Math.round((p / 100) * baseWidthPx))); // min 80px per column

  const columns = headers.map((header, ci) => {
    const align = aligns[ci] || 'left';
    const cellRenderer = (rowIndex) => {
      const raw = rows[rowIndex]?.[ci] ?? '';
      const html = renderMarkdownCellHTML(String(raw));
      return (
        <BpCell style={{ textAlign: align, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
          <span dangerouslySetInnerHTML={{ __html: html }} />
        </BpCell>
      );
    };
    const columnHeaderCellRenderer = () => (
      <BpColumnHeaderCell name={header} />
    );
    return (
      <BpColumn key={`col-${ci}`} cellRenderer={cellRenderer} columnHeaderCellRenderer={columnHeaderCellRenderer} />
    );
  });

  const downloadCSV = () => {
    try {
      const escapeCell = (s = '') => {
        const v = String(s ?? '');
        if (/[",\n]/.test(v)) {
          return '"' + v.replace(/"/g, '""') + '"';
        }
        return v;
      };
      const lines = [];
      lines.push(headers.map(escapeCell).join(','));
      for (const r of rows) {
        lines.push((r || []).slice(0, headers.length).map(escapeCell).join(','));
      }
      const bom = '\ufeff'; // help Excel detect UTF-8
      const csv = bom + lines.join('\n');
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      const ts = new Date().toISOString().replace(/[:.]/g, '-');
      a.href = url;
      a.download = `table-${ts}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (_) {}
  };

  return (
    <div style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
      <div style={{ minWidth: Math.max(baseWidthPx, colWidths.reduce((a,b)=>a+b,0)) }}>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 6, marginBottom: 6 }}>
          <Button small minimal icon="download" onClick={downloadCSV} text="CSV" />
        </div>
        <BpTable
          numRows={rows.length}
          columnWidths={colWidths}
          enableGhostCells={false}
          enableRowHeader={false}
          defaultRowHeight={28}
        >
          {columns}
        </BpTable>
      </div>
    </div>
  );
}

function CodeFenceRenderer({ text = '', generatedFiles = [] }) {
  // Normalize newlines and run fence detection in two passes (with and without language hint)
  const textNorm = String(text || '')
    .replace(/\r\n/g, '\n')
    // CHART_SPEC marker is control metadata; do not render it as visible prose.
    .replace(/^\s*<!--\s*CHART_SPEC:v1\s*-->\s*$/gim, '');
  const trimmed = textNorm.trimStart();

  // 0) If content is already HTML (<pre>, <code>, <table>), render it as-is
  if (/^</.test(trimmed) && /<(pre|code|table)\b/i.test(trimmed)) {
    const html = rewriteSandboxHrefInHTML(textNorm, generatedFiles);
    // eslint-disable-next-line react/no-danger
    return (
      <div style={{ width: '60vw', overflowX: 'auto' }}>
        <div className="prose max-w-full text-sm" dangerouslySetInnerHTML={{ __html: html }} />
      </div>
    );
  }
  const pattern = /```\s*([a-zA-Z0-9_+\-]*)\n([\s\S]*?)```/g;
  const fallbackPattern = /```([\s\S]*?)```/g; // any fence without explicit lang
  const out = [];
  let lastIndex = 0;
  let m;
  let idx = 0;
  let fenceCount = 0;
  const pushMarkdownBodyWithTables = (body = '', keyPrefix = 'mdf') => {
    const chunk = String(body || '');
    if (!chunk) return false;
    let cursor = 0;
    let anyTable = false;
    while (true) {
      const block = findNextPipeTableBlock(chunk, cursor);
      if (!block) break;
      anyTable = true;
      if (block.start > cursor) {
        out.push(<MinimalText key={`${keyPrefix}-pre-${idx++}`} text={chunk.slice(cursor, block.start)} generatedFiles={generatedFiles} />);
      }
      const tableBody = chunk.slice(block.start, block.end);
      out.push(
        <div key={`${keyPrefix}-tbl-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
          {renderPipeTable(tableBody)}
        </div>
      );
      cursor = block.end;
    }
    if (anyTable && cursor < chunk.length) {
      out.push(<MinimalText key={`${keyPrefix}-tail-${idx++}`} text={chunk.slice(cursor)} generatedFiles={generatedFiles} />);
    }
    return anyTable;
  };
  const pushPlainWithTables = (plain = '', keyPrefix = 'pt') => {
    const chunk = String(plain || '');
    if (!chunk) return;
    let cursor = 0;
    let anyTable = false;
    while (true) {
      const block = findNextPipeTableBlock(chunk, cursor);
      if (!block) break;
      anyTable = true;
      if (block.start > cursor) {
        out.push(<MinimalText key={`${keyPrefix}-pre-${idx++}`} text={chunk.slice(cursor, block.start)} generatedFiles={generatedFiles} />);
      }
      const body = chunk.slice(block.start, block.end);
      out.push(
        <div key={`${keyPrefix}-tbl-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
          {renderPipeTable(body)}
        </div>
      );
      cursor = block.end;
    }
    if (anyTable) {
      if (cursor < chunk.length) {
        out.push(<MinimalText key={`${keyPrefix}-tail-${idx++}`} text={chunk.slice(cursor)} generatedFiles={generatedFiles} />);
      }
      return;
    }
    out.push(<MinimalText key={`${keyPrefix}-${idx++}`} text={chunk} generatedFiles={generatedFiles} />);
  };
  while ((m = pattern.exec(textNorm)) !== null) {
    const [full, langRaw, body] = m;
    const start = m.index;
    const end = start + full.length;
    if (start > lastIndex) {
      const plain = textNorm.slice(lastIndex, start);
      pushPlainWithTables(plain, 'seg');
    }
    const lang = languageHint(langRaw);
    fenceCount += 1;
    // If this fenced block looks like a Markdown pipe table, render as a scrollable HTML table
    const chartSpec = parseChartSpecFromFence(lang, body);
    if (chartSpec) {
      out.push(<ChartSpecPanel key={`chart-${idx++}`} spec={chartSpec} />);
    } else if ((lang === 'markdown' || lang === 'md' || lang === 'plaintext') && looksLikePipeTable(body)) {
      out.push(
        <div key={`table-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
          {renderPipeTable(body)}
        </div>
      );
    } else if (lang === 'mermaid' || /^\s*(sequenceDiagram|flowchart|graph|classDiagram|stateDiagram)/.test(body)) {
      // Render Mermaid diagrams when the fence language is 'mermaid' or the body starts with a known diagram type
      out.push(
        <div key={`mermaid-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
          <Mermaid code={body} />
        </div>
      );
    } else {
      const isMarkdownCode = (lang === 'markdown' || lang === 'md');
      const renderedMixedMarkdownTable = isMarkdownCode ? pushMarkdownBodyWithTables(body, `md-main-${idx}`) : false;
      if (!renderedMixedMarkdownTable) {
        out.push(
          <div key={`e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
            {isMarkdownCode
              ? <MarkdownLinksPre body={body} generatedFiles={generatedFiles} />
              : <CodeBlock value={body} language={lang} height={'auto'} />}
          </div>
        );
      }
    }
    lastIndex = end;
  }
  // If no fences matched with language hints, try fallback (no explicit lang)
  if (fenceCount === 0) {
    // Detect unfenced Markdown pipe tables within plain prose
    let cursor = 0;
    let anyTable = false;
    while (true) {
      const block = findNextPipeTableBlock(textNorm, cursor);
      if (!block) break;
      anyTable = true;
      if (block.start > cursor) {
        pushPlainWithTables(textNorm.slice(cursor, block.start), 'pt-scan-pre');
      }
      const body = textNorm.slice(block.start, block.end);
      out.push(
        <div key={`pt-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
          {renderPipeTable(body)}
        </div>
      );
      cursor = block.end;
    }
    if (anyTable) {
      if (cursor < textNorm.length) {
        pushPlainWithTables(textNorm.slice(cursor), 'pt-scan-tail');
      }
      return <div style={{ width: '60vw', overflowX: 'auto' }}>{out}</div>;
    }
    // 0a) Manual splitter on triple backticks if present
    const split = textNorm.split('```');
    if (split.length > 1) {
      for (let i = 0; i < split.length; i++) {
        const chunk = split[i];
        if (i % 2 === 0) {
          // prose chunk
          if (chunk) pushPlainWithTables(chunk, 'ms');
        } else {
          // code chunk; first line may be language
          const mLang = /\s*^([a-zA-Z0-9_+\-]*)\n([\s\S]*)/m.exec(chunk);
          let lang = 'plaintext';
          let body = chunk;
          if (mLang) {
            lang = languageHint(mLang[1] || '');
            body = mLang[2] || '';
          }
          fenceCount += 1;
          const chartSpec = parseChartSpecFromFence(lang, body);
          if (chartSpec) {
            out.push(<ChartSpecPanel key={`ms-chart-${idx++}`} spec={chartSpec} />);
          } else if ((lang === 'markdown' || lang === 'md' || lang === 'plaintext') && looksLikePipeTable(body)) {
            out.push(
              <div key={`ms-table-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
                {renderPipeTable(body)}
              </div>
            );
          } else if (lang === 'mermaid' || /^\s*(sequenceDiagram|flowchart|graph|classDiagram|stateDiagram)/.test(body)) {
            out.push(
              <div key={`ms-mermaid-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
                <Mermaid code={body} />
              </div>
            );
          } else {
            const isMarkdownCode = (lang === 'markdown' || lang === 'md');
            const renderedMixedMarkdownTable = isMarkdownCode ? pushMarkdownBodyWithTables(body, `md-split-${idx}`) : false;
            if (!renderedMixedMarkdownTable) {
              out.push(
                <div key={`ms-e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
                  {isMarkdownCode
                    ? <MarkdownLinksPre body={body} generatedFiles={generatedFiles} />
                    : <CodeBlock value={body} language={lang} height={'auto'} />}
                </div>
              );
            }
          }
        }
      }
      return <div style={{ width: '60vw', overflowX: 'auto' }}>{out}</div>;
    }
    // 0b) Heuristic: language label on first line followed by code-like body
    const langLabelMatch = /^(go|js|ts|javascript|typescript|python|java|c|cpp|csharp|rust|yaml|yml|json|sql|sh|bash)\s*\n([\s\S]+)/i.exec(textNorm);
    if (langLabelMatch) {
      const lang = languageHint(langLabelMatch[1]);
      const body = langLabelMatch[2] || '';
      fenceCount = 1;
      const chartSpec = parseChartSpecFromFence(lang, body);
      if (chartSpec) {
        return <ChartSpecPanel spec={chartSpec} />;
      }
      if ((lang === 'markdown' || lang === 'md' || lang === 'plaintext') && looksLikePipeTable(body)) {
        return (
          <div style={{ width: '60vw', overflowX: 'auto' }}>
            {renderPipeTable(body)}
          </div>
        );
      }
      if (lang === 'mermaid' || /^\s*(sequenceDiagram|flowchart|graph|classDiagram|stateDiagram)/.test(body)) {
        return (
          <div style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
            <Mermaid code={body} />
          </div>
        );
      }
      const isMarkdownCode = (lang === 'markdown' || lang === 'md');
      return (
        <div style={{ width: '60vw', overflowX: 'auto' }}>
          <div style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
            {isMarkdownCode
              ? <MarkdownLinksPre body={body} generatedFiles={generatedFiles} />
              : <CodeBlock value={body} language={lang} height={'auto'} />}
          </div>
        </div>
      );
    }

    // 0c) Heuristic: language label appearing later in the text
    if (fenceCount === 0) {
      const anyLang = /(\n|^)\s*(go|js|ts|javascript|typescript|python|java|c|cpp|csharp|rust|yaml|yml|json|sql|sh|bash)\s*\n([\s\S]+)/i.exec(textNorm);
      if (anyLang) {
        const lang = languageHint(anyLang[2]);
        const before = textNorm.slice(0, anyLang.index);
        const body = anyLang[3] || '';
        fenceCount = 1;
        const parts = [];
        if (before.trim()) {
          parts.push(<MinimalText key={`pre-${idx++}`} text={before} generatedFiles={generatedFiles} />);
        }
        const isMarkdownCode = (lang === 'markdown' || lang === 'md');
        const renderedMixedMarkdownTable = isMarkdownCode ? pushMarkdownBodyWithTables(body, `md-anylang-${idx}`) : false;
        if (!renderedMixedMarkdownTable) {
          parts.push(
            <div key={`e3-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
              {isMarkdownCode
                ? <MarkdownLinksPre body={body} />
                : <CodeBlock value={body} language={lang} height={'auto'} />}
            </div>
          );
        }
        return <div style={{ width: '60vw', overflowX: 'auto' }}>{parts}</div>;
      }
    }

    let m2; let scanned = false;
    fallbackPattern.lastIndex = 0;
    while ((m2 = fallbackPattern.exec(textNorm)) !== null) {
      scanned = true;
      const [full, body] = m2;
      const start = m2.index;
      const end = start + full.length;
      if (start > lastIndex) {
        const plain = textNorm.slice(lastIndex, start);
        pushPlainWithTables(plain, 'fb');
      }
      fenceCount += 1;
      if (looksLikePipeTable(body)) {
        out.push(
          <div key={`table2-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
            {renderPipeTable(body)}
          </div>
        );
      } else if (parseChartSpecFromFence('json', body)) {
        out.push(<ChartSpecPanel key={`table2-chart-${idx++}`} spec={parseChartSpecFromFence('json', body)} />);
      } else if (/^\s*(sequenceDiagram|flowchart|graph|classDiagram|stateDiagram)/.test(body)) {
        out.push(
          <div key={`m2-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
            <Mermaid code={body} />
          </div>
        );
      } else {
        // Fallback fence has no language; keep plain code block
        out.push(
          <div key={`e2-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
            <CodeBlock value={body} language={'plaintext'} height={'auto'} />
          </div>
        );
      }
      lastIndex = end;
    }
    if (scanned) {
      // append remaining prose after fallback pass
      if (lastIndex < textNorm.length) {
        pushPlainWithTables(textNorm.slice(lastIndex), 'fb-tail');
      }
      return <div style={{ width: '60vw', overflowX: 'auto' }}>{out}</div>;
    }
  }
  if (lastIndex < textNorm.length) {
    pushPlainWithTables(textNorm.slice(lastIndex), 'tail');
  }
  // If we rendered at least one fence, wrap in a wider container to expand the bubble.
  if (fenceCount > 0) {
    return <div style={{ width: '60vw', overflowX: 'auto' }}>{out}</div>;
  }
  return <>{out}</>;
}
export default React.memo(CodeFenceRenderer, (a, b) => {
  if ((a.text || '') !== (b.text || '')) return false;
  if (generatedFileKey(a.generatedFiles) !== generatedFileKey(b.generatedFiles)) return false;
  return true;
});
