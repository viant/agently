import React from 'react';
import { renderMarkdownBlock } from 'agently-core-ui-sdk';

// Elicitation prose is Markdown, not an executable rich-content/Forge fence.
export default function ElicitationMessage({ message = '' }) {
  if (!String(message || '').trim()) return null;
  return <div
    className="app-rich-prose elicitation-message"
    style={{ marginBottom: 12, overflowWrap: 'anywhere', maxHeight: 280, overflow: 'auto' }}
    dangerouslySetInnerHTML={{ __html: renderMarkdownBlock(String(message)) }}
  />;
}
