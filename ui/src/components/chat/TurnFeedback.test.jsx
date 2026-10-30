// @vitest-environment jsdom
import React from 'react';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import TurnFeedback from './TurnFeedback.jsx';
import { __resetAll, onTranscript, onTurnFeedback, useChatProjection } from '../../services/chatStore.js';
import { client } from '../../services/agentlyClient.js';

vi.mock('../../services/agentlyClient.js', () => ({ client: {setTurnFeedback:vi.fn(), clearTurnFeedback:vi.fn()} }));

function Feed() {
  const row = useChatProjection('c').find(value => value.kind === 'iteration');
  return <TurnFeedback conversationId="c" turnId="t" feedback={row?.feedback} editable={row?.feedbackEditable}/>;
}

const saved = (rating, comment = null) => ({rating, comment, updatedAt:'2026-10-24T12:00:00Z'});
async function openMenu(name = 'Feedback options') {
  const trigger = screen.getByRole('button', {name, exact:true});
  trigger.focus();
  // Native buttons emit a click after Enter. Do not also toggle on keydown.
  fireEvent.keyDown(trigger, {key:'Enter',keyCode:13});
  fireEvent.click(trigger);
  const menu = await screen.findByRole('menu', {name:'Response feedback'});
  await waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));
  return menu;
}
async function choose(name, trigger) {
  const menu = await openMenu(trigger);
  fireEvent.click(within(menu).getByRole('menuitem', {name, exact:true}));
  await waitFor(() => expect(screen.queryByRole('menu')).toBeNull());
}
async function openComment(name = 'Add comment') {
  await choose(name);
  const dialog = await screen.findByRole('dialog', {name:'Feedback'});
  await waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('textbox')));
  return within(dialog).getByRole('textbox');
}
async function closed() {
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
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
  it('saves positive feedback without opening a form, edits through options, flips and clears', async () => {
    render(<Feed/>);
    const rate = screen.getByRole('button', {name:'Rate response'});
    expect(rate.getAttribute('data-forge-variant')).toBe('ghost');
    await choose('Good response', 'Rate response');
    expect(client.setTurnFeedback).toHaveBeenLastCalledWith('c','t',{rating:1,comment:null});
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getByRole('button',{name:'Remove positive feedback'}).getAttribute('aria-pressed')).toBe('true');

    const field = await openComment();
    expect(field.getAttribute('data-forge-type')).toBe('body');
    expect(screen.queryAllByRole('textbox')).toHaveLength(1);
    expect(screen.queryByRole('checkbox')).toBeNull();
    expect(screen.queryByRole('combobox')).toBeNull();
    fireEvent.change(field,{target:{value:'Unsaved'}});
    fireEvent.click(screen.getByRole('button',{name:'Not now'}));
    await closed();
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Feedback options'}));
    expect(client.setTurnFeedback).toHaveBeenCalledTimes(1);
    expect((await openComment()).value).toBe('');
    fireEvent.change(screen.getByRole('textbox'),{target:{value:'Pomocne 👍'}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await closed();
    expect(screen.queryByText('Pomocne 👍')).toBeNull();
    expect(screen.getByRole('status').textContent).toBe('Comment saved');

    await choose('Bad response');
    const dialog = await screen.findByRole('dialog',{name:'Feedback'});
    expect(screen.queryByText('Comment saved')).toBeNull();
    expect(within(dialog).getByRole('textbox').value).toBe('Pomocne 👍');
    expect(client.setTurnFeedback).toHaveBeenLastCalledWith('c','t',{rating:-1,comment:'Pomocne 👍'});
    fireEvent.click(screen.getByRole('button',{name:'Cancel'}));
    await closed();
    fireEvent.click(screen.getByRole('button',{name:'Remove negative feedback'}));
    await screen.findByRole('button',{name:'Rate response'});
    expect(client.clearTurnFeedback).toHaveBeenCalledWith('c','t');
    const menu = await openMenu();
    expect(within(menu).queryByRole('menuitem',{name:/comment/})).toBeNull();
  }, 15000);

  it('opens the negative comment only after acknowledgment and blocks duplicate writes', async () => {
    let finish;
    client.setTurnFeedback.mockImplementationOnce((_c,_t,input) => new Promise(resolve => {
      finish = () => resolve({...input,updatedAt:'2026-10-24T12:00:00Z'});
    }));
    render(<Feed/>);
    await choose('Bad response','Rate response');
    expect(screen.queryByRole('dialog')).toBeNull();
    const trigger = screen.getByRole('button',{name:'Rate response'});
    expect(trigger.disabled).toBe(true);
    expect(screen.getByRole('button',{name:'Feedback options'}).disabled).toBe(true);
    fireEvent.click(trigger);
    expect(client.setTurnFeedback).toHaveBeenCalledTimes(1);
    await act(async () => finish());
    const dialog = await screen.findByRole('dialog',{name:'Feedback'});
    await waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('textbox')));
    expect(screen.getByRole('button',{name:'Remove negative feedback'}).getAttribute('aria-pressed')).toBe('true');
  });

  it('keeps the canonical vote after a failed rating and lets the user retry', async () => {
    client.setTurnFeedback.mockRejectedValueOnce(new Error('offline'));
    render(<Feed/>);
    await choose('Bad response','Rate response');
    await screen.findByRole('alert');
    expect(screen.queryByRole('dialog')).toBeNull();
    const trigger = screen.getByRole('button',{name:'Rate response'});
    expect(trigger.getAttribute('aria-pressed')).toBe('false');
    expect(document.activeElement).toBe(trigger);
    await choose('Bad response','Rate response');
    await screen.findByRole('dialog',{name:'Feedback'});
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('restores the initiating control after a write unless focus moved elsewhere', async () => {
    let finish;
    client.setTurnFeedback.mockImplementation((_c, _t, input) => new Promise(resolve => {
      finish = () => resolve({...input, updatedAt:'2026-10-24T12:00:00Z'});
    }));
    render(<><button>Other control</button><Feed/></>);
    await choose('Good response','Rate response');
    await act(async () => finish());
    const selected = await screen.findByRole('button',{name:'Remove positive feedback'});
    expect(document.activeElement).toBe(selected);
    // Change a saved negative vote to positive so there is no automatic modal.
    act(() => onTurnFeedback('c','t',saved(-1)));
    await choose('Good response');
    const other = screen.getByRole('button',{name:'Other control'});
    other.focus();
    await act(async () => finish());
    await screen.findByRole('button',{name:'Remove positive feedback'});
    expect(document.activeElement).toBe(other);
  });

  it('keeps saved comments out of the footer, skips unchanged writes and preserves edited text', async () => {
    const original = 'Pomocne 👍\n\n  Zażółć gęślą jaźń.\tWięcej szczegółów.';
    onTurnFeedback('c','t',saved(-1,original));
    render(<Feed/>);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryByText(/Zażółć/)).toBeNull();
    const field = await openComment('Edit comment');
    expect(field.value).toBe(original);
    expect(client.setTurnFeedback).not.toHaveBeenCalled();
    fireEvent.change(field,{target:{value:'Discard this draft'}});
    fireEvent.click(screen.getByRole('button',{name:'Cancel'}));
    await closed();
    const reopened = await openComment('Edit comment');
    expect(reopened.value).toBe(original);
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(true);
    fireEvent.submit(reopened.form);
    expect(client.setTurnFeedback).not.toHaveBeenCalled();
    fireEvent.change(reopened,{target:{value:original + '\nUpdated'}});
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(false);
    fireEvent.change(reopened,{target:{value:original}});
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(true);
    fireEvent.change(reopened,{target:{value:original + '\nUpdated'}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await closed();
    expect(client.setTurnFeedback).toHaveBeenCalledWith('c','t',{rating:-1,comment:original + '\nUpdated'});
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Feedback options'}));
  });

  it('explains the saved rating and skips blank new comments without another write', async () => {
    render(<Feed/>);
    await choose('Bad response','Rate response');
    const field = await screen.findByRole('textbox');
    const hint = screen.getByText('Your rating is saved.');
    expect(field.getAttribute('aria-describedby').split(' ')).toContain(hint.id);
    const save = screen.getByRole('button',{name:'Save comment'});
    expect(save.disabled).toBe(true);
    fireEvent.submit(field.form);
    fireEvent.change(field,{target:{value:' \n\t '}});
    expect(save.disabled).toBe(true);
    fireEvent.submit(field.form);
    fireEvent.change(field,{target:{value:'A comment'}});
    expect(save.disabled).toBe(false);
    fireEvent.change(field,{target:{value:''}});
    expect(save.disabled).toBe(true);
    fireEvent.click(screen.getByRole('button',{name:'Not now'}));
    await closed();
    expect(client.setTurnFeedback).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button',{name:'Remove negative feedback'}).getAttribute('aria-pressed')).toBe('true');
  });

  it('clears only the comment when saving whitespace and restores Add comment', async () => {
    onTurnFeedback('c','t',saved(-1,'Old comment'));
    client.setTurnFeedback.mockResolvedValueOnce(saved(-1));
    render(<Feed/>);
    fireEvent.change(await openComment('Edit comment'),{target:{value:' \n\t '}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await closed();
    expect(screen.getByRole('button',{name:'Remove negative feedback'}).getAttribute('aria-pressed')).toBe('true');
    expect(client.clearTurnFeedback).not.toHaveBeenCalled();
    expect(client.setTurnFeedback).toHaveBeenCalledWith('c','t',{rating:-1,comment:null});
    expect((await openComment()).value).toBe('');
  });

  it('retains the full draft on failure, locks closing during a write, then saves a retry', async () => {
    const original = 'Original\ncomment';
    const replacement = 'Changed 👍\n  More details';
    onTurnFeedback('c','t',saved(1,original));
    let reject;
    client.setTurnFeedback.mockReturnValueOnce(new Promise((_resolve,fail) => {reject=fail;}));
    render(<Feed/>);
    fireEvent.change(await openComment('Edit comment'),{target:{value:replacement}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    expect(screen.getByRole('textbox').disabled).toBe(true);
    expect(screen.getByRole('button',{name:'Cancel'}).disabled).toBe(true);
    expect(screen.getByRole('button',{name:'Close feedback'}).disabled).toBe(true);
    fireEvent.keyDown(screen.getByRole('dialog'),{key:'Escape',keyCode:27});
    expect(screen.getByRole('dialog')).toBeTruthy();
    await act(async () => reject(new Error('offline')));
    await screen.findByRole('alert');
    expect(screen.getByRole('textbox').value).toBe(replacement);
    fireEvent.click(screen.getByRole('button',{name:'Cancel'}));
    await closed();
    expect((await openComment('Edit comment')).value).toBe(original);
    fireEvent.change(screen.getByRole('textbox'),{target:{value:replacement}});
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await closed();
    expect(client.setTurnFeedback).toHaveBeenLastCalledWith('c','t',{rating:1,comment:replacement});
    expect((await openComment('Edit comment')).value).toBe(replacement);
  });

  it('counts Unicode characters and keeps the full allowed comment when saving', async () => {
    onTurnFeedback('c','t',saved(1));
    render(<Feed/>);
    const field = await openComment();
    fireEvent.change(field,{target:{value:'👍'.repeat(4001)}});
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(true);
    expect(field.getAttribute('aria-invalid')).toBe('true');
    const limitError = screen.getByRole('alert');
    expect(limitError.textContent).toContain('Remove 1 character to save your comment.');
    expect(field.getAttribute('aria-describedby').split(' ')).toContain(limitError.id);
    fireEvent.change(field,{target:{value:'👍'.repeat(4002)}});
    expect(screen.getByRole('alert').textContent).toContain('Remove 2 characters to save your comment.');
    const allowed = '👍'.repeat(4000);
    fireEvent.change(field,{target:{value:allowed}});
    expect(screen.getByText('4000/4000 characters')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(field.getAttribute('aria-invalid')).toBe('false');
    expect(field.getAttribute('aria-describedby').split(' ')).not.toContain(limitError.id);
    expect(screen.getByRole('button',{name:'Save comment'}).disabled).toBe(false);
    fireEvent.click(screen.getByRole('button',{name:'Save comment'}));
    await closed();
    expect(client.setTurnFeedback).toHaveBeenCalledWith('c','t',{rating:1,comment:allowed});
    expect((await openComment('Edit comment')).value).toBe(allowed);
  });

  it.each(['Escape','Close feedback','backdrop'])('discards a draft on %s while preserving the saved vote', async action => {
    render(<Feed/>);
    await choose('Bad response','Rate response');
    const field = await screen.findByRole('textbox');
    await waitFor(() => expect(document.activeElement).toBe(field));
    fireEvent.change(field,{target:{value:'Unsaved comment'}});
    if (action === 'Escape') fireEvent.keyDown(field,{key:'Escape',keyCode:27});
    else if (action === 'backdrop') fireEvent.mouseDown(document.querySelector('.bp6-overlay-backdrop'));
    else fireEvent.click(screen.getByRole('button',{name:action}));
    await closed();
    expect(screen.getByRole('button',{name:'Remove negative feedback'}).getAttribute('aria-pressed')).toBe('true');
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Remove negative feedback'}));
    expect(client.setTurnFeedback).toHaveBeenCalledTimes(1);
    expect((await openComment()).value).toBe('');
  });

  it('navigates rating options by keyboard and returns focus on Escape', async () => {
    render(<Feed/>);
    const menu = await openMenu('Rate response');
    const good = within(menu).getByRole('menuitem',{name:'Good response'});
    const bad = within(menu).getByRole('menuitem',{name:'Bad response'});
    expect(document.activeElement).toBe(good);
    fireEvent.keyDown(good,{key:'ArrowDown'});
    expect(document.activeElement).toBe(bad);
    fireEvent.keyDown(bad,{key:'Home'});
    expect(document.activeElement).toBe(good);
    fireEvent.keyDown(good,{key:'End'});
    expect(document.activeElement).toBe(bad);
    fireEvent.keyDown(bad,{key:'Escape',keyCode:27});
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole('button',{name:'Rate response'}));
    expect(client.setTurnFeedback).not.toHaveBeenCalled();
  });
});
