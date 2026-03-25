const DEFAULT_TTL_MS = 5000;

let currentQueueSync = {
  conversationId: '',
  queuedTurns: [],
  suppressedTurnIds: [],
  updatedAt: 0
};

function uniqueStrings(values = []) {
  return [...new Set((Array.isArray(values) ? values : []).map((value) => String(value || '').trim()).filter(Boolean))];
}

export function publishQueueSync({ conversationId = '', queuedTurns = [], suppressedTurnIds = [] } = {}) {
  const nextConversationId = String(conversationId || '').trim();
  const priorSuppressed = nextConversationId && nextConversationId === currentQueueSync.conversationId
    ? currentQueueSync.suppressedTurnIds
    : [];
  currentQueueSync = {
    conversationId: nextConversationId,
    queuedTurns: Array.isArray(queuedTurns) ? queuedTurns : [],
    suppressedTurnIds: uniqueStrings([...priorSuppressed, ...suppressedTurnIds]),
    updatedAt: Date.now()
  };
  return currentQueueSync;
}

export function clearQueueSync(conversationId = '') {
  const nextConversationId = String(conversationId || '').trim();
  if (nextConversationId && nextConversationId !== currentQueueSync.conversationId) return currentQueueSync;
  currentQueueSync = {
    conversationId: nextConversationId,
    queuedTurns: [],
    suppressedTurnIds: [],
    updatedAt: Date.now()
  };
  return currentQueueSync;
}

export function getQueueSyncSnapshot(conversationId = '', ttlMs = DEFAULT_TTL_MS) {
  const nextConversationId = String(conversationId || '').trim();
  if (!nextConversationId || nextConversationId !== currentQueueSync.conversationId) {
    return {
      conversationId: nextConversationId,
      queuedTurns: [],
      suppressedTurnIds: [],
      updatedAt: 0
    };
  }
  const age = Math.max(0, Date.now() - Number(currentQueueSync.updatedAt || 0));
  if (age > ttlMs) {
    return {
      conversationId: nextConversationId,
      queuedTurns: [],
      suppressedTurnIds: [],
      updatedAt: 0
    };
  }
  return currentQueueSync;
}
