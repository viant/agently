import { client } from './agentlyClient';
import { normalizeToolResult } from './reportingToolClient';

export async function emitReportUIEvent({ kind = '', detail = {}, windowId = '', windowKey = '', clientId = '', conversationId = '' } = {}) {
  const normalizedKind = String(kind || '').trim();
  if (!normalizedKind) {
    throw new Error('report UI event kind is required');
  }
  const normalizedConversationId = String(conversationId || '').trim();
  if (!normalizedConversationId) {
    throw new Error('report UI event conversationId is required');
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
