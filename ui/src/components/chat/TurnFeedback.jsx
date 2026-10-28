import React, { useEffect, useId, useRef, useState } from 'react';
import { TextArea } from '@blueprintjs/core';
import { Button, typographyProps } from 'forge/components';
import { ThumbsUp, ThumbsDown, ChatText, CaretDown } from '@phosphor-icons/react';
import { client } from '../../services/agentlyClient.js';
import { onTurnFeedback } from '../../services/chatStore.js';

// Saved feedback belongs to the canonical turn. Only an unsaved comment and
// request state are local to this editor.
export default function TurnFeedback({ conversationId, turnId, feedback, editable }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const inFlight = useRef(false);
  const root = useRef(null);
  const wasEditing = useRef(false);
  const requestFocus = useRef(null);
  const commentId = useId();
  const rating = feedback?.rating || 0;
  const comment = feedback?.comment || '';
  const commentPreview = comment.replace(/\s+/gu, ' ').trim();
  const tooLong = Array.from(draft).length > 4000;

  useEffect(() => {
    if (wasEditing.current && !editing &&
        (document.activeElement === document.body || root.current?.contains(document.activeElement))) {
      const action = root.current?.querySelector('[data-feedback-comment-action]') || root.current?.querySelector('button');
      action?.focus();
    }
    wasEditing.current = editing;
  }, [editing]);

  useEffect(() => {
    if (pending) return;
    const action = requestFocus.current;
    requestFocus.current = null;
    if (action?.isConnected && !action.disabled &&
        (document.activeElement === document.body || root.current?.contains(document.activeElement))) {
      action.focus();
    }
  }, [pending]);

  if (!editable) return null;

  async function save(nextRating, nextComment, closeEditor) {
    if (inFlight.current) return;
    inFlight.current = true;
    requestFocus.current = root.current?.contains(document.activeElement) ? document.activeElement : null;
    setPending(true);
    setError('');
    try {
      const saved = nextRating === 0
        ? await client.clearTurnFeedback(conversationId, turnId)
        : await client.setTurnFeedback(conversationId, turnId, { rating: nextRating, comment: nextComment || null });
      onTurnFeedback(conversationId, turnId, saved);
      if (closeEditor || nextRating === 0) {
        setEditing(false);
        setDraft('');
      }
    } catch (_) {
      setError('Could not save feedback. Please try again.');
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  return (
    <div {...typographyProps('small')} ref={root} className="app-turn-feedback" data-turn-id={turnId} aria-busy={pending}>
      <div className="app-turn-feedback-actions app-turn-feedback-toolbar" role="group" aria-label="Rate response">
        <Button variant="ghost" size="compact" hideLabel aria-label="Good response" title={rating === 1 ? 'Remove positive feedback' : 'Good response'}
          aria-pressed={rating === 1} disabled={pending} icon={<ThumbsUp aria-hidden="true" size={16} weight={rating === 1 ? 'fill' : 'regular'} />}
          onClick={() => save(rating === 1 ? 0 : 1, comment, false)} />
        <Button variant="ghost" size="compact" hideLabel aria-label="Bad response" title={rating === -1 ? 'Remove negative feedback' : 'Bad response'}
          aria-pressed={rating === -1} disabled={pending} icon={<ThumbsDown aria-hidden="true" size={16} weight={rating === -1 ? 'fill' : 'regular'} />}
          onClick={() => save(rating === -1 ? 0 : -1, comment, false)} />
        {rating !== 0 && !editing ? <Button variant="ghost" size="compact" disabled={pending} data-feedback-comment-action
          className={comment ? 'app-turn-feedback-preview-action' : undefined} hideLabel={!comment}
          aria-label={comment ? 'Edit comment' : 'Add comment'} title={comment ? 'Edit comment' : 'Add comment'}
          icon={<ChatText aria-hidden="true" size={16} />}
          onClick={() => { setDraft(comment); setEditing(true); setError(''); }}>
          {comment ? <>
            <span {...typographyProps('small')} className="app-turn-feedback-preview">{commentPreview}</span>
            <CaretDown aria-hidden="true" size={14} />
          </> : null}
        </Button> : null}
        <span {...typographyProps('metadata')} className="app-turn-feedback-status" role="status">{pending ? 'Saving…' : ''}</span>
      </div>
      {editing && rating !== 0 ? (
        <form className="app-turn-feedback-editor" onSubmit={(event) => { event.preventDefault(); if (!tooLong) save(rating, draft, true); }}>
          <label {...typographyProps('label')} htmlFor={commentId}>Feedback comment (optional)</label>
          <TextArea {...typographyProps('body')} id={commentId} value={draft} rows={3} fill autoFocus disabled={pending}
            aria-invalid={tooLong} aria-describedby={`${commentId}-limit`}
            onChange={(event) => setDraft(event.target.value)} />
          <span {...typographyProps('metadata')} id={`${commentId}-limit`} className="app-turn-feedback-status">{Array.from(draft).length}/4000 characters</span>
          <div className="app-turn-feedback-actions">
            <Button variant="secondary" size="compact" type="submit" disabled={pending || tooLong}>Save comment</Button>
            <Button variant="ghost" size="compact" disabled={pending} onClick={() => { setEditing(false); setDraft(''); setError(''); }}>Cancel</Button>
          </div>
        </form>
      ) : null}
      {error ? <p {...typographyProps('metadata')} className="app-turn-feedback-error" role="alert">{error}</p> : null}
    </div>
  );
}
