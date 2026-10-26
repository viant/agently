import { describe, expect, it } from 'vitest';
import { resolveLayoutWindowKey, resolveLayoutWindowOptions, selectedWindowInputs } from './WorkspaceSidebar.jsx';
import {layoutWindowDestinations} from './layoutNavigation';
import {activeWindows, selectedWindowId, getFormSignal} from 'forge/core';

describe('workspace layout window actions', () => {
  it('reads window form values without requiring a datasource context', () => {
    const previous = activeWindows.peek(), previousId = selectedWindowId.peek();
    try {
      activeWindows.value = [{windowId: 'plain-output', parameters: {campaignId: 'a'}}];
      selectedWindowId.value = 'plain-output';
      getFormSignal('plain-output:windowForm').value = {campaignId: 'b', reportId: 'r'};
      expect(selectedWindowInputs()).toEqual({campaignId: 'b', reportId: 'r'});
    } finally {activeWindows.value = previous; selectedWindowId.value = previousId;}
  });
  it('keeps local and remote window identities distinct', () => {
    expect(resolveLayoutWindowKey({ windowKey: 'overview' })).toBe('overview');
    expect(resolveLayoutWindowKey({ provider: 'workspace', windowKey: 'overview' })).toBe('overview');
    expect(resolveLayoutWindowKey({ provider: 'operations-ui', windowKey: 'overview' })).toBe('provider:operations-ui:overview');
  });

  it.each(['advertisers', 'campaigns', 'spo'])('opens %s without requiring a chat host on the landing page', (windowKey) => {
    const options = resolveLayoutWindowOptions({windowKey, parameters: {executeOnOpen: true}}, '');
    expect(options).toEqual({parameters: {executeOnOpen: true}, conversationId: undefined,
      parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top'});
  });

  it('hosts menu windows in the active conversation and preserves report parameters', () => {
    expect(resolveLayoutWindowOptions({parameters: {reportStarterId: 'spo', executeOnOpen: true}}, 'conv-1')).toEqual({
      parameters: {reportStarterId: 'spo', executeOnOpen: true}, conversationId: 'conv-1',
      parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top',
    });
  });

  it('carries the provider and canonical root selector for a published window', () => {
    const options = resolveLayoutWindowOptions({resourceUri: 'window://studio/Connectors', providerIdentity: 'studio'}, 'conv-1');
    expect(options).toMatchObject({providerIdentity: 'studio', resourceRef: {uri: 'window://studio/Connectors'}});
    expect(resolveLayoutWindowOptions({resourceUri: 'window://studio/Connectors'}, 'conv-1').resourceRef).toBeUndefined();
  });
});

it('identifies nested destinations by catalog path and inherits disabled state', () => {
 const action = {type:'window',windowKey:'records',parameters:{privateContext:'not-a-route'}};
 const result = layoutWindowDestinations({applications:[
  {id:'a',menus:[{id:'group',disabled:true,children:[{id:'records',title:'Records',action}]}]},
  {id:'b',menus:[{id:'records',title:'Records',action},{id:'external',action:{type:'url',url:'https://example.com'}}]},
 ]});
 expect(result.map(({destination,disabled})=>({destination,disabled}))).toEqual([
  {destination:JSON.stringify(['a','group','records']),disabled:true},
  {destination:JSON.stringify(['b','records']),disabled:false},
 ]);
 expect(result[1].action).toBe(action);
 expect(result[1].destination).not.toContain('privateContext');
});
