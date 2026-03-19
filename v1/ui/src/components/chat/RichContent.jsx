import React from 'react';
import {
  ResponsiveContainer,
  CartesianGrid,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  LineChart,
  Line,
  AreaChart,
  Area,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  ScatterChart,
  Scatter
} from 'recharts';
import Mermaid from './Mermaid';

function escapeHTML(value = '') {
  return String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function resolveHref(url = '') {
  const raw = String(url || '').trim();
  if (!raw) return '#';
  if (raw.startsWith('/')) return raw;
  if (/^(https?:\/\/|mailto:|tel:)/i.test(raw)) return raw;
  return '#';
}

function renderMarkdownInline(value = '') {
  const escaped = escapeHTML(value);
  const withHeadings = escaped
    .replace(/^######\s+(.+)$/gm, '<h6>$1</h6>')
    .replace(/^#####\s+(.+)$/gm, '<h5>$1</h5>')
    .replace(/^####\s+(.+)$/gm, '<h4>$1</h4>')
    .replace(/^###\s+(.+)$/gm, '<h3>$1</h3>')
    .replace(/^##\s+(.+)$/gm, '<h2>$1</h2>')
    .replace(/^#\s+(.+)$/gm, '<h1>$1</h1>');
  const withCode = withHeadings.replace(/`([^`\n]+?)`/g, '<code>$1</code>');
  const withBold = withCode.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
  const withItalic = withBold
    .replace(/\*(.*?)\*/g, '<em>$1</em>')
    .replace(/(^|[^\w])_([^_\n]+)_/g, '$1<em>$2</em>');
  const withLinks = withItalic.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, url) => {
    const href = resolveHref(url);
    return `<a href="${href}" target="_blank" rel="noopener noreferrer">${label}</a>`;
  });
  return withLinks.replace(/\n/g, '<br/>');
}

export function parseFences(content = '') {
  const text = String(content || '');
  const result = [];
  const pattern = /```([a-zA-Z0-9_-]*)\r?\n([\s\S]*?)```/g;
  let index = 0;
  let match;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > index) {
      result.push({ kind: 'text', value: text.slice(index, match.index) });
    }
    result.push({
      kind: 'fence',
      lang: String(match[1] || '').trim().toLowerCase(),
      body: String(match[2] || '')
    });
    index = pattern.lastIndex;
  }
  if (index < text.length) {
    const tail = text.slice(index);
    const openFence = tail.match(/^```([a-zA-Z0-9_-]*)\r?\n([\s\S]*)$/);
    if (openFence) {
      result.push({
        kind: 'fence',
        lang: String(openFence[1] || '').trim().toLowerCase(),
        body: String(openFence[2] || '')
      });
    } else {
      result.push({ kind: 'text', value: tail });
    }
  }
  return result;
}

function isObject(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function inferStringKey(row = {}) {
  const keys = Object.keys(row || {});
  return keys.find((key) => typeof row[key] === 'string') || keys[0] || 'x';
}

function inferNumberKeys(row = {}, deny = []) {
  const blocked = new Set(Array.isArray(deny) ? deny : []);
  return Object.keys(row || {}).filter((key) => !blocked.has(key) && typeof row[key] === 'number');
}

function parseChartSpec(lang = '', body = '') {
  const language = String(lang || '').toLowerCase();
  if (!['chart', 'json', 'javascript', 'js', 'markdown', 'md', 'plaintext'].includes(language)) return null;

  const raw = String(body || '').trim();
  if (!(raw.startsWith('{') || raw.startsWith('['))) return null;
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (_) {
    return null;
  }

  if (!isObject(parsed)) return null;
  if (isObject(parsed.chart) && Array.isArray(parsed.data)) {
    return parsed;
  }
  if (typeof parsed.type === 'string' && isObject(parsed.data) && Array.isArray(parsed.data.labels) && Array.isArray(parsed.data.datasets)) {
    const labels = parsed.data.labels;
    const dataset = parsed.data.datasets[0] || {};
    const values = Array.isArray(dataset.data) ? dataset.data : [];
    const rows = labels.map((label, idx) => ({
      label: String(label),
      value: Number(values[idx] ?? 0)
    }));
    return {
      chart: {
        type: String(parsed.type || 'bar').toLowerCase(),
        x: { key: 'label' },
        y: [{ key: 'value' }]
      },
      data: rows,
      title: parsed.title || dataset.label || ''
    };
  }
  if (typeof parsed.type === 'string' && Array.isArray(parsed.data)) {
    const row = isObject(parsed.data[0]) ? parsed.data[0] : {};
    const xKey = String(parsed?.x?.field || parsed?.xKey || inferStringKey(row));
    const yKeys = Array.isArray(parsed?.y?.fields) ? parsed.y.fields.map(String).filter(Boolean) : [];
    const inferred = yKeys.length > 0 ? yKeys : inferNumberKeys(row, [xKey]);
    return {
      chart: { type: String(parsed.type || 'line').toLowerCase(), x: { key: xKey }, y: inferred.map((key) => ({ key })) },
      data: parsed.data,
      title: parsed.title || ''
    };
  }
  return null;
}

function normalizeSpec(spec = {}) {
  const chart = isObject(spec?.chart) ? spec.chart : {};
  const data = Array.isArray(spec?.data) ? spec.data : [];
  const type = String(chart?.type || 'line').toLowerCase();
  const xKey = String(chart?.x?.key || 'x');
  const yKeys = Array.isArray(chart?.y) ? chart.y.map((item) => String(item?.key || '')).filter(Boolean) : [];
  return {
    title: String(spec?.title || ''),
    type,
    xKey,
    yKeys,
    data
  };
}

const chartPalette = ['#2563eb', '#ef4444', '#16a34a', '#f59e0b', '#7c3aed', '#0f766e'];

function ChartPanel({ spec }) {
  const normalized = normalizeSpec(spec);
  const { type, xKey, yKeys, data, title } = normalized;
  if (!Array.isArray(data) || data.length === 0) return null;
  const series = yKeys.length > 0 ? yKeys : inferNumberKeys(data[0] || {}, [xKey]);
  if (series.length === 0 && type !== 'pie') return null;

  const chartProps = { margin: { top: 10, right: 20, left: 0, bottom: 10 } };

  return (
    <div className="app-rich-chart">
      {title ? <div className="app-rich-chart-title">{title}</div> : null}
      <div className="app-rich-chart-canvas">
        <ResponsiveContainer width="100%" height="100%">
          {type === 'bar' ? (
            <BarChart data={data} {...chartProps}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey={xKey} />
              <YAxis />
              <Tooltip />
              <Legend />
              {series.map((item, idx) => <Bar key={item} dataKey={item} fill={chartPalette[idx % chartPalette.length]} />)}
            </BarChart>
          ) : null}
          {type === 'area' ? (
            <AreaChart data={data} {...chartProps}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey={xKey} />
              <YAxis />
              <Tooltip />
              <Legend />
              {series.map((item, idx) => <Area key={item} type="monotone" dataKey={item} stroke={chartPalette[idx % chartPalette.length]} fill={chartPalette[idx % chartPalette.length]} fillOpacity={0.2} />)}
            </AreaChart>
          ) : null}
          {type === 'pie' ? (
            <PieChart>
              <Tooltip />
              <Legend />
              <Pie data={data} dataKey={series[0] || 'value'} nameKey={xKey} outerRadius={80} label>
                {data.map((_, idx) => <Cell key={`cell-${idx}`} fill={chartPalette[idx % chartPalette.length]} />)}
              </Pie>
            </PieChart>
          ) : null}
          {type === 'scatter' ? (
            <ScatterChart {...chartProps}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey={xKey} />
              <YAxis dataKey={series[0] || 'value'} />
              <Tooltip />
              <Legend />
              <Scatter name={series[0] || 'value'} data={data} fill={chartPalette[0]} />
            </ScatterChart>
          ) : null}
          {!['bar', 'area', 'pie', 'scatter'].includes(type) ? (
            <LineChart data={data} {...chartProps}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey={xKey} />
              <YAxis />
              <Tooltip />
              <Legend />
              {series.map((item, idx) => <Line key={item} type="monotone" dataKey={item} stroke={chartPalette[idx % chartPalette.length]} dot={false} />)}
            </LineChart>
          ) : null}
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function FenceBlock({ lang = '', body = '' }) {
  const language = String(lang || '').trim().toLowerCase();
  if (language === 'mermaid') {
    return <Mermaid code={body} />;
  }
  const chartSpec = parseChartSpec(language, body);
  if (chartSpec) {
    return <ChartPanel spec={chartSpec} />;
  }
  if (language === 'markdown' || language === 'md') {
    const html = renderMarkdownInline(body);
    return <div className="app-rich-markdown" dangerouslySetInnerHTML={{ __html: html }} />;
  }
  return (
    <pre className="app-rich-code">
      <code>{String(body || '')}</code>
    </pre>
  );
}

export default function RichContent({ content = '' }) {
  const parts = React.useMemo(() => parseFences(content), [content]);
  if (!parts.length) return <span>&nbsp;</span>;
  return (
    <div className="app-rich-content">
      {parts.map((part, idx) => {
        if (part.kind === 'fence') {
          return <FenceBlock key={`fence-${idx}`} lang={part.lang} body={part.body} />;
        }
        const html = renderMarkdownInline(part.value);
        return <div key={`text-${idx}`} className="app-rich-prose" dangerouslySetInnerHTML={{ __html: html }} />;
      })}
    </div>
  );
}
