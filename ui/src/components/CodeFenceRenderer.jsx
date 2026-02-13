// CodeFenceRenderer – detects ```lang fenced code blocks and renders them
// with Forge's Editor in read-only, scrollable mode. Falls back to minimal
// inline formatting for non-code text.

import React from 'react';
import CodeBlock from './CodeBlock.jsx';
import Mermaid from './Mermaid.jsx';
import { Button, Dialog } from '@blueprintjs/core';
import { Table as BpTable, Column as BpColumn, Cell as BpCell, ColumnHeaderCell as BpColumnHeaderCell } from '@blueprintjs/table';
import { findNextPipeTableBlock } from './markdownTableUtils.js';

// Use Editor from forge/components directly (consistent with other imports like Chat).

function MinimalText({ text = '' }) {
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
  const withItalic = withBold.replace(/\*(.*?)\*/g, '<em>$1</em>');
  const withLinks  = withItalic.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  const html = withLinks.replace(/\n/g, '<br/>');
  // eslint-disable-next-line react/no-danger
  return <div className="prose max-w-full text-sm" dangerouslySetInnerHTML={{ __html: html }} />;
}

// Renders fenced 'markdown' code as preformatted HTML with clickable markdown links only.
// It preserves monospaced layout while converting [label](url) to <a> anchors.
function MarkdownLinksPre({ body = '' }) {
  const escaped = String(body || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  const withLinks = escaped.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
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

// Enhanced fenced table with toolbar, column chooser, pagination and expandable cells
function FencedPipeTable({ headers = [], rows = [], aligns = [] }) {
  const pageSize = 40;
  const [visible, setVisible] = React.useState(() => new Set(headers.map((_, i) => i)));
  const [page, setPage] = React.useState(0);
  const [showCols, setShowCols] = React.useState(false);
  const [expand, setExpand] = React.useState(null);
  const [truncateAt, setTruncateAt] = React.useState(100);

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
  const colWidths = visIdx.map(i => {
    const p = allMaxLens[i] / totalLens;
    return Math.max(80, Math.round(p * baseWidthPx));
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
      return (
        <BpCell style={{ textAlign: align, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', cursor: isLong ? 'pointer' : 'default' }}
                onClick={() => { if (isLong) setExpand({ title: headers[ci], content: text }); }}>
          {display}
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
          <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{expand ? expand.content : ''}</pre>
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
    const cellRenderer = (rowIndex) => (
      <BpCell style={{ textAlign: align, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {rows[rowIndex]?.[ci] ?? ''}
      </BpCell>
    );
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

function CodeFenceRenderer({ text = '' }) {
  // Normalize newlines and run fence detection in two passes (with and without language hint)
  const textNorm = String(text || '').replace(/\r\n/g, '\n');
  const trimmed = textNorm.trimStart();

  // 0) If content is already HTML (<pre>, <code>, <table>), render it as-is
  if (/^</.test(trimmed) && /<(pre|code|table)\b/i.test(trimmed)) {
    // eslint-disable-next-line react/no-danger
    return (
      <div style={{ width: '60vw', overflowX: 'auto' }}>
        <div className="prose max-w-full text-sm" dangerouslySetInnerHTML={{ __html: textNorm }} />
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
  while ((m = pattern.exec(textNorm)) !== null) {
    const [full, langRaw, body] = m;
    const start = m.index;
    const end = start + full.length;
    if (start > lastIndex) {
      const plain = textNorm.slice(lastIndex, start);
      out.push(<MinimalText key={`t-${idx++}`} text={plain} />);
    }
    const lang = languageHint(langRaw);
    fenceCount += 1;
    // If this fenced block looks like a Markdown pipe table, render as a scrollable HTML table
    if ((lang === 'markdown' || lang === 'md' || lang === 'plaintext') && looksLikePipeTable(body)) {
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
      out.push(
        <div key={`e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
          {isMarkdownCode
            ? <MarkdownLinksPre body={body} />
            : <CodeBlock value={body} language={lang} height={'auto'} />}
        </div>
      );
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
        out.push(<MinimalText key={`pt-pre-${idx++}`} text={textNorm.slice(cursor, block.start)} />);
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
        out.push(<MinimalText key={`pt-tail-${idx++}`} text={textNorm.slice(cursor)} />);
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
          if (chunk) out.push(<MinimalText key={`ms-t-${idx++}`} text={chunk} />);
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
          if ((lang === 'markdown' || lang === 'md' || lang === 'plaintext') && looksLikePipeTable(body)) {
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
            out.push(
              <div key={`ms-e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
                {isMarkdownCode
                  ? <MarkdownLinksPre body={body} />
                  : <CodeBlock value={body} language={lang} height={'auto'} />}
              </div>
            );
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
              ? <MarkdownLinksPre body={body} />
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
          parts.push(<MinimalText key={`pre-${idx++}`} text={before} />);
        }
        const isMarkdownCode = (lang === 'markdown' || lang === 'md');
        parts.push(
          <div key={`e3-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0', padding: isMarkdownCode ? '8px 10px' : 0 }}>
            {isMarkdownCode
              ? <MarkdownLinksPre body={body} />
              : <CodeBlock value={body} language={lang} height={'auto'} />}
          </div>
        );
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
        out.push(<MinimalText key={`t-${idx++}`} text={plain} />);
      }
      fenceCount += 1;
      if (looksLikePipeTable(body)) {
        out.push(
          <div key={`table2-${idx++}`} style={{ width: '60vw', overflowX: 'auto', margin: '6px 0' }}>
            {renderPipeTable(body)}
          </div>
        );
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
        out.push(<MinimalText key={`t2-${idx++}`} text={textNorm.slice(lastIndex)} />);
      }
      return <div style={{ width: '60vw', overflowX: 'auto' }}>{out}</div>;
    }
  }
  if (lastIndex < textNorm.length) {
    out.push(<MinimalText key={`t-${idx++}`} text={textNorm.slice(lastIndex)} />);
  }
  // If we rendered at least one fence, wrap in a wider container to expand the bubble.
  if (fenceCount > 0) {
    return <div style={{ width: '60vw', overflowX: 'auto' }}>{out}</div>;
  }
  return <>{out}</>;
}
export default React.memo(CodeFenceRenderer, (a, b) => (a.text || '') === (b.text || ''));
