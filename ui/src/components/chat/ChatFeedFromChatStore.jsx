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

import React, { useContext } from 'react';

import { useChatProjection } from '../../services/chatStore.js';
import { isStreamDebugEnabled } from '../../services/debugFlags';
import IterationRowBlock from './IterationRowBlock.jsx';
import ToolFeedDetail from '../ToolFeedDetail.jsx';
import BubbleMessage from './BubbleMessage.jsx';
import MCPUIBubble from './MCPUIBubble.jsx';
import StarterTasks from './StarterTasks.jsx';
import WorkspaceAttachmentCard from './WorkspaceAttachmentCard.jsx';
import { ConversationViewContext } from '../../context/ConversationViewContext.js';
import { resolveWorkspaceAttachmentOwnerIndex } from '../../services/workspaceAttachment.js';

export { resolveWorkspaceAttachmentOwnerIndex } from '../../services/workspaceAttachment.js';

function UserBubble({ row, conversationId = '' }) {
  return (
    <div
      data-render-key={row.renderKey}
      data-message-id={row.messageId || ''}
      data-client-request-id={row.clientRequestId || ''}
    >
      <BubbleMessage
        conversationId={conversationId}
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

function AssistantBubble({ row, conversationId = '', attachment = null }) {
  return (
    <div
      data-render-key={row.renderKey}
      data-message-id={row.messageId || ''}
    >
      <BubbleMessage
        conversationId={conversationId}
        attachment={attachment}
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

function renderRow(row, context, conversationId = '', attachment = null) {
  switch (row.kind) {
    case 'user':
      return <UserBubble key={row.renderKey} row={row} conversationId={conversationId} />;
    case 'assistant':
      return <AssistantBubble key={row.renderKey} row={row} conversationId={conversationId} attachment={attachment} />;
    case 'mcpui':
      return <MCPUIBubble key={row.renderKey} row={row} />;
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
  const viewContext = useContext(ConversationViewContext);
  const subscribed = useChatProjection(conversationId);
  const rows = rowsOverride !== undefined ? rowsOverride : subscribed;
  const conversationForm = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
  const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  const starterTasks = Array.isArray(metaForm?.starterTasks) ? metaForm.starterTasks : [];
  const showStarterTasks = !String(conversationForm?.id || conversationId || '').trim() && starterTasks.length > 0;

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

  if (!Array.isArray(rows) || rows.length === 0) {
    if (!showStarterTasks) return null;
    return (
      <div className="app-chat-feed" data-source="chatStore">
        <StarterTasks
          context={context}
          message={{
            _type: 'starter',
            starterTasks,
            title: 'Start with an agent prompt',
          }}
        />
      </div>
    );
  }
  const lastIndexByTurn = latestTurnRowIndex(rows);
  const workspaceAttachmentOwnerIndex = viewContext?.workspaceVisible
    ? -1
    : resolveWorkspaceAttachmentOwnerIndex(rows, viewContext?.workspaceWindow);
  const workspaceAttachment = workspaceAttachmentOwnerIndex >= 0 ? (
    <WorkspaceAttachmentCard
      workspaceWindow={viewContext.workspaceWindow}
      onOpen={viewContext.onOpenWorkspace}
    />
  ) : null;
  const retryPromptByTurn = new Map();
  rows.forEach((row) => {
    const turnId = String(row?.turnId || '').trim();
    if (row?.kind === 'user' && turnId && String(row?.content || '').trim()) {
      retryPromptByTurn.set(turnId, String(row.content).trim());
    }
  });

  return (
    <div className="app-chat-feed" data-source="chatStore">
      {rows.map((row, index) => {
        const turnId = String(row?.turnId || '').trim();
        const isFinalTurnRepresentation = !!turnId
          && (row?.kind === 'iteration' || row?.kind === 'assistant')
          && (lastIndexByTurn.get(turnId) ?? index) === index;
        const inlineFeed = isFinalTurnRepresentation ? (
          <ToolFeedDetail
            context={context}
            conversationId={conversationId}
            turnId={turnId}
            placement="inline"
            includeAuto={viewContext?.toolFeedDock !== 'right'}
          />
        ) : null;
        if (row?.kind !== 'iteration') {
          const rendered = renderRow(
            row,
            context,
            conversationId,
            index === workspaceAttachmentOwnerIndex ? workspaceAttachment : null
          );
          if (!inlineFeed) return rendered;
          return (
            <React.Fragment key={row.renderKey}>
              {rendered}
              {inlineFeed}
            </React.Fragment>
          );
        }
        const suppressBubble = !!turnId && (lastIndexByTurn.get(turnId) ?? index) > index;
        return (
          <React.Fragment key={row.renderKey}>
            <IterationRowBlock
              iterationRow={row}
              context={context}
              showToolFeedDetail={false}
              suppressBubble={suppressBubble}
              retryPrompt={retryPromptByTurn.get(turnId) || ''}
              attachment={index === workspaceAttachmentOwnerIndex ? workspaceAttachment : null}
            />
            {inlineFeed}
          </React.Fragment>
        );
      })}
    </div>
  );
}
