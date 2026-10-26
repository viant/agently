import {chatStoreApplyTranscript, chatStoreNewConversationState, chatStoreProjectConversation} from 'agently-core-ui-sdk';
import {client, getAuthMeSilently, getReportRestoreScope} from './agentlyClient.js';
import {canonicalReportSources} from './workspaceAdapters.js';

export const compareReportVersions = (a, b) => Number(b.assembly.resetVersion || 0) - Number(a.assembly.resetVersion || 0)
  || Number(b.assembly.sequence || 0) - Number(a.assembly.sequence || 0);

export function selectCanonicalReport(rows, content) {
  return canonicalReportSources(rows).flatMap(source =>
    (source.renderedContent?.reports || []).filter(assembly =>
      (source.row.turnId || source.messageId) === (content.turnId || content.messageId)
      && assembly.id === content.reportId && (assembly.scope || 'message') === content.reportScope
      && assembly.status === 'committed'
      && !(source.renderedContent.diagnostics || []).some(item => item.reportId === assembly.id))
      .map(assembly => ({...source, assembly}))).sort(compareReportVersions)[0];
}

export async function readWorkspaceReport(entry) {
  await getAuthMeSilently();
  const scope = getReportRestoreScope();
  // Recheck the owning read endpoint before showing a cached report on open/retry.
  const payload = await client.readConversationHistory({conversationId: entry.conversationId,
    includeModelCalls: true, includeToolCalls: true, includeFeeds: true});
  if (scope !== getReportRestoreScope()) throw new Error('Report account changed.');
  const conversation = payload?.conversation;
  if (conversation?.conversationId !== entry.conversationId) throw new Error('Report conversation does not match.');
  const state = chatStoreNewConversationState(entry.conversationId);
  chatStoreApplyTranscript(state, conversation);
  const selected = selectCanonicalReport(chatStoreProjectConversation(state), entry.workspaceObject.content);
  if (!selected) throw Object.assign(new Error('Report is unavailable.'), {status: 404});
  return {selected, scope};
}
