import { beforeEach, describe, expect, it, vi } from 'vitest';

const { executeTool, applyPermission, transport } = vi.hoisted(() => ({ executeTool: vi.fn(), applyPermission: vi.fn(), transport: vi.fn() }));
vi.mock('./agentlyClient', () => ({ client: { executeTool, applyPermission } }));

vi.mock('forge/core', () => ({getActiveUIBridgeTransportContext: transport}));

import { emitReportUIEvent } from './reportEventService';

describe('reportEventService', () => {
  beforeEach(() => { executeTool.mockReset(); applyPermission.mockReset(); transport.mockReturnValue({clientId:'owned-client',sessionId:'owned-session'}); });

  it('does not publish conversation telemetry for a standalone menu report', async () => {
    await expect(emitReportUIEvent({kind: 'report.context_updated', windowId: 'menu-spo'})).resolves.toEqual({recorded: false});
    expect(executeTool).not.toHaveBeenCalled();
  });

  it('omits other report telemetry when the window has no conversation', async () => {
    await expect(emitReportUIEvent({kind: 'report.export_complete'})).resolves.toEqual({recorded: false});
    expect(executeTool).not.toHaveBeenCalled();
  });

  it('records a scoped report UI event', async () => {
    executeTool.mockResolvedValue({ recorded: true });
    await emitReportUIEvent({
      kind: 'report.export_complete',
      windowId: 'window-1',
      conversationId: 'conversation-1',
      detail: { reportName: 'Inventory Brief', artifactId: 'artifact-1' },
    });
    expect(executeTool).toHaveBeenCalledWith('ui/events:record', {
      kind: 'report.export_complete',
      windowId: 'window-1',
      detail: { reportName: 'Inventory Brief', artifactId: 'artifact-1' },
    }, { conversationId: 'conversation-1' });
  });

  it('records an unkeyed report context update at conversation scope', async () => {
    executeTool.mockResolvedValue({ recorded: true });

    await emitReportUIEvent({
      kind: 'report.context_updated',
      conversationId: 'conversation-1',
      windowId: 'stale-local-window',
      detail: { reportId: 'report-1' },
    });

    expect(executeTool).toHaveBeenCalledWith('ui/events:record', {
      kind: 'report.context_updated',
      detail: { reportId: 'report-1' },
    }, { conversationId: 'conversation-1' });
  });
});

describe('admitted framework frame report events', () => {
 beforeEach(() => { executeTool.mockReset(); applyPermission.mockReset(); transport.mockReturnValue({clientId:'owned-client',sessionId:'owned-session'}); });
 const original = {providerIdentity:'configured-framework',uri:'window://agently/chat-new',kind:'working',contentFingerprint:'a'.repeat(64),authorityBinding:'old-owner',validUntil:'2000-01-01T00:00:00Z'};
 const context = {identity:{windowId:'chat/new'},metadata:{resource:original,resourceTarget:{platform:'web',formFactor:'desktop',surface:'conversation',capabilities:['table'],variantFingerprint:'retained-variant',sourceProof:'expired-proof'}},windowState:{windowId:'chat/new',windowKey:'chat/new',parameters:{EntityId:['original'], nested:{v:1}}},services:{__connectorRuntime:{targetContext:{platform:'web',formFactor:'phone'}}}};
 it('opens once immediately before a long-turn event using original selector and inputs', async () => {
  let release;applyPermission.mockImplementation(()=>new Promise(resolve=>{release=resolve;}));executeTool.mockResolvedValue({recorded:true});
  const work=emitReportUIEvent({kind:'report.context_updated',conversationId:'conv-long',context,detail:{reportId:'report-own'}});
  expect(applyPermission).toHaveBeenCalledTimes(1);expect(executeTool).not.toHaveBeenCalled();
  const [key,input]=applyPermission.mock.calls[0];expect(key).toBe('chat/new');
  expect(input).toMatchObject({windowId:'chat/new',clientId:'owned-client',bridgeSessionId:'owned-session',conversationId:'conv-long',windowParams:context.windowState.parameters,targetContext:{platform:'web',formFactor:'desktop',surface:'conversation',capabilities:['table']}});
  expect(input.expectedResource).toEqual({providerIdentity:original.providerIdentity,uri:original.uri,kind:original.kind,revision:undefined,contentFingerprint:original.contentFingerprint});
  expect(input).not.toHaveProperty('resolvedResource');expect(input.expectedResource).not.toHaveProperty('validUntil');expect(input.expectedResource).not.toHaveProperty('authorityBinding');
  expect(input.windowParams).not.toBe(context.windowState.parameters);
  release({});await work;
  expect(executeTool).toHaveBeenCalledTimes(1);expect(executeTool).toHaveBeenCalledWith('ui/events:record',{kind:'report.context_updated',windowId:'chat/new',windowKey:'chat/new',clientId:'owned-client',detail:{reportId:'report-own'}},{conversationId:'conv-long'});
  expect(context.metadata.resource).toEqual(original);expect(context.windowState.parameters.EntityId).toEqual(['original']);
 });
 for(const reason of ['denied','revision drift','foreign actor']) it(`does not record or retry after ${reason}`,async()=>{
  applyPermission.mockRejectedValue(new Error(reason));await expect(emitReportUIEvent({kind:'report.context_updated',conversationId:'conv',context})).rejects.toThrow(reason);
  expect(applyPermission).toHaveBeenCalledTimes(1);expect(executeTool).not.toHaveBeenCalled();
 });
 it('does not record when the negotiated client changes during OPEN',async()=>{
  applyPermission.mockImplementation(async()=>{transport.mockReturnValue({clientId:'foreign-client',sessionId:'foreign-session'});return {};});
  await expect(emitReportUIEvent({kind:'report.context_updated',conversationId:'conv',context})).rejects.toThrow('transport changed');
  expect(executeTool).not.toHaveBeenCalled();expect(applyPermission).toHaveBeenCalledTimes(1);
 });
 it('does not guess a framework frame for ordinary conversation events',async()=>{
  executeTool.mockResolvedValue({recorded:true});await emitReportUIEvent({kind:'report.context_updated',conversationId:'conv'});expect(applyPermission).not.toHaveBeenCalled();
 });
});
