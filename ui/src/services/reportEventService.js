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
  return normalizeToolResult(await client.executeTool('ui/events:record', {
    kind: normalizedKind,
    detail: detail && typeof detail === 'object' && !Array.isArray(detail) ? detail : {},
    ...(String(windowId || '').trim() ? { windowId: String(windowId).trim() } : {}),
    ...(String(windowKey || '').trim() ? { windowKey: String(windowKey).trim() } : {}),
    ...(String(clientId || '').trim() ? { clientId: String(clientId).trim() } : {}),
  }, { conversationId: normalizedConversationId }));
}
