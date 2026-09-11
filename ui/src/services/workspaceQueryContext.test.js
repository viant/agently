import {beforeEach,describe,it,expect} from 'vitest';
import {activeWindows,getMetadataSignal,getViewSignal,getInputSignal,getSelectionSignal} from 'forge/core';
import {setScopedActiveSurface,setScopedWorkspaceSelection} from './conversationWindow';
import {buildWorkspaceQueryContext} from './workspaceQueryContext';

describe('workspace composer context',()=>{
 beforeEach(()=>{
  const values=new Map();
  global.window={sessionStorage:{getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value)}};
  activeWindows.value=[{windowId:'view-1',conversationId:'c',workspaceObject:{objectId:'object-1',lifecycle:{state:'ready'}}}];
  setScopedActiveSurface('c','workspace');setScopedWorkspaceSelection('c','view-1');
  getMetadataSignal('view-1').value={dataSource:{records:{uniqueKey:[{field:'id'}]}}};
  getViewSignal('view-1').value={tabs:{sections:'activity'}};
  getInputSignal('view-1DSrecords').value={filter:{status:'active'},fetch:true};
  getSelectionSignal('view-1DSrecords').value={selected:{id:7,privateText:'do not copy whole records'}};
 });
 it('includes active view choices and selected keys, not execution or permission state',()=>{
  expect(buildWorkspaceQueryContext('c')).toEqual({workspace:{objectId:'object-1',windowId:'view-1',renderer:'forgeWindow',internalTabs:{sections:'activity'},dataSources:{records:{filter:{status:'active'},selectedKeys:{id:7}}}}});
 });
 it('does not leak another conversation or imply workspace context from chat',()=>{
  expect(buildWorkspaceQueryContext('other')).toEqual({});
  setScopedActiveSurface('c','conversation');
  expect(buildWorkspaceQueryContext('c')).toEqual({});
 });
});
