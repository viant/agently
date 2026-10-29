// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, within} from '@testing-library/react';
import {activeWindows, addWindow, selectedTabId, selectedWindowId} from 'forge/core';
import {useWindowHandlers} from 'forge/hooks';
import {sectionWindowIds, belongsToAppSection} from './sectionWindowTree.js';

vi.mock('forge/window', async importOriginal => {
  const actual = await importOriginal();
  return {...actual, default: ({window: entry}) => {
    React.useEffect(() => actual.syncWindowRuntimeHints(entry.windowId, {presentation:'hosted',region:'chat.top'}), [entry.windowId]);
    return <div><span>{entry.windowKey} content</span>
      <button onClick={() => useWindowHandlers(entry.windowId).openTarget({target:{kind:'window',windowKey:entry.windowKey.endsWith('detail')?'nested':'detail',windowTitle:entry.windowKey.endsWith('detail')?'Nested':'Detail',parameters:{RecordId:[7]}}})}>Open detail</button>
    </div>;
  }};
});
vi.mock('./ReportWindow.jsx', () => ({default: () => <div>Report</div>, isReportWindow: () => false}));
import AppSectionContent from './AppSectionContent.jsx';

function visible(element) {
  for (let node=element; node instanceof HTMLElement; node=node.parentElement) {
    const style=getComputedStyle(node);
    if (node.hidden || style.display==='none' || style.visibility==='hidden') return false;
  }
  return true;
}

afterEach(() => {cleanup(); activeWindows.value=[]; selectedTabId.value=null; selectedWindowId.value=null;});
it.each(['records', 'window://example/records'])('renders linked children for %s', async windowKey => {
  let root;
  await act(async () => {root=addWindow('Records', null, windowKey, '', true, {}, {region:'app.section'});});
  render(<AppSectionContent window={root}/>);
  expect(visible(await screen.findByText(`${windowKey} content`))).toBe(true);
  await act(async () => {fireEvent.click(screen.getByRole('button',{name:'Open detail'}));});
  const key=windowKey.startsWith('window://')?'window://example/detail':'detail';
  expect(visible(await screen.findByText(`${key} content`))).toBe(true);
  expect(activeWindows.peek().find(win=>win.windowKey===key).parameters).toEqual({RecordId:[7]});
  await act(async () => {fireEvent.click(screen.getByRole('button',{name:'Open detail'}));});
  const nestedKey=windowKey.startsWith('window://')?'window://example/nested':'nested';
  expect(visible(await screen.findByText(`${nestedKey} content`))).toBe(true);
  await act(async () => {fireEvent.click(screen.getByRole('button',{name:'Open detail'}));});
  expect(visible(screen.getByText(`${key} content`))).toBe(true);
  await act(async () => {fireEvent.click(screen.getByRole('tab',{name:/Records/}));});
  expect(visible(screen.getByText(`${windowKey} content`))).toBe(true);
  expect(visible(screen.getByText(`${key} content`))).toBe(false);
});
it('keeps unrelated sections, chat, and cycles outside the selected tree', () => {
  const windows=[{windowId:'root',region:'app.section'},{windowId:'child',parentKey:'root'},
    {windowId:'nested',parentKey:'child'},{windowId:'other',region:'app.section'},
    {windowId:'chat'},{windowId:'cycle-a',parentKey:'cycle-b'},{windowId:'cycle-b',parentKey:'cycle-a'}];
  expect([...sectionWindowIds(windows,'root')]).toEqual(['root','child','nested']);
  expect(belongsToAppSection(windows,windows[2])).toBe(true);
  expect(belongsToAppSection(windows,windows[4])).toBe(false);
  expect(belongsToAppSection(windows,windows[5])).toBe(false);
});

it.each(['records','window://example/records'])('keeps the three-level parent navigation in the header for %s',async windowKey=>{
 const header=document.createElement('header');document.body.append(header);
 try {
  let root;
  await act(async()=>{root=addWindow('Records',null,windowKey,'',true,{}, {region:'app.section'});});
  const view=render(<AppSectionContent window={root} headerMount={header}/>);
  expect(within(header).getByRole('tab',{name:'Records'})).toBeTruthy();
  expect(view.container.querySelector('[role=tablist]')).toBeNull();
  await act(async()=>fireEvent.click(screen.getByRole('button',{name:'Open detail'})));
  await act(async()=>fireEvent.click(screen.getByRole('button',{name:'Open detail'})));
  expect(within(header).getAllByRole('tab')).toHaveLength(3);
  expect(header.querySelectorAll('.forge-icon')).toHaveLength(3);
  await act(async()=>fireEvent.click(within(header).getByRole('tab',{name:'Detail'})));
  const detail=windowKey.startsWith('window://')?'window://example/detail':'detail';
  expect(visible(screen.getByText(`${detail} content`))).toBe(true);
  await act(async()=>fireEvent.click(within(header).getByRole('tab',{name:'Records'})));
  expect(visible(screen.getByText(`${windowKey} content`))).toBe(true);
  expect(visible(screen.getByText(`${detail} content`))).toBe(false);
 } finally {cleanup();header.remove();}
});
