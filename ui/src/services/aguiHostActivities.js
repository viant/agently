import { useSyncExternalStore } from 'react';

const empty = Object.freeze([]);
const activitiesByConversation = new Map();
const listeners = new Set();
const publish = () => { for (const listener of listeners) listener(); };
const subscribe = listener => { listeners.add(listener); return () => listeners.delete(listener); };

// Called exclusively by the trusted Agently protocol subscription. This state
// is renderer-only: it is never merged into messages, prompts, or transcript.
export function replaceAgUiHostActivities(conversationId, activities = [], profile = 'standard') {
  const id = String(conversationId || '').trim();
  if (!id) return;
  const next = (profile === 'agently' ? activities : []).filter(message => {
    const content = message?.content;
    const binding = content?._agentlyApp;
    return message?.role === 'activity' && message?.activityType === 'mcp-apps'
      && typeof message.id === 'string' && message.id
      && binding?.version === '1' && typeof binding.appInstanceId === 'string' && binding.appInstanceId
      && typeof content.serverId === 'string' && content.serverId.startsWith('agui-app:')
      && binding.publicServerId === content.serverId && binding.serverHash === content.serverHash
      && typeof content.resourceUri === 'string' && content.resourceUri.startsWith('ui://')
      && binding.resourceUri === content.resourceUri;
  }).map(message => ({ ...structuredClone(message), conversationId: id,
    mountKey: `agently:${id}:${message.id}:${message.content._agentlyApp.appInstanceId}` }));
  if (JSON.stringify(activitiesByConversation.get(id) || empty) === JSON.stringify(next)) return;
  activitiesByConversation.set(id, next.length ? Object.freeze(next) : empty);
  publish();
}
export function resetAgUiHostActivities() { activitiesByConversation.clear(); publish(); }
export function getAgUiHostActivities(conversationId) { return activitiesByConversation.get(conversationId) || empty; }
export function useAgUiHostActivities(conversationId) {
  return useSyncExternalStore(subscribe, () => getAgUiHostActivities(conversationId), () => empty);
}
if (typeof window !== 'undefined') window.addEventListener?.('agently:session-reset', resetAgUiHostActivities);
