import {beforeEach,describe,it,expect} from 'vitest';
import {activeWindows,getMetadataSignal,getViewSignal,getInputSignal,getSelectionSignal} from 'forge/core';
import {setScopedActiveSurface,setScopedWorkspaceSelection} from './conversationWindow';
import {buildWorkspaceQueryContext} from './workspaceQueryContext';

describe('workspace composer context',()=>{
 beforeEach(()=>{
  const values=new Map();
  global.window={sessionStorage:{getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value)}};
  activeWindows.value=[{windowId:'view-1',title:'Orders',conversationId:'c',workspaceObject:{objectId:'object-1',lifecycle:{state:'ready'}}}];
  setScopedActiveSurface('c','workspace');setScopedWorkspaceSelection('c','view-1');
  getMetadataSignal('view-1').value={dataSource:{records:{uniqueKey:[{field:'id'}]}}};
  getViewSignal('view-1').value={tabs:{sections:'activity'}};
  getInputSignal('view-1DSrecords').value={filter:{status:'active'},fetch:true};
  getSelectionSignal('view-1DSrecords').value={selected:{id:7,privateText:'do not copy whole records'}};
 });
 it('includes active view choices and selected keys, not execution or permission state',()=>{
  expect(buildWorkspaceQueryContext('c')).toEqual({workspace:{objectId:'object-1',windowId:'view-1',title:'Orders',renderer:'forgeWindow',internalTabs:{sections:'activity'},dataSources:{records:{filter:{status:'active'},selectedKeys:{id:7}}}}});
 });
 it('does not leak another conversation or imply workspace context from chat',()=>{
  expect(buildWorkspaceQueryContext('other')).toEqual({});
  setScopedActiveSurface('c','conversation');
  expect(buildWorkspaceQueryContext('c')).toEqual({});
 });
 it('snapshots multiple declared composite identifiers without copying rows or complex key values',()=>{
  getMetadataSignal('view-1').value={dataSource:{records:{uniqueKey:['id',{field:'region'}, {field:'unsafe'}]}}};
  getSelectionSignal('view-1DSrecords').value={selection:[{id:123,region:'west',privateText:'secret',unsafe:{secret:true}},{id:456,region:'east'}]};
  const result=buildWorkspaceQueryContext('c');
  expect(result.workspace.dataSources.records.selectedIds).toEqual([{id:123,region:'west'},{id:456,region:'east'}]);
  expect(JSON.stringify(result)).not.toContain('secret');
  getViewSignal('view-1').value.tabs.sections='changed';
  getInputSignal('view-1DSrecords').value.filter.status='changed';
  expect(result.workspace.internalTabs).toEqual({sections:'activity'});
  expect(result.workspace.dataSources.records.filter).toEqual({status:'active'});
 });
 it('uses the newly selected window and conversation, clearing absent windows',()=>{
  const first=buildWorkspaceQueryContext('c');
  activeWindows.value=[...activeWindows.peek(),{windowId:'view-2',title:'Inventory',conversationId:'c',workspaceObject:{content:{renderer:'mcpApp'},lifecycle:{state:'ready'}}}];
  setScopedWorkspaceSelection('c','view-2');
  expect(buildWorkspaceQueryContext('c').workspace).toMatchObject({windowId:'view-2',title:'Inventory',renderer:'mcpApp',internalTabs:{},dataSources:{}});
  expect(first.workspace.windowId).toBe('view-1');
  setScopedActiveSurface('other','workspace');setScopedWorkspaceSelection('other','view-1');
  expect(buildWorkspaceQueryContext('other')).toEqual({});
  activeWindows.value=[];
  expect(buildWorkspaceQueryContext('c')).toEqual({});
  expect(buildWorkspaceQueryContext('')).toEqual({});
 });

});
