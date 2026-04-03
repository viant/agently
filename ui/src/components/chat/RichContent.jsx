// RichContent – renders assistant message content with syntax-highlighted code,
// Blueprint tables, Recharts charts, Mermaid diagrams, and inline markdown.
// Parsing and classification logic comes from the SDK's pluggable richContent module.

import React from 'react';
import CodeBlock from './CodeBlock.jsx';
import Mermaid from './Mermaid';
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

// Import all parsing/rendering utilities from SDK
import {
  parseFences,
  languageHint,
  findNextPipeTableBlock,
  looksLikePipeTable,
  parsePipeTable,
  parseChartSpecFromFence,
  normalizeChartSpec,
  buildChartSeries,
  renderMarkdownBlock,
  renderMarkdownCellHTML,
} from 'agently-core-ui-sdk';

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
  // following pipe-table onto one line, e.g. "### Daily Trend | Date | ...".
  // Split that back into a heading line plus the table block.
  value = value.replace(/^(#{1,6}\s+[^|\n][^|\n]*?)\s+(\|.+)$/gm, '$1\n\n$2');

  // Some responses also collapse a heading and the first bullet onto one line,
  // e.g. "## Highlights- Item". Preserve the heading and start the list below.
  value = value.replace(/^(#{1,6}\s+[^\n]+?)(-\s+)/gm, '$1\n\n$2');

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
    if (type === 'pie') return <PieChart><RcTooltip /><RcLegend /><Pie data={n.data || []} dataKey={series[0] || 'value'} nameKey={n.seriesKey || xKey} outerRadius={110} label>{(n.data || []).map((_, i) => <Cell key={`cell-${i}`} fill={palette[i % palette.length]} />)}</Pie></PieChart>;
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
  const { headers, rows, aligns } = parsePipeTable(body);
  return <FencedPipeTable headers={headers} rows={rows} aligns={aligns} generatedFiles={generatedFiles} />;
}

// ── Main component ──

function RichContent({ content = '', generatedFiles = [] }) {
  const textNorm = String(content || '').replace(/\r\n/g, '\n');
  const parts = React.useMemo(() => parseFences(textNorm), [textNorm]);

  if (!parts.length) return <span>&nbsp;</span>;

  const out = [];
  let idx = 0;
  let hasFence = false;

  /** Push prose text, auto-detecting pipe tables within it. */
  const pushPlainWithTables = (text, keyPrefix) => {
    const chunk = normalizeBrokenMarkdownLayout(String(text || ''));
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
      out.push(<div key={`${keyPrefix}-tbl-${idx++}`} style={{ overflowX: 'auto', margin: '6px 0' }}>{renderPipeTable(chunk.slice(block.start, block.end), generatedFiles)}</div>);
      cursor = block.end;
    }
    if (anyTable && cursor < chunk.length) {
      out.push(<MinimalText key={`${keyPrefix}-tail-${idx++}`} text={chunk.slice(cursor)} generatedFiles={generatedFiles} />);
    }
    if (!anyTable) {
      out.push(<MinimalText key={`${keyPrefix}-${idx++}`} text={chunk} generatedFiles={generatedFiles} />);
    }
  };

  for (const part of parts) {
    if (part.kind === 'text') {
      pushPlainWithTables(part.value, 'seg');
      continue;
    }

    // Fenced block
    hasFence = true;
    const lang = languageHint(part.lang);
    const body = part.body;

    // Chart spec
    const chartSpec = parseChartSpecFromFence(lang, body);
    if (chartSpec) {
      out.push(<ChartSpecPanel key={`chart-${idx++}`} spec={chartSpec} />);
      continue;
    }

    // Pipe table inside markdown/plaintext fence
    if ((lang === 'markdown' || lang === 'md' || lang === 'plaintext') && looksLikePipeTable(body)) {
      out.push(<div key={`table-${idx++}`} style={{ overflowX: 'auto', margin: '6px 0' }}>{renderPipeTable(body, generatedFiles)}</div>);
      continue;
    }

    // Mermaid
    if (lang === 'mermaid' || /^\s*(sequenceDiagram|flowchart|graph|classDiagram|stateDiagram)/.test(body)) {
      out.push(
        <div key={`mermaid-${idx++}`} className="app-rich-mermaid">
          <Mermaid code={body} />
        </div>
      );
      continue;
    }

    // Markdown fence → render as prose
    if (lang === 'markdown' || lang === 'md') {
      const html = rewriteSandboxHrefInHTML(
        renderMarkdownBlock(rewriteSandboxMarkdownLinks(body, generatedFiles)),
        generatedFiles
      );
      out.push(<div key={`md-${idx++}`} className="app-rich-markdown" dangerouslySetInnerHTML={{ __html: html }} />);
      continue;
    }

    // Code block with syntax highlighting
    out.push(
      <div key={`code-${idx++}`} style={{ borderRadius: 8, overflow: 'hidden', margin: '6px 0' }}>
        <CodeBlock value={body} language={lang} height={'auto'} />
      </div>
    );
  }

  return <div className="app-rich-content">{out}</div>;
}

export default React.memo(RichContent, (a, b) => (
  (a.content || '') === (b.content || '')
  && generatedFileKey(a.generatedFiles) === generatedFileKey(b.generatedFiles)
));

// Re-export parseFences from SDK for consumers that import from this module.
export { parseFences } from 'agently-core-ui-sdk';
