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

function renderHtmlWithCodeBlocks(html = '') {
  const out = [];
  const re = /<pre[^>]*>\s*<code([^>]*)>([\s\S]*?)<\/code>\s*<\/pre>/gi;
  let last = 0; let idx = 0; let m;
  while ((m = re.exec(html)) !== null) {
    const [full, codeAttrs, codeInner] = m;
    const start = m.index;
    if (start > last) {
      const before = html.slice(last, start);
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
  if (last < html.length) {
    const tail = html.slice(last);
    if (tail.trim()) out.push(<div key={`h-${idx++}`} dangerouslySetInnerHTML={{ __html: tail }} />);
  }
  return out.length ? out : [<div key={`h-${idx++}`} dangerouslySetInnerHTML={{ __html: html }} />];
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
                                {renderHtmlWithCodeBlocks(content)}
                              </div>
                            );
                        }
                        return (
                          <div style={{ width: '60vw', overflowX: 'auto' }}>
                            <CodeFenceRenderer text={content} />
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
  if ((a.createdAt || '') !== (b.createdAt || '')) return false;
  return true;
}
export default React.memo(HTMLTableBubble, areEqual);
