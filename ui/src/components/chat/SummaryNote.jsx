// SummaryNote.jsx – collapsed system note that holds a conversation summary.

import React from 'react';
import { Icon } from '@blueprintjs/core';
import { format as formatDate } from 'date-fns';

// Minimal markdown → HTML renderer identical to ExecutionBubble copy.
function renderMarkdown(md = '') {
    const escaped = md
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');

    const withCodeBlocks = escaped.replace(/```([\s\S]*?)```/g, (match, p1) => `<pre><code>${p1}</code></pre>`);
    const withInlineCode = withCodeBlocks.replace(/`([^`]+?)`/g, '<code>$1</code>');
    const withBold   = withInlineCode.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
    const withItalic = withBold.replace(/\*(.*?)\*/g, '<em>$1</em>');
    const withStrike = withItalic.replace(/~~(.*?)~~/g, '<del>$1</del>');
    const withLinks  = withStrike.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
    return withLinks.replace(/\n/g, '<br/>');
}

export default function SummaryNote({ message }) {
    const preview = (message.content || '').split(/\n/)[0].slice(0, 120);

    const avatarColour = 'var(--light-gray4)';

    return (
        <div className="chat-row system"> {/* alignment flex row */}
            <div style={{ display: 'flex', alignItems: 'flex-start' }}>
                <div className="avatar" style={{ background: avatarColour }}>
                    <Icon icon="document" color="var(--black)" size={12} />
                </div>
                <details className="chat-bubble chat-bot" data-ts={(function(){ try { const d = new Date(message.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })()}
                         style={{ maxWidth: '60vw' }}>
                    <summary className="cursor-pointer text-xs text-blue-500">
                        Conversation summary – {preview}{message.content.length > 120 ? '…' : ''}
                    </summary>
                    <div className="prose max-w-full text-sm mt-2" dangerouslySetInnerHTML={{ __html: renderMarkdown(message.content) }} />
                </details>
            </div>
        </div>
    );
}
