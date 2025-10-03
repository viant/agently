// CodeFenceRenderer – detects ```lang fenced code blocks and renders them
// with Forge's Editor in read-only, scrollable mode. Falls back to minimal
// inline formatting for non-code text.

import React from 'react';
import CodeBlock from './CodeBlock.jsx';

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
    out.push(
      <div key={`e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
        <CodeBlock value={body} language={lang} height={'auto'} />
      </div>
    );
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
          out.push(
            <div key={`ms-e-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
              <CodeBlock value={body} language={lang} height={'auto'} />
            </div>
          );
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
      out.push(
        <div key={`e2-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
          <CodeBlock value={body} language={'plaintext'} height={'auto'} />
        </div>
      );
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
