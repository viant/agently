import {describe,it,expect} from 'vitest';
import {mcpWorkspaceDescriptor} from './workspaceAdapters.js';
describe('MCP workspace adapter',()=>{
 it('retains logical identity and marks historical hydration as non-activating',()=>{
  const request={uri:'ui://view',conversationId:'c',title:'Interactive view',turnId:'t'};
  const live=mcpWorkspaceDescriptor({...request,historical:false});
  const restored=mcpWorkspaceDescriptor({...request,historical:true});
  expect(live.windowId).toBe(restored.windowId);
  expect(live.hostOpenState).toBe('fresh');
  expect(restored.hostOpenState).toBe('historical_replay');
  expect(live.workspaceObject.origin.turnId).toBe('t');
  expect(live.workspaceObject.lifecycle.state).toBe('opening');
 });
 it('scopes identity to its conversation',()=>{
  expect(mcpWorkspaceDescriptor({uri:'ui://view',conversationId:'one'}).windowId)
   .not.toBe(mcpWorkspaceDescriptor({uri:'ui://view',conversationId:'two'}).windowId);
 });
});
