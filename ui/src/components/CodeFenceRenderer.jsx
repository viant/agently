// CodeFenceRenderer – detects ```lang fenced code blocks and renders them
// with Forge's Editor in read-only, scrollable mode. Falls back to minimal
// inline formatting for non-code text.

import React from 'react';
import CodeBlock from './CodeBlock.jsx';
import { Button } from '@blueprintjs/core';
import { Table as BpTable, Column as BpColumn, Cell as BpCell, ColumnHeaderCell as BpColumnHeaderCell } from '@blueprintjs/table';

// Use Editor from forge/components directly (consistent with other imports like Chat).

function MinimalText({ text = '' }) {
  // Escape and apply minimal inline formatting: inline code, bold, italic, links, newlines
  const escaped = String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
  const withInline = escaped.replace(/`([^`]+?)`/g, '<code>$1</code>');
  const withBold   = withInline.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
  const withItalic = withBold.replace(/\*(.*?)\*/g, '<em>$1</em>');
  const withLinks  = withItalic.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  const html = withLinks.replace(/\n/g, '<br/>');
  // eslint-disable-next-line react/no-danger
  return <div className="prose max-w-full text-sm" dangerouslySetInnerHTML={{ __html: html }} />;
}

function languageHint(lang = '') {
  const v = String(lang || '').trim().toLowerCase();
  if (!v) return 'plaintext';
  // Normalize a few common aliases
  if (v === 'js') return 'javascript';
  if (v === 'ts') return 'typescript';
  if (v === 'yml') return 'yaml';
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

// Renders a Markdown pipe table as a scrollable HTML table with basic styling.
function renderPipeTable(body = '') {
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

export default function CodeFenceRenderer({ text = '' }) {
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
    } else {
      out.push(
        <div key={`e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
          <CodeBlock value={body} language={lang} height={'auto'} />
        </div>
      );
    }
    lastIndex = end;
  }
  // If no fences matched with language hints, try fallback (no explicit lang)
  if (fenceCount === 0) {
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
          } else {
            out.push(
              <div key={`ms-e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
                <CodeBlock value={body} language={lang} height={'auto'} />
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
      return (
        <div style={{ width: '60vw', overflowX: 'auto' }}>
          <div style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
            <CodeBlock value={body} language={lang} height={'auto'} />
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
        parts.push(
          <div key={`e3-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
            <CodeBlock value={body} language={lang} height={'auto'} />
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
      } else {
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
