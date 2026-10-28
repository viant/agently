// @vitest-environment jsdom
import React from 'react';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import TurnFeedback from './TurnFeedback.jsx';
import { __resetAll, onTranscript, onTurnFeedback, useChatProjection } from '../../services/chatStore.js';
import { client } from '../../services/agentlyClient.js';

vi.mock('../../services/agentlyClient.js', () => ({ client: {setTurnFeedback:vi.fn(), clearTurnFeedback:vi.fn()} }));

function Feed() {
  const row = useChatProjection('c').find(value => value.kind === 'iteration');
  return <TurnFeedback conversationId="c" turnId="t" feedback={row?.feedback} editable={row?.feedbackEditable} />;
}

beforeEach(() => {
  __resetAll(); vi.clearAllMocks();
  onTranscript('c',{conversationId:'c',turns:[{turnId:'t',status:'completed',feedbackEditable:true,
    feedback:{rating:0,comment:null,updatedAt:null},assistant:{final:{messageId:'m',content:'Hello'}}}]});
  client.setTurnFeedback.mockImplementation(async (_c,_t,input) => ({...input,updatedAt:'2026-10-24T12:00:00Z'}));
  client.clearTurnFeedback.mockResolvedValue({rating:0,comment:null,updatedAt:null});
});
afterEach(cleanup);

describe('TurnFeedback', () => {
  it('saves immediately, cancels drafts, preserves saved comment on flip and clears both', async () => {
    render(<Feed />);
    expect(screen.getByRole('button',{name:'Good response'}).getAttribute('data-forge-variant')).toBe('ghost');
    fireEvent.click(screen.getByRole('button',{name:'Good response'}));
    const addComment = await screen.findByRole('button',{name:'Add comment'});
    expect(addComment.textContent).toBe('');
    expect(addComment.title).toBe('Add comment');
    fireEvent.click(screen.getByRole('button',{name:'Add comment'}));
    expect(screen.getByLabelText('Feedback comment (optional)').getAttribute('data-forge-type')).toBe('body');
    expect(document.activeElement).toBe(screen.getByLabelText('Feedback comment (optional)'));
    fireEvent.change(screen.getByLabelText('Feedback comment (optional)'),{target:{value:'Unsaved'}});
    fireEvent.click(screen.getByRole('button',{name:'Cancel'}));
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Add comment'}));
    expect(client.setTurnFeedback).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button',{name:'Add comment'}));
    expect(screen.getByLabelText('Feedback comment (optional)').value).toBe('');
    fireEvent.change(screen.getByLabelText('Feedback comment (optional)'),{target:{value:'Pomocne 👍'}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await screen.findByRole('button',{name:'Edit comment'});
    fireEvent.click(screen.getByRole('button',{name:'Bad response'}));
    await waitFor(() => expect(screen.getByRole('button',{name:'Bad response'}).getAttribute('aria-pressed')).toBe('true'));
    expect(client.setTurnFeedback).toHaveBeenLastCalledWith('c','t',{rating:-1,comment:'Pomocne 👍'});
    fireEvent.click(screen.getByRole('button',{name:'Bad response'}));
    await waitFor(() => expect(screen.getByRole('button',{name:'Bad response'}).getAttribute('aria-pressed')).toBe('false'));
    expect(client.clearTurnFeedback).toHaveBeenCalledWith('c','t');
    expect(screen.queryByText('Pomocne 👍')).toBeNull();
  });

  it('blocks duplicate writes and keeps the saved vote and draft after a failure', async () => {
    let reject;
    client.setTurnFeedback.mockReturnValueOnce(new Promise((_resolve,fail) => {reject=fail;}));
    render(<Feed />);
    const good = screen.getByRole('button',{name:'Good response'});
    good.focus();
    fireEvent.click(good);
    fireEvent.click(screen.getByRole('button',{name:'Bad response'}));
    expect(client.setTurnFeedback).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button',{name:'Bad response'}).disabled).toBe(true);
    // Browsers blur a focused button when the pending state disables it.
    good.blur();
    reject(new Error('offline'));
    await screen.findByRole('alert');
    expect(document.activeElement).toBe(good);
    expect(screen.getByRole('button',{name:'Good response'}).getAttribute('aria-pressed')).toBe('false');
    fireEvent.click(screen.getByRole('button',{name:'Good response'}));
    await screen.findByRole('button',{name:'Add comment'});
    fireEvent.click(screen.getByRole('button',{name:'Add comment'}));
    fireEvent.change(screen.getByLabelText('Feedback comment (optional)'),{target:{value:'keep my draft'}});
    client.setTurnFeedback.mockRejectedValueOnce(new Error('offline'));
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await screen.findByRole('alert');
    expect(screen.getByLabelText('Feedback comment (optional)').value).toBe('keep my draft');
    expect(screen.getByRole('button',{name:'Good response'}).getAttribute('aria-pressed')).toBe('true');
  });

  it('restores focus after a successful write unless the user moved elsewhere', async () => {
    let finish;
    client.setTurnFeedback.mockImplementation((_c, _t, input) => new Promise(resolve => {
      finish = () => resolve({...input, updatedAt:'2026-10-24T12:00:00Z'});
    }));
    render(<><button>Other control</button><Feed /></>);
    const good = screen.getByRole('button',{name:'Good response'});
    good.focus();
    fireEvent.click(good);
    good.blur();
    finish();
    await screen.findByRole('button',{name:'Add comment'});
    expect(document.activeElement).toBe(good);

    const bad = screen.getByRole('button',{name:'Bad response'});
    bad.focus();
    fireEvent.click(bad);
    bad.blur();
    const other = screen.getByRole('button',{name:'Other control'});
    other.focus();
    finish();
    await waitFor(() => expect(bad.getAttribute('aria-pressed')).toBe('true'));
    expect(document.activeElement).toBe(other);
  });

  it('opens the complete saved comment from a single-line preview and cancels only the draft', async () => {
    const original = 'Pomocne 👍\n\n  Zażółć gęślą jaźń.\tWięcej szczegółów.';
    const preview = 'Pomocne 👍 Zażółć gęślą jaźń. Więcej szczegółów.';
    onTurnFeedback('c','t',{rating:1,comment:original,updatedAt:'2026-10-24T12:00:00Z'});
    render(<Feed />);
    const action = screen.getByRole('button',{name:'Edit comment'});
    expect(action.title).toBe('Edit comment');
    expect(action.textContent).toBe(preview);
    expect(screen.queryByText('Edit comment')).toBeNull();
    expect(screen.queryByRole('textbox')).toBeNull();
    fireEvent.click(screen.getByText(preview));
    const field = screen.getByLabelText('Feedback comment (optional)');
    expect(field.value).toBe(original);
    expect(document.activeElement).toBe(field);
    expect(screen.queryByRole('button',{name:'Edit comment'})).toBeNull();
    expect(client.setTurnFeedback).not.toHaveBeenCalled();
    fireEvent.change(field,{target:{value:'Discard this draft'}});
    fireEvent.click(screen.getByRole('button',{name:'Cancel'}));
    expect(screen.getByRole('button',{name:'Edit comment'}).textContent).toBe(preview);
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Edit comment'}));
    fireEvent.click(screen.getByRole('button',{name:'Edit comment'}));
    expect(screen.getByRole('textbox').value).toBe(original);
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await screen.findByRole('button',{name:'Edit comment'});
    expect(client.setTurnFeedback).toHaveBeenCalledWith('c','t',{rating:1,comment:original});
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Edit comment'}));
  });

  it('returns to the icon-only action after clearing comment text without clearing the vote', async () => {
    onTurnFeedback('c','t',{rating:-1,comment:'Old comment',updatedAt:'2026-10-24T12:00:00Z'});
    client.setTurnFeedback.mockResolvedValueOnce({rating:-1,comment:null,updatedAt:'2026-10-24T12:00:01Z'});
    render(<Feed />);
    fireEvent.click(screen.getByRole('button',{name:'Edit comment'}));
    fireEvent.change(screen.getByRole('textbox'),{target:{value:' \n\t '}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    const action = await screen.findByRole('button',{name:'Add comment'});
    expect(action.textContent).toBe('');
    expect(document.activeElement).toBe(action);
    expect(screen.getByRole('button',{name:'Bad response'}).getAttribute('aria-pressed')).toBe('true');
    expect(client.clearTurnFeedback).not.toHaveBeenCalled();
    fireEvent.click(action);
    expect(screen.getByRole('textbox').value).toBe('');
  });

  it('retains the original preview and full draft on failure, then shows the acknowledged replacement', async () => {
    const original = 'Original\ncomment';
    const replacement = 'Changed 👍\n  More details';
    onTurnFeedback('c','t',{rating:1,comment:original,updatedAt:'2026-10-24T12:00:00Z'});
    let reject;
    client.setTurnFeedback.mockReturnValueOnce(new Promise((_resolve,fail) => {reject=fail;}));
    render(<Feed />);
    fireEvent.click(screen.getByRole('button',{name:'Edit comment'}));
    fireEvent.change(screen.getByRole('textbox'),{target:{value:replacement}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    expect(screen.getByRole('textbox').disabled).toBe(true);
    expect(screen.getByRole('button',{name:'Cancel'}).disabled).toBe(true);
    reject(new Error('offline'));
    await screen.findByRole('alert');
    expect(screen.getByRole('textbox').value).toBe(replacement);
    expect(screen.queryByRole('button',{name:'Edit comment'})).toBeNull();
    fireEvent.click(screen.getByRole('button',{name:'Cancel'}));
    expect(screen.getByRole('button',{name:'Edit comment'}).textContent).toBe('Original comment');
    fireEvent.click(screen.getByRole('button',{name:'Edit comment'}));
    fireEvent.change(screen.getByRole('textbox'),{target:{value:replacement}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    const action = await screen.findByRole('button',{name:'Edit comment'});
    expect(action.textContent).toBe('Changed 👍 More details');
    expect(client.setTurnFeedback).toHaveBeenLastCalledWith('c','t',{rating:1,comment:replacement});
  });

  it('counts Unicode characters and keeps the full allowed comment when saving', async () => {
    onTurnFeedback('c','t',{rating:1,comment:null,updatedAt:'2026-10-24T12:00:00Z'});
    render(<Feed />);
    fireEvent.click(screen.getByRole('button',{name:'Add comment'}));
    const field = screen.getByRole('textbox');
    fireEvent.change(field,{target:{value:'👍'.repeat(4001)}});
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(true);
    expect(field.getAttribute('aria-invalid')).toBe('true');
    const allowed = '👍'.repeat(4000);
    fireEvent.change(field,{target:{value:allowed}});
    expect(screen.getByText('4000/4000 characters')).toBeTruthy();
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(false);
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    const action = await screen.findByRole('button',{name:'Edit comment'});
    expect(client.setTurnFeedback).toHaveBeenCalledWith('c','t',{rating:1,comment:allowed});
    expect(action.textContent).toBe(allowed);
    fireEvent.click(action);
    expect(screen.getByRole('textbox').value).toBe(allowed);
  });
});
