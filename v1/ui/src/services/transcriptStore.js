import { mergeRowSnapshots } from './rowMerge';

export function syncTranscriptSnapshot({
  context,
  turns,
  pendingElicitations = [],
  reason = 'poll',
  ensureContextResources,
  resolveActiveStreamTurnId,
  mapTranscriptToRows,
  findLatestRunningTurnIdFromTurns,
  findLatestRunningTurnId,
  publishChangeFeed,
  publishPlanFeed,
  setStage,
  liveRows = []
}) {
  const conversationsCtx = context?.Context?.('conversations');
  if (!conversationsCtx) return null;
  const conversationsDS = conversationsCtx.handlers?.dataSource;
  if (!conversationsDS) return null;

  const chatState = ensureContextResources(context);
  const anchoredStreamTurnId = resolveActiveStreamTurnId(turns, chatState);
  if (anchoredStreamTurnId) {
    chatState.activeStreamTurnId = anchoredStreamTurnId;
  }

  const normalizedLiveRows = anchoredStreamTurnId
    ? (Array.isArray(liveRows) ? liveRows : []).map((row) => {
      if (String(row?._type || '').toLowerCase() !== 'stream') return row;
      if (String(row?.turnId || '').trim()) return row;
      return { ...row, turnId: anchoredStreamTurnId, turnStatus: String(row?.turnStatus || 'running') };
    })
    : (Array.isArray(liveRows) ? liveRows : []);
  const activeStreamRow = [...normalizedLiveRows].reverse().find((row) => String(row?._type || '').toLowerCase() === 'stream');
  const rawRunningTurnId = findLatestRunningTurnIdFromTurns(turns);
  const holdAfterTurnId = String(
    activeStreamRow?.turnId
    || anchoredStreamTurnId
    || rawRunningTurnId
    || chatState.runningTurnId
    || ''
  ).trim();
  const { rows, queuedTurns, runningTurnId } = mapTranscriptToRows(turns, {
    holdAfterTurnId: holdAfterTurnId && chatState.stream ? holdAfterTurnId : '',
    pendingElicitations
  });
  const convForm = conversationsDS.peekFormData?.() || {};
  const conversationID = String(convForm?.id || '').trim();
  chatState.activeConversationID = conversationID;
  const sameConversation = String(chatState.lastConversationID || '').trim() === conversationID;
  const previousTranscriptRows = Array.isArray(chatState.transcriptRows) ? chatState.transcriptRows : [];
  const mergedRows = reason === 'poll' && sameConversation && previousTranscriptRows.length > 0
    ? mergeRowSnapshots(previousTranscriptRows, rows)
    : rows;

  const hasRunning = turns.some((turn) => {
    const status = String(turn?.status || turn?.Status || '').trim().toLowerCase();
    return ['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress'].includes(status);
  });

  conversationsDS.setFormData?.({
    values: {
      ...convForm,
      running: hasRunning,
      queuedCount: queuedTurns.length,
      queuedTurns
    }
  });

  publishChangeFeed({ conversationId: conversationID, rows: mergedRows });
  publishPlanFeed({ conversationId: conversationID, rows: mergedRows });

  chatState.lastSyncReason = reason;
  chatState.transcriptRows = mergedRows;
  chatState.lastQueuedTurns = queuedTurns;
  chatState.lastHasRunning = hasRunning;
  chatState.lastConversationID = conversationID;
  chatState.runningTurnId = runningTurnId || findLatestRunningTurnId(mergedRows);

  const shouldFinalizeActiveStream = !hasRunning && !activeStreamRow;
  if (shouldFinalizeActiveStream) {
    chatState.activeStreamPrompt = '';
    chatState.activeStreamTurnId = '';
    chatState.activeStreamStartedAt = 0;
  }

  if (hasRunning) {
    setStage({ phase: 'executing', text: 'Assistant executing…' });
  } else if (queuedTurns.length > 0) {
    setStage({ phase: 'waiting', text: `Queued turns: ${queuedTurns.length}` });
  } else if (reason === 'poll' || reason === 'fetch') {
    setStage({ phase: 'ready', text: 'Ready' });
  }

  return {
    transcriptRows: mergedRows,
    liveRows: normalizedLiveRows,
    queuedTurns,
    hasRunning,
    runningTurnId: chatState.runningTurnId,
    conversationID,
    shouldFinalizeActiveStream
  };
}

export async function tickTranscript({
  context,
  options = {},
  ensureContextResources,
  fetchTranscript,
  fetchPendingElicitations,
  resolveLastTranscriptCursor,
  syncTranscriptSnapshot: doSync
}) {
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const conversationID = String(options?.conversationID || conversationsDS?.peekFormData?.()?.id || '').trim();
  if (!conversationID) return;
  const chatState = ensureContextResources(context);
  const since = String(chatState.lastSinceCursor || '').trim();
  const turns = await fetchTranscript(conversationID, since);
  const currentID = String(conversationsDS?.peekFormData?.()?.id || '').trim();
  if (currentID && currentID !== conversationID) {
    return;
  }
  if (since && turns.length === 0) {
    return;
  }
  if (turns.length > 0) {
    chatState.lastSinceCursor = resolveLastTranscriptCursor(turns);
  }
  const pendingElicitations = await fetchPendingElicitations(conversationID);
  return doSync({ context, turns, pendingElicitations, reason: 'poll' });
}

export function resetTranscriptState({
  context,
  ensureContextResources,
  clearChangeFeed,
  clearPlanFeed,
  getCurrentConversationID
}) {
  const chatState = ensureContextResources(context);
  clearChangeFeed(String(chatState.lastConversationID || getCurrentConversationID(context) || '').trim());
  clearPlanFeed(String(chatState.lastConversationID || getCurrentConversationID(context) || '').trim());
  chatState.lastSinceCursor = '';
  chatState.transcriptRows = [];
  chatState.lastQueuedTurns = [];
  chatState.lastHasRunning = false;
  chatState.runningTurnId = '';
}

export function queueTranscriptRefresh({
  context,
  delay = 120,
  resetSince = false,
  ensureContextResources,
  resetTranscriptState: doReset,
  tickTranscript: doTick
}) {
  const chatState = ensureContextResources(context);
  if (resetSince) {
    doReset({ context });
  }
  if (chatState.refreshTimer) {
    clearTimeout(chatState.refreshTimer);
    chatState.refreshTimer = null;
  }
  chatState.refreshTimer = window.setTimeout(async () => {
    chatState.refreshTimer = null;
    if (chatState.refreshInFlight) return;
    chatState.refreshInFlight = true;
    try {
      await doTick({ context });
    } finally {
      chatState.refreshInFlight = false;
    }
  }, Math.max(0, Number(delay) || 0));
}
