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
  return {activeWindows:signal([{windowId:'selected',parameters:inputs}]), selectedWindowId:signal('selected'),getFormSignal:()=>({value:inputs})};
});
import WorkspaceSidebar from './WorkspaceSidebar.jsx';
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.clearAllMocks();});
it.each(['report://ar/overview', 'report://ar/group/overview'])('opens %s in the hosted workspace using bound parameters', async (resourceUri) => {
  const action={type:'report',resourceUri,providerIdentity:'menu-connection',parameters:{ViewId:1294},parameterBindings:{AdvertiserIds:{from:'AdvertiserId',array:true}}};
  const original=JSON.stringify(action);
  vi.stubGlobal('fetch', vi.fn(async()=>({ok:true,json:async()=>({workspaceId:'test',layout:{left:{history:{enabled:false}},applications:[{id:'ar',title:'AR',menus:[{id:'overview',title:'Overview',action}]}]}})})));
  render(<WorkspaceSidebar conversationId="current-chat"/>);
  fireEvent.click(await screen.findByRole('button',{name:'Overview'}));
  expect(openWindow).toHaveBeenCalledWith(resourceUri,'Overview',[],{
    parameters:{ViewId:1294,AdvertiserIds:[42]},conversationId:'current-chat',parentKey:'chat/new',presentation:'hosted',region:'chat.top',resourceRef:{uri:resourceUri},
  });
  expect(JSON.stringify(action)).toBe(original);
});
