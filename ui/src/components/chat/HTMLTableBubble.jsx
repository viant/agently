import React from 'react';
import { Icon } from '@blueprintjs/core';
import { format as formatDate } from 'date-fns';
import CodeBlock from '../CodeBlock.jsx';
import CodeFenceRenderer from '../CodeFenceRenderer.jsx';
import { getLogger } from 'forge/utils/logger';
const log = getLogger('agently');

function decodeHTML(str = '') {
  return String(str)
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'");
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

function resolveSandboxGeneratedFileHref(url = '', generatedFiles = []) {
  const href = String(url || '').trim();
  if (!href || !/^sandbox:\//i.test(href)) return href;
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

function rewriteSandboxHrefInHTML(html = '', generatedFiles = []) {
  return String(html || '').replace(/href=(["'])(sandbox:[^"']+)\1/gi, (m, q, url) => {
    const href = resolveSandboxGeneratedFileHref(url, generatedFiles);
    return `href=${q}${href}${q}`;
  });
}

function extractLang(attrs = '') {
  const m = /class\s*=\s*"([^"]+)"/i.exec(attrs) || /class\s*=\s*'([^']+)'/i.exec(attrs);
  const cls = (m && m[1]) ? m[1] : '';
  const parts = cls.split(/\s+/);
  for (const c of parts) {
    const low = c.toLowerCase();
    if (low.startsWith('language-')) return low.substring('language-'.length);
    if (low.startsWith('lang-')) return low.substring('lang-'.length);
  }
  return 'plaintext';
}

function renderHtmlWithCodeBlocks(html = '', generatedFiles = []) {
  const normalizedHTML = rewriteSandboxHrefInHTML(html, generatedFiles);
  const out = [];
  const re = /<pre[^>]*>\s*<code([^>]*)>([\s\S]*?)<\/code>\s*<\/pre>/gi;
  let last = 0; let idx = 0; let m;
  while ((m = re.exec(normalizedHTML)) !== null) {
    const [full, codeAttrs, codeInner] = m;
    const start = m.index;
    if (start > last) {
      const before = normalizedHTML.slice(last, start);
      if (before.trim()) {
        out.push(<div key={`h-${idx++}`} dangerouslySetInnerHTML={{ __html: before }} />);
      }
    }
    const lang = extractLang(codeAttrs || '');
    const code = decodeHTML(codeInner || '').replace(/^\n/, '');
    out.push(
      <div key={`cb-${idx++}`} style={{ border: '1px solid var(--light-gray2)', borderRadius: 4, margin: '6px 0' }}>
        <CodeBlock value={code} language={lang} height={'auto'} />
      </div>
    );
    last = start + full.length;
  }
  if (last < normalizedHTML.length) {
    const tail = normalizedHTML.slice(last);
    if (tail.trim()) out.push(<div key={`h-${idx++}`} dangerouslySetInnerHTML={{ __html: tail }} />);
  }
  return out.length ? out : [<div key={`h-${idx++}`} dangerouslySetInnerHTML={{ __html: normalizedHTML }} />];
}

function HTMLTableBubble({message, context}) {
    const role = String(message?.role || '').toLowerCase();
    const avatarColour = role === 'user' ? 'var(--blue4)'
        : role === 'assistant' ? 'var(--light-gray4)'
        : 'var(--orange3)';
    const iconName = role === 'assistant' ? 'chat' : role === 'tool' ? 'wrench' : 'person';
    const bubbleClass = role === 'user' ? 'chat-bubble chat-user'
        : role === 'assistant' ? 'chat-bubble chat-bot'
        : 'chat-bubble chat-tool';

    return (
        <div className={`chat-row ${role}`}>
            <div style={{display:'flex', alignItems:'center'}}>
                <div className="avatar" style={{background: avatarColour, display:'flex', alignItems:'center', justifyContent:'center'}}>
                    <Icon icon={iconName} color="var(--black)" size={12}/>
                </div>
                <div className={bubbleClass} data-ts={(function(){ try { const d = new Date(message.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })()}>
                    {/* no per-message delete in UI */}
                    {/* Render HTML with CodeBlock replacement; for plain text, reuse CodeFenceRenderer */}
                    {(() => {
                        const content = String(message?.content || '');
                        if (content.trim().startsWith('<')) {
                            return (
                              <div className="prose max-w-full text-sm" style={{ width: '60vw', overflowX: 'auto' }}>
                                {renderHtmlWithCodeBlocks(content, message?.generatedFiles || [])}
                              </div>
                            );
                        }
                        return (
                          <div style={{ width: '60vw', overflowX: 'auto' }}>
                            <CodeFenceRenderer text={content} generatedFiles={message?.generatedFiles || []} />
                          </div>
                        );
                    })()}
                    {(() => {
                        const files = Array.isArray(message?.generatedFiles) ? message.generatedFiles : [];
                        if (!files.length) return null;
                        return (
                            <div style={{ marginTop: 10, paddingTop: 8, borderTop: '1px solid var(--light-gray3)' }}>
                                <div style={{ fontSize: 12, opacity: 0.8, marginBottom: 4 }}>Generated files</div>
                                {files.map((f) => {
                                    const id = String(f?.id || '').trim();
                                    if (!id) return null;
                                    const filename = String(f?.filename || 'generated-file.bin').trim();
                                    const status = String(f?.status || '').trim().toLowerCase();
                                    const sizeBytes = Number.isFinite(Number(f?.sizeBytes)) ? Number(f.sizeBytes) : null;
                                    const sizeLabel = (sizeBytes !== null && sizeBytes >= 0) ? ` (${sizeBytes} bytes)` : '';
                                    const href = `/v1/api/generated-files/${encodeURIComponent(id)}/download`;
                                    const disabled = status === 'failed' || status === 'expired';
                                    return (
                                        <div key={id} style={{ marginBottom: 2 }}>
                                            {disabled ? (
                                                <span style={{ opacity: 0.75 }}>{filename}{sizeLabel} [{status}]</span>
                                            ) : (
                                                <a href={href} style={{ textDecoration: 'underline' }}>{filename}{sizeLabel}</a>
                                            )}
                                        </div>
                                    );
                                })}
                            </div>
                        );
                    })()}
                </div>
            </div>
        </div>
    );
}
function areEqual(prev, next) {
  const a = prev.message || {};
  const b = next.message || {};
  if (a.id !== b.id) return false;
  if ((a.content || '') !== (b.content || '')) return false;
  if (JSON.stringify(a.generatedFiles || []) !== JSON.stringify(b.generatedFiles || [])) return false;
  if ((a.createdAt || '') !== (b.createdAt || '')) return false;
  return true;
}
export default React.memo(HTMLTableBubble, areEqual);
