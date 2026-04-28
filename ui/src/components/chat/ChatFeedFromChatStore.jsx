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
import { isStreamDebugEnabled } from '../../services/debugFlags';
import IterationRowBlock from './IterationRowBlock.jsx';
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
      return <IterationRowBlock key={row.renderKey} iterationRow={row} context={context} />;
    default:
      return null;
  }
}

function latestTurnRowIndex(rows = []) {
  const result = new Map();
  (Array.isArray(rows) ? rows : []).forEach((row, index) => {
    const turnId = String(row?.turnId || '').trim();
    if (!turnId) return;
    if (row?.kind !== 'iteration' && row?.kind !== 'assistant') return;
    result.set(turnId, index);
  });
  return result;
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

  React.useEffect(() => {
    if (!isStreamDebugEnabled()) return;
    try {
      console.log('[chat-projection]', {
        ts: new Date().toISOString(),
        conversationId: String(conversationId || '').trim(),
        rowCount: Array.isArray(rows) ? rows.length : 0,
        rows: (Array.isArray(rows) ? rows : []).map((row) => ({
          kind: row?.kind,
          renderKey: row?.renderKey,
          turnId: row?.turnId,
          lifecycle: row?.lifecycle,
          status: row?.status,
          contentHead: String(row?.content || '').slice(0, 120),
          rounds: Array.isArray(row?.rounds) ? row.rounds.length : undefined,
        })),
      });
    } catch (_) {}
  }, [conversationId, rows]);

  if (!Array.isArray(rows) || rows.length === 0) return null;
  const lastIndexByTurn = latestTurnRowIndex(rows);

  return (
    <div className="app-chat-feed" data-source="chatStore">
      {rows.map((row, index) => {
        if (row?.kind !== 'iteration') {
          return renderRow(row, context);
        }
        const turnId = String(row?.turnId || '').trim();
        const suppressBubble = !!turnId && (lastIndexByTurn.get(turnId) ?? index) > index;
        return (
          <IterationRowBlock
            key={row.renderKey}
            iterationRow={row}
            context={context}
            suppressBubble={suppressBubble}
          />
        );
      })}
    </div>
  );
}
