import React, { useMemo, useState } from 'react';
import { Button } from '@blueprintjs/core';

function resolveConversationID(context, message) {
  const fromMessage = String(message?.conversationId || '').trim();
  if (fromMessage) return fromMessage;
  const form = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
  return String(form?.id || '').trim();
}

function readQueued(message) {
  return Array.isArray(message?.queuedTurns) ? message.queuedTurns : [];
}

export default function SteerQueue({ message, context }) {
  const queue = readQueued(message);
  const [busyId, setBusyId] = useState('');
  const [error, setError] = useState('');
  const [expanded, setExpanded] = useState(false);
  const [editingId, setEditingId] = useState('');
  const [draftContent, setDraftContent] = useState('');
  const isRunning = !!message?.running;
  const chat = context?.services?.chat;
  const conversationID = resolveConversationID(context, message);

  const visible = useMemo(() => (expanded ? queue : queue.slice(0, 3)), [queue, expanded]);

  if (queue.length === 0) {
    return null;
  }

  const run = async (turnID, action) => {
    if (!chat || !conversationID || !turnID) return;
    setBusyId(turnID);
    setError('');
    try {
      await action();
    } catch (err) {
      setError(String(err?.message || err || 'queue action failed'));
    } finally {
      setBusyId('');
    }
  };

  return (
    <section className="steer-queue" data-testid="steer-queue-card">
      <div className="steer-queue-header">
        <div className="steer-queue-title-wrap">
          <div className="steer-queue-title">{queue.length} queued {queue.length === 1 ? 'request' : 'requests'}</div>
          <div className="steer-queue-subtitle">Latest user turns waiting in queue</div>
        </div>
        {queue.length > 3 ? (
          <Button small minimal onClick={() => setExpanded((value) => !value)}>
            {expanded ? 'Show less' : `Show all (${queue.length})`}
          </Button>
        ) : null}
      </div>

      <div className="steer-queue-list">
        {visible.map((item, index) => {
          const id = String(item?.id || '');
          const pending = id && busyId === id;
          return (
            <article className="steer-queue-item" key={id || index}>
              <div className="steer-queue-preview">
                {String(item?.preview || '').trim() || id}
              </div>
              {editingId === id ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 8 }}>
                  <input
                    value={draftContent}
                    onChange={(event) => setDraftContent(String(event?.target?.value || ''))}
                    style={{
                      flex: 1,
                      minWidth: 0,
                      border: '1px solid var(--gray4, #d0d7de)',
                      borderRadius: 8,
                      padding: '6px 8px',
                      fontSize: 12,
                    }}
                  />
                  <Button
                    small
                    disabled={pending}
                    onClick={() => {
                      const next = String(draftContent || '').trim();
                      if (!next) return;
                      void run(id, async () => {
                        await chat.editQueuedTurn?.({
                          context,
                          conversationID,
                          turnID: id,
                          content: next,
                        });
                        setEditingId('');
                        setDraftContent('');
                      });
                    }}
                  >
                    Save
                  </Button>
                  <Button
                    small
                    minimal
                    disabled={pending}
                    onClick={() => {
                      setEditingId('');
                      setDraftContent('');
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              ) : null}
              <div className="steer-queue-actions">
                <Button
                  small
                  disabled={!isRunning || pending}
                  onClick={() => run(id, () => chat.forceSteerQueuedTurn?.({ context, conversationID, turnID: id }))}
                >
                  Steer
                </Button>
                <Button
                  small
                  minimal
                  icon="arrow-up"
                  disabled={pending}
                  onClick={() => run(id, () => chat.moveQueuedTurn?.({ context, conversationID, turnID: id, direction: 'up' }))}
                />
                <Button
                  small
                  minimal
                  icon="arrow-down"
                  disabled={pending}
                  onClick={() => run(id, () => chat.moveQueuedTurn?.({ context, conversationID, turnID: id, direction: 'down' }))}
                />
                <Button
                  small
                  minimal
                  icon="edit"
                  disabled={pending}
                  onClick={() => {
                    setEditingId(id);
                    setDraftContent(String(item?.preview || '').trim());
                  }}
                />
                <Button
                  small
                  minimal
                  icon="trash"
                  disabled={pending}
                  onClick={() => run(id, () => chat.cancelQueuedTurnByID?.({ context, conversationID, turnID: id }))}
                />
              </div>
            </article>
          );
        })}
      </div>

      {error ? <div className="steer-queue-error">{error}</div> : null}
    </section>
  );
}
