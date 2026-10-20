import {beforeEach,describe,it,expect} from 'vitest';
import {activeWindows,selectedWindowId,getMetadataSignal,getViewSignal,getInputSignal,getSelectionSignal} from 'forge/core';
import {setScopedActiveSurface,setScopedWorkspaceSelection} from './conversationWindow';
import {buildWorkspaceQueryContext} from './workspaceQueryContext';

describe('workspace composer context',()=>{
 beforeEach(()=>{
  const values=new Map();
  global.window={sessionStorage:{getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value)}};
  activeWindows.value=[{windowId:'view-1',title:'Orders',conversationId:'c',workspaceObject:{objectId:'object-1',lifecycle:{state:'ready'}}}];
  selectedWindowId.value = null;
  setScopedActiveSurface('c','workspace');setScopedWorkspaceSelection('c','view-1');
  getMetadataSignal('view-1').value={dataSource:{records:{uniqueKey:[{field:'id'}]}}};
  getViewSignal('view-1').value={tabs:{sections:'activity'}};
  getInputSignal('view-1DSrecords').value={filter:{status:'active'},fetch:true};
  getSelectionSignal('view-1DSrecords').value={selected:{id:7,privateText:'do not copy whole records'}};
 });
 it('captures live tab focus before persisted selection catches up, with entity parameters',()=>{
  activeWindows.value.push({windowId:'view-2',windowKey:'record',windowTitle:'Current record',conversationId:'c',
   parameters:{RecordId:[321],region:'west',form:{private:'do not include'}}});
  getViewSignal('view-2').value={tabs:{details:'overview'}};
  selectedWindowId.value='view-2';
  const captured=buildWorkspaceQueryContext('c').workspace;
  expect(captured).toMatchObject({windowId:'view-2',windowKey:'record',title:'Current record',
   internalTabs:{details:'overview'},parameters:{RecordId:[321],region:'west'}});
  expect(JSON.stringify(captured)).not.toContain('private');
  activeWindows.value[1].parameters.RecordId[0]=999;
  expect(captured.parameters.RecordId).toEqual([321]);
 });
 it('does not populate context with unused datasource definitions',()=>{
  const metadata=getMetadataSignal('view-1').peek();
  for(let i=0;i<100;i++) metadata.dataSource[`unused_${i}`]={uniqueKey:['id']};
  expect(Object.keys(buildWorkspaceQueryContext('c').workspace.dataSources)).toEqual(['records']);
 });
 it('keeps the visible workspace when focus moves to the composer',()=>{
  activeWindows.value.push({windowId:'chat/new',windowKey:'chat/new',conversationId:'c'});
  selectedWindowId.value='chat/new';
  expect(buildWorkspaceQueryContext('c').workspace.windowId).toBe('view-1');
 });
 it('captures the current carousel item without copying content or another window',()=>{
  getViewSignal('view-1').value.carousels={featured:{itemId:'item-b',title:'Selected item',index:1,total:3,body:{private:'not context'}}};
  getViewSignal('view-2').value={carousels:{featured:{itemId:'foreign'}}};
  const captured=buildWorkspaceQueryContext('c').workspace;
  expect(captured.carousels).toEqual({featured:{itemId:'item-b',title:'Selected item',index:1,total:3}});
  getViewSignal('view-1').value.carousels.featured={itemId:'item-c',title:'Next item',index:2,total:3};
  expect(captured.carousels.featured.itemId).toBe('item-b');
  expect(buildWorkspaceQueryContext('c').workspace.carousels.featured.itemId).toBe('item-c');
 });
 it('distinguishes measured visible cards from an actual focused card',()=>{
  getViewSignal('view-1').value.cardRails={offers:{title:'Offers',total:30,visibleItems:[{itemId:7,title:'First visible',index:5},{itemId:8,title:'Second visible',index:6}]}};
  const first=buildWorkspaceQueryContext('c').workspace.cardRails.offers;
  expect(first.visibleItems.map(item=>item.itemId)).toEqual([7,8]);
  expect(first.focusedItem).toBeUndefined();
  getViewSignal('view-1').value.cardRails.offers.focusedItem={itemId:8,title:'Second visible',index:6};
  expect(buildWorkspaceQueryContext('c').workspace.cardRails.offers.focusedItem.itemId).toBe(8);
  expect(first.focusedItem).toBeUndefined();
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
