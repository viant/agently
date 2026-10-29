import React, { useContext, useEffect, useId, useRef, useState } from 'react';
import { Dialog, Menu, MenuDivider, MenuItem, Popover, TextArea } from '@blueprintjs/core';
import { Button, typographyProps } from 'forge/components';
import { ThumbsUp, ThumbsDown, ChatText, DotsThree, X } from '@phosphor-icons/react';
import { client } from '../../services/agentlyClient.js';
import { onTurnFeedback } from '../../services/chatStore.js';
import { ConversationViewContext } from '../../context/ConversationViewContext.js';
import './turnFeedback.css';

function FeedbackHeading(props) {
  return <h2 {...typographyProps('section')} {...props}/>;
}

// Saved feedback belongs to the canonical turn. Only an unsaved comment and
// request/presentation state are local to this editor.
export default function TurnFeedback({ conversationId, turnId, feedback, editable }) {
  const { presentation } = useContext(ConversationViewContext);
  const [menu, setMenu] = useState(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const inFlight = useRef(false);
  const root = useRef(null);
  const menuRoot = useRef(null);
  const editor = useRef(null);
  const menuTrigger = useRef(null);
  const editorTrigger = useRef(null);
  const requestFocus = useRef(null);
  const commentId = useId();
  const rating = feedback?.rating || 0;
  const comment = feedback?.comment || '';
  const length = Array.from(draft).length;
  const tooLong = length > 4000;

  // A capability change or authoritative clear must also dismiss local UI.
  useEffect(() => {
    if (!editable || rating === 0) {
      setEditing(false);
      setMenu(null);
    }
  }, [editable, rating]);

  function restoreFocus(action) {
    const active = document.activeElement;
    if (action?.isConnected && !action.disabled &&
        (active === document.body || root.current?.contains(active) ||
          menuRoot.current?.contains(active) || editor.current?.contains(active))) {
      action.focus();
    }
  }

  useEffect(() => {
    if (pending) return;
    const action = requestFocus.current;
    requestFocus.current = null;
    restoreFocus(action);
  }, [pending]);

  if (!editable) return null;

  function openComment(value = comment, trigger = menuTrigger.current) {
    editorTrigger.current = trigger;
    requestFocus.current = null;
    setMenu(null);
    setDraft(value);
    setError('');
    setEditing(true);
  }

  function closeComment() {
    if (inFlight.current) return;
    requestFocus.current = null;
    setEditing(false);
    setError('');
  }

  async function save(nextRating, nextComment, { closeEditor = false, openEditor = false, trigger } = {}) {
    if (inFlight.current) return;
    inFlight.current = true;
    const active = document.activeElement;
    requestFocus.current = trigger ||
      (root.current?.contains(active) || editor.current?.contains(active) ? active : null);
    setMenu(null);
    setPending(true);
    setError('');
    try {
      const saved = nextRating === 0
        ? await client.clearTurnFeedback(conversationId, turnId)
        : await client.setTurnFeedback(conversationId, turnId, { rating: nextRating, comment: nextComment || null });
      onTurnFeedback(conversationId, turnId, saved);
      if (openEditor && saved.rating === -1) {
        openComment(saved.comment || '', trigger);
      } else if (closeEditor || nextRating === 0) {
        setEditing(false);
        if (closeEditor) requestFocus.current = null;
      }
    } catch (_) {
      setError('Could not save feedback. Please try again.');
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  function chooseRating(value) {
    if (value !== rating) save(value, comment, { openEditor: value === -1, trigger: menuTrigger.current });
  }

  function handleMenuKeyDown(event) {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const items = [...event.currentTarget.querySelectorAll('[role="menuitem"]:not([aria-disabled="true"])')];
    const index = items.indexOf(document.activeElement);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
      : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
    event.preventDefault();
    items[next]?.focus();
  }

  function renderMenu(kind) {
    return <Menu {...typographyProps('body')} ulRef={menuRoot} aria-label="Response feedback" onKeyDown={handleMenuKeyDown}>
      <MenuItem text="Good response" icon={<ThumbsUp size={16} weight={rating === 1 ? 'fill' : 'regular'}/>}
        disabled={pending || rating === 1} onClick={() => chooseRating(1)}/>
      <MenuItem text="Bad response" icon={<ThumbsDown size={16} weight={rating === -1 ? 'fill' : 'regular'}/>}
        disabled={pending || rating === -1} onClick={() => chooseRating(-1)}/>
      {kind === 'more' && rating !== 0 ? <>
        <MenuDivider/>
        <MenuItem text={comment ? 'Edit comment' : 'Add comment'} icon={<ChatText size={16}/>}
          disabled={pending} onClick={() => openComment()}/>
      </> : null}
    </Menu>;
  }

  const popoverProps = kind => ({
    content: renderMenu(kind),
    isOpen: menu === kind,
    disabled: pending || editing || (kind === 'rating' && rating !== 0),
    placement: 'bottom-start',
    minimal: true,
    popoverClassName: 'app-turn-feedback-menu',
    shouldReturnFocusOnClose: false,
    onInteraction: open => {
      if (open) menuTrigger.current = root.current?.querySelector('[data-feedback-action="' + kind + '"]');
      setMenu(open ? kind : null);
    },
    onOpened: () => menuRoot.current?.querySelector('[role="menuitem"]:not([aria-disabled="true"])')?.focus(),
    onClosed: () => { if (!menu && !editing && !inFlight.current) restoreFocus(menuTrigger.current); },
  });
  const ratingLabel = rating === 1 ? 'Remove positive feedback' : rating === -1 ? 'Remove negative feedback' : 'Rate response';

  return (
    <div {...typographyProps('small')} ref={root} className={`app-turn-feedback${presentation === 'document' ? ' is-document' : ''}`} data-turn-id={turnId} aria-busy={pending}>
      <div className="app-turn-feedback-actions app-turn-feedback-toolbar" role="group" aria-label="Rate response">
        {/* Native buttons already turn Enter/Space into clicks; omit Popover's duplicate key handler. */}
        <Popover {...popoverProps('rating')} renderTarget={({ref, className, onClick}) =>
          <span ref={ref} className={className}>
            <Button variant="ghost" size="compact" hideLabel data-feedback-action="rating"
              aria-label={ratingLabel} title={ratingLabel} aria-pressed={rating !== 0} disabled={pending || editing}
              aria-haspopup={rating === 0 ? 'menu' : undefined} aria-expanded={rating === 0 ? menu === 'rating' : undefined}
              icon={rating === 1 ? <ThumbsUp aria-hidden="true" size={16} weight="fill"/> : rating === -1
                ? <ThumbsDown aria-hidden="true" size={16} weight="fill"/>
                : <span className="app-turn-feedback-combined-icon" aria-hidden="true"><ThumbsUp size={14}/><ThumbsDown size={14}/></span>}
              onClick={event => rating !== 0 ? save(0, '', {trigger: event.currentTarget}) : onClick(event)}/>
          </span>}/>
        <Popover {...popoverProps('more')} renderTarget={({ref, className, onClick}) =>
          <span ref={ref} className={className}>
            <Button variant="ghost" size="compact" hideLabel data-feedback-action="more"
              aria-label="Feedback options" title="Feedback options" disabled={pending || editing}
              aria-haspopup="menu" aria-expanded={menu === 'more'} onClick={onClick}
              icon={<DotsThree aria-hidden="true" size={20} weight="bold"/>}/>
          </span>}/>
        <span {...typographyProps('metadata')} className="app-turn-feedback-status" role="status">{pending ? 'Saving…' : ''}</span>
      </div>
      {error && !editing ? <p {...typographyProps('metadata')} className="app-turn-feedback-error" role="alert">{error}</p> : null}
      <Dialog isOpen={editing && rating !== 0} className="app-turn-feedback-dialog" title="Feedback" titleTagName={FeedbackHeading}
        isCloseButtonShown={false} onClose={closeComment} canEscapeKeyClose={!pending} canOutsideClickClose={!pending}
        shouldReturnFocusOnClose={false}
        onOpened={() => editor.current?.querySelector('textarea')?.focus()}
        onClosed={() => { setDraft(''); restoreFocus(editorTrigger.current); }}>
        <Button variant="ghost" size="compact" hideLabel className="app-turn-feedback-close" aria-label="Close feedback"
          icon={<X aria-hidden="true" size={16}/>} disabled={pending} onClick={closeComment}/>
        <form ref={editor} className="app-turn-feedback-editor" aria-busy={pending}
          onSubmit={event => { event.preventDefault(); if (!tooLong) save(rating, draft, {closeEditor: true}); }}>
          <label {...typographyProps('label')} htmlFor={commentId}>Comment (optional)</label>
          <TextArea {...typographyProps('body')} id={commentId} value={draft} rows={3} fill disabled={pending}
            aria-invalid={tooLong} aria-describedby={commentId + '-limit' + (error ? ' ' + commentId + '-error' : '')}
            onChange={event => setDraft(event.target.value)}/>
          <span {...typographyProps('metadata')} id={commentId + '-limit'}>{length}/4000 characters</span>
          {error ? <p {...typographyProps('metadata')} id={commentId + '-error'} className="app-turn-feedback-error" role="alert">{error}</p> : null}
          <div className="app-turn-feedback-actions app-turn-feedback-dialog-actions">
            <Button variant="ghost" size="compact" disabled={pending} onClick={closeComment}>Cancel</Button>
            <Button variant="primary" size="compact" type="submit" disabled={pending || tooLong}>Save comment</Button>
            <span {...typographyProps('metadata')} role="status">{pending ? 'Saving…' : ''}</span>
          </div>
        </form>
      </Dialog>
    </div>
  );
}
