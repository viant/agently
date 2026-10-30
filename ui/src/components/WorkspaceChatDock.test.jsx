// @vitest-environment jsdom
import React, {useEffect, useState} from 'react';
import {render, fireEvent, screen, cleanup} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
vi.mock('forge/components',async()=>({Button:(await import('forge/components/Button.jsx')).default}));
import WorkspaceChatDock from './WorkspaceChatDock.jsx';
afterEach(cleanup);
it('retains one conversation and draft across opening, closing, and leaving a workspace',()=>{
  let mounts=0;
  function Conversation(){const [value,setValue]=useState('');useEffect(()=>{mounts++;},[]);return <input aria-label="Message" value={value} onChange={event=>setValue(event.target.value)}/>;}
  const renderDock=active=><><div>Open report</div><WorkspaceChatDock active={active}><Conversation/></WorkspaceChatDock></>;
  const view=render(renderDock(false));
  fireEvent.change(screen.getByLabelText('Message'),{target:{value:'Keep this draft'}});
  view.rerender(renderDock(true));
  expect(screen.getByRole('button',{name:'Chat'}).getAttribute('aria-expanded')).toBe('false');
  fireEvent.click(screen.getByRole('button',{name:'Chat'}));
  expect(screen.getByRole('region',{name:'Workspace chat'}).contains(screen.getByLabelText('Message'))).toBe(true);
  expect(screen.getByLabelText('Message').value).toBe('Keep this draft');
  expect(screen.getByText('Open report')).toBeTruthy();
  fireEvent.click(screen.getByRole('button',{name:'Split view'}));
  expect(screen.getByRole('region',{name:'Workspace chat'}).className).toContain('is-split');
  expect(screen.getByLabelText('Message').value).toBe('Keep this draft');
  fireEvent.click(screen.getByRole('button',{name:'Overlay'}));
  expect(screen.getByRole('region',{name:'Workspace chat'}).className).toContain('is-overlay');
  fireEvent.click(screen.getByRole('button',{name:'Hide chat'}));
  view.rerender(renderDock(false));
  expect(screen.getByLabelText('Message').value).toBe('Keep this draft');
  expect(mounts).toBe(1);
});

it('initializes the existing chat when opened from a direct workspace route',()=>{
 const onOpen=vi.fn();render(<WorkspaceChatDock active onOpen={onOpen}><span>Conversation</span></WorkspaceChatDock>);
 expect(onOpen).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'Chat'}));expect(onOpen).toHaveBeenCalledOnce();
});
