// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
const {openWindow, inputs} = vi.hoisted(() => ({openWindow:vi.fn(), inputs:{AdvertiserId:42}}));
vi.mock('./menuBarHelpers', () => ({openWindow}));
vi.mock('../services/agentlyClient', () => ({getAuthMeSilently:vi.fn(async()=>({subject:'menu-test'}))}));
vi.mock('./Sidebar', () => ({default:()=>null}));
vi.mock('forge/core', async () => {
  const {signal} = await import('@preact/signals-react');
  return {activeWindows:signal([{windowId:'selected',parameters:inputs}]), selectedWindowId:signal('selected'),getWindowContext:()=>({signals:{windowForm:{value:inputs}}})};
});
import WorkspaceSidebar from './WorkspaceSidebar.jsx';
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.clearAllMocks();});
it('opens a typed report in the hosted workspace using bound current parameters and a report resource ref', async () => {
  const action={type:'report',resourceUri:'report://ar/overview',providerIdentity:'menu-connection',parameters:{ViewId:1294},parameterBindings:{AdvertiserIds:{from:'AdvertiserId',array:true}}};
  const original=JSON.stringify(action);
  vi.stubGlobal('fetch', vi.fn(async()=>({ok:true,json:async()=>({workspaceId:'test',layout:{left:{history:{enabled:false}},applications:[{id:'ar',title:'AR',menus:[{id:'overview',title:'Overview',action}]}]}})})));
  render(<WorkspaceSidebar conversationId="current-chat"/>);
  fireEvent.click(await screen.findByRole('button',{name:'Overview'}));
  expect(openWindow).toHaveBeenCalledWith('report://ar/overview','Overview',[],{
    parameters:{ViewId:1294,AdvertiserIds:[42]},conversationId:'current-chat',parentKey:'chat/new',presentation:'hosted',region:'chat.top',resourceRef:{uri:'report://ar/overview'},
  });
  expect(JSON.stringify(action)).toBe(original);
});
