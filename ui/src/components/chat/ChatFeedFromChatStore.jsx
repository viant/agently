/**
 * ChatFeedFromChatStore.jsx — renders the chat feed directly from the
 * chatStore's `projectConversation` output.
 *
 * This is the new read path that PR-0 wires into the live chat surface.
 * It consumes one `RenderRow` union kind at a time and delegates to the
 * corresponding small component (UserBubble / IterationRowBlock / etc).
 *
 * Contract references:
 *   - Core principle: the renderer reads merged canonical client state only,
 *     via the projector. There is no legacy-shape input to this component.
 *   - §6.1 one user row per user message entity
 *   - §6.2 one execution-details card per turn
 *   - §6.8 steering: `[u_first, IterationRow, u_rest…]` placement
 */

import React from 'react';

import { useChatProjection } from '../../services/chatStore.js';
import IterationRowBlock from './IterationRowBlock.jsx';
import { rowToLegacyIterationMessage } from './iterationRowLegacyAdapter.js';
import BubbleMessage from './BubbleMessage.jsx';

function UserBubble({ row }) {
  return (
    <div
      data-render-key={row.renderKey}
      data-message-id={row.messageId || ''}
      data-client-request-id={row.clientRequestId || ''}
    >
      <BubbleMessage
        message={{
          id: row.renderKey,
          role: 'user',
          content: row.content,
          createdAt: row.createdAt || '',
          turnId: row.turnId || '',
        }}
      />
    </div>
  );
}

function AssistantBubble({ row }) {
  return (
    <div
      data-render-key={row.renderKey}
      data-message-id={row.messageId || ''}
    >
      <BubbleMessage
        message={{
          id: row.messageId || row.renderKey,
          role: 'assistant',
          content: row.content,
          createdAt: row.createdAt || '',
          turnId: row.turnId || '',
          status: row.status || '',
          mode: row.mode || '',
          interim: 0,
        }}
      />
    </div>
  );
}

function renderRow(row, context) {
  switch (row.kind) {
    case 'user':
      return <UserBubble key={row.renderKey} row={row} />;
    case 'assistant':
      return <AssistantBubble key={row.renderKey} row={row} />;
    case 'iteration':
      return <IterationRowBlock key={row.renderKey} message={rowToLegacyIterationMessage(row)} context={context} />;
    default:
      return null;
  }
}

/**
 * Props:
 *   conversationId — the conversation whose projection to render.
 *   rowsOverride   — optional explicit RenderRow[], used by tests to avoid
 *                    wiring the global chatStore. When omitted, reads from
 *                    `useChatProjection(conversationId)`.
 */
export default function ChatFeedFromChatStore({ conversationId, rowsOverride, context }) {
  const subscribed = useChatProjection(conversationId);
  const rows = rowsOverride !== undefined ? rowsOverride : subscribed;

  if (!Array.isArray(rows) || rows.length === 0) return null;

  return (
    <div className="app-chat-feed" data-source="chatStore">
      {rows.map((row) => renderRow(row, context))}
    </div>
  );
}
