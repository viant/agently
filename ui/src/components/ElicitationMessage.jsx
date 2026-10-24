import React from 'react';
import { renderMarkdownBlock } from 'agently-core-ui-sdk';
import {typographyProps} from 'forge/components';

// Elicitation prose is Markdown, not an executable rich-content/Forge fence.
export default function ElicitationMessage({ message = '', appearance }) {
  if (!String(message || '').trim()) return null;
  return <div
    {...(appearance === 'document' ? typographyProps('body') : {})}
    className={`app-rich-prose elicitation-message${appearance === 'document' ? ' app-request-review-message' : ''}`}
    style={appearance === 'document' ? undefined : { marginBottom: 12, overflowWrap: 'anywhere', maxHeight: 280, overflow: 'auto' }}
    dangerouslySetInnerHTML={{ __html: renderMarkdownBlock(String(message)) }}
  />;
}
