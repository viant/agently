// @vitest-environment jsdom
import React, {useEffect, useState} from 'react';
import {render, fireEvent, screen, cleanup} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
vi.mock('forge/components',()=>({Button:({icon,variant,...props})=><button {...props}/>}));
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
  fireEvent.click(screen.getByRole('button',{name:'Hide chat'}));
  view.rerender(renderDock(false));
  expect(screen.getByLabelText('Message').value).toBe('Keep this draft');
  expect(mounts).toBe(1);
});
