import {MAIN_CHAT_WINDOW_ID, resolveConversationSelection} from './conversationWindow';

// A delayed restoration owns only the selection present when it started.
// Activity and creation notifications do not represent user navigation.
export function trackConversationRestoration(target = typeof window !== 'undefined' ? window : null) {
  const startingConversationId = resolveConversationSelection(MAIN_CHAT_WINDOW_ID);
  let generation = 0;
  const initialGeneration = generation;
  const events = ['agently:conversation-new', 'agently:conversation-select', 'agently:conversation-path', 'popstate'];
  const advance = event => {
    const windowId = String(event?.detail?.windowId || '').trim();
    if (windowId && windowId !== MAIN_CHAT_WINDOW_ID) return;
    if (event?.type === 'agently:conversation-new' && String(event?.detail?.id || '').trim()) return;
    if (['agently:conversation-select', 'agently:conversation-path'].includes(event?.type)) {
      const selectedId = String(event?.detail?.id || event?.detail?.conversationId || '').trim();
      if (selectedId === startingConversationId) return;
    }
    generation += 1;
  };
  for (const name of events) target?.addEventListener?.(name, advance);
  return {
    isCurrent: () => generation === initialGeneration,
    dispose: () => {
      generation += 1;
      for (const name of events) target?.removeEventListener?.(name, advance);
    },
  };
}
