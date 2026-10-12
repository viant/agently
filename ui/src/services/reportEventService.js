import {getActiveUIBridgeTransportContext} from 'forge/core';
import { client } from './agentlyClient';
import { normalizeToolResult } from './reportingToolClient';

export async function emitReportUIEvent({ kind = '', detail = {}, windowId = '', windowKey = '', clientId = '', conversationId = '', context = null } = {}) {
  const normalizedKind = String(kind || '').trim();
  if (!normalizedKind) {
    throw new Error('report UI event kind is required');
  }
  const normalizedConversationId = String(conversationId || '').trim();
  if (!normalizedConversationId) {
    // UI events annotate chat history; export/run persistence is handled by
    // its own services. A standalone report has no history to annotate.
    return { recorded: false };
  }
  // A mounted framework frame may outlive its original OPEN lease. A report
  // event takes a fresh OPEN of that same selector; it grants no report authority.
  const frame = context?.windowState;
  const metadata = context?.metadata;
  if (frame?.windowKey === 'chat/new' && frame?.windowId === 'chat/new' && context?.identity?.windowId === frame?.windowId && metadata?.resource) {
    const transport = getActiveUIBridgeTransportContext();
    if (!transport?.clientId || !transport?.sessionId) throw new Error('Workspace UI transport is unavailable');
    const original = metadata.resource;
    const expectedResource = {uri: original.uri, providerIdentity: original.providerIdentity,
      kind: original.kind, revision: original.revision, contentFingerprint: original.contentFingerprint};
    const originalTarget = metadata.resourceTarget || frame.resourceTarget || context?.services?.__connectorRuntime?.targetContext || {};
    const targetContext = Object.fromEntries(['platform','formFactor','surface','capabilities']
      .filter(key => originalTarget[key] !== undefined).map(key => [key, structuredClone(originalTarget[key])]));
    await client.applyPermission(frame.windowKey, {
      conversationId: normalizedConversationId,
      windowId: frame.windowId, clientId: transport.clientId, bridgeSessionId: transport.sessionId,
      expectedResource,
      windowParams: structuredClone(frame.openedWindowParameters || frame.parameters || {}),
      targetContext,
    });
    const currentTransport = getActiveUIBridgeTransportContext();
    if (currentTransport?.clientId !== transport.clientId || currentTransport?.sessionId !== transport.sessionId) throw new Error('Workspace UI transport changed');
    windowId = frame.windowId; windowKey = frame.windowKey; clientId = transport.clientId;
  }
  const normalizedWindowId = String(windowId || '').trim();
  const normalizedWindowKey = String(windowKey || '').trim();
  // A report builder may retain its local window id after the browser bridge
  // has reconnected and replaced the registry snapshot. Context updates are
  // advisory conversation telemetry, so an unpaired local id must not turn a
  // successful report interaction into an "invalid input" request. Exact
  // window identities (and all durable lifecycle events) remain unchanged.
  const conversationScopedContextUpdate = normalizedKind === 'report.context_updated'
    && normalizedWindowId
    && !normalizedWindowKey;
  return normalizeToolResult(await client.executeTool('ui/events:record', {
    kind: normalizedKind,
    detail: detail && typeof detail === 'object' && !Array.isArray(detail) ? detail : {},
    ...(!conversationScopedContextUpdate && normalizedWindowId ? { windowId: normalizedWindowId } : {}),
    ...(normalizedWindowKey ? { windowKey: normalizedWindowKey } : {}),
    ...(String(clientId || '').trim() ? { clientId: String(clientId).trim() } : {}),
  }, { conversationId: normalizedConversationId }));
}
