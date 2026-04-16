import { mergeRowSnapshots } from './rowMerge';

const RUNNING_TURN_STATUSES = new Set(['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress']);

function parseStageTimestamp(value) {
  const text = String(value || '').trim();
  if (!text) return 0;
  const parsed = Date.parse(text);
  return Number.isFinite(parsed) ? parsed : 0;
}

function resolveRunningStartedAt(rows = [], runningTurnId = '') {
  const targetTurnId = String(runningTurnId || '').trim();
  if (!targetTurnId) return 0;
  let startedAt = 0;
  for (const row of Array.isArray(rows) ? rows : []) {
    if (String(row?.turnId || '').trim() !== targetTurnId) continue;
    const value = parseStageTimestamp(row?.startedAt || row?.createdAt || '');
    if (!value) continue;
    if (!startedAt || value < startedAt) startedAt = value;
  }
  return startedAt;
}

function filterOwnedTurnRows(rows = [], conversationID = '', ownedConversationID = '', ownedTurnIds = []) {
  const currentID = String(conversationID || '').trim();
  const liveID = String(ownedConversationID || '').trim();
  if (!currentID || !liveID || currentID !== liveID) return Array.isArray(rows) ? rows : [];
  const owned = new Set((Array.isArray(ownedTurnIds) ? ownedTurnIds : []).map((item) => String(item || '').trim()).filter(Boolean));
  if (owned.size === 0) return Array.isArray(rows) ? rows : [];
  return (Array.isArray(rows) ? rows : []).filter((row) => !owned.has(String(row?.turnId || '').trim()));
}

function shouldRecoverWithFullTranscript(chatState = {}) {
  if (chatState?.lastHasRunning) return true;
  const rows = Array.isArray(chatState?.transcriptRows) ? chatState.transcriptRows : [];
  if (rows.length === 0) return false;
  const visibleRows = rows.filter((row) => String(row?._type || '').toLowerCase() !== 'queue');
  if (visibleRows.length === 0) return false;
  const lastRow = visibleRows[visibleRows.length - 1];
  if (String(lastRow?.role || '').toLowerCase() !== 'user') return false;
  const lastTurnId = String(lastRow?.turnId || '').trim();
  if (!lastTurnId) return true;
  const hasAssistantForTurn = visibleRows.some((row) => {
    if (String(row?.role || '').toLowerCase() !== 'assistant') return false;
    if (String(row?.turnId || '').trim() !== lastTurnId) return false;
    if (Number(row?.interim ?? row?.Interim ?? 0) !== 0) return false;
    return String(row?.content || '').trim() !== '';
  });
  return !hasAssistantForTurn;
}

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

  let normalizedLiveRows = anchoredStreamTurnId
    ? (Array.isArray(liveRows) ? liveRows : []).map((row) => {
      if (String(row?._type || '').toLowerCase() !== 'stream') return row;
      if (String(row?.turnId || '').trim()) return row;
      return { ...row, turnId: anchoredStreamTurnId, turnStatus: String(row?.turnStatus || 'running') };
    })
    : (Array.isArray(liveRows) ? liveRows : []);
  const activeStreamRow = [...normalizedLiveRows].reverse().find((row) => {
    if (String(row?._type || '').toLowerCase() !== 'stream') return false;
    if (row?.isStreaming === false) return false;
    const status = String(row?.status || row?.turnStatus || '').trim().toLowerCase();
    if (['completed', 'succeeded', 'success', 'done', 'failed', 'error', 'canceled', 'cancelled', 'terminated'].includes(status)) {
      return false;
    }
    return true;
  });
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
  const sameLiveConversation = conversationID
    && String(chatState.liveOwnedConversationID || '').trim() === conversationID;
  chatState.activeConversationID = conversationID;
  const filteredRows = filterOwnedTurnRows(rows, conversationID, chatState.liveOwnedConversationID, chatState.liveOwnedTurnIds);
  const sameConversation = String(chatState.lastConversationID || '').trim() === conversationID;
  const previousTranscriptRows = filterOwnedTurnRows(
    Array.isArray(chatState.transcriptRows) ? chatState.transcriptRows : [],
    conversationID,
    chatState.liveOwnedConversationID,
    chatState.liveOwnedTurnIds
  );
  const mergedRows = reason === 'poll' && sameConversation && previousTranscriptRows.length > 0
    ? mergeRowSnapshots(previousTranscriptRows, filteredRows)
    : filteredRows;

  // Opening an already-running conversation should bootstrap the active turn
  // from transcript once, then let SSE own that turn going forward. This keeps
  // active-turn rendering on the live side without losing request/detail fields
  // that were persisted before the browser subscribed.
  const effectiveRunningTurnId = String(runningTurnId || findLatestRunningTurnId(mergedRows) || '').trim();
  if (effectiveRunningTurnId && normalizedLiveRows.length === 0 && conversationID) {
    const seeded = mergedRows
      .filter((row) => String(row?.turnId || '').trim() === effectiveRunningTurnId)
      .map((row) => ({ ...row }));
    if (seeded.length > 0) {
      normalizedLiveRows = seeded;
      chatState.liveRows = seeded;
      chatState.liveOwnedConversationID = conversationID;
      chatState.liveOwnedTurnIds = [effectiveRunningTurnId];
    }
  }

  const hasRunning = turns.some((turn) => {
    const status = String(turn?.status || turn?.Status || '').trim().toLowerCase();
    return RUNNING_TURN_STATUSES.has(status);
  });
  const ownedTurnIds = new Set((Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : []).map((item) => String(item || '').trim()).filter(Boolean));
  const transcriptConfirmsOwnedTurnTerminal = !hasRunning
    && ownedTurnIds.size > 0
    && turns.some((turn) => {
      const turnId = String(turn?.turnId || turn?.id || turn?.TurnID || '').trim();
      if (!ownedTurnIds.has(turnId)) return false;
      const status = String(turn?.status || turn?.Status || '').trim().toLowerCase();
      return status !== '' && !RUNNING_TURN_STATUSES.has(status);
    });
  const streamOwnsActiveTurn = sameLiveConversation
    && ownedTurnIds.size > 0
    && !transcriptConfirmsOwnedTurnTerminal;
  const effectiveHasRunning = streamOwnsActiveTurn ? true : hasRunning;

  conversationsDS.setFormData?.({
    values: {
      ...convForm,
      running: effectiveHasRunning
    }
  });

  publishChangeFeed({ conversationId: conversationID, rows: mergedRows });
  publishPlanFeed({ conversationId: conversationID, rows: mergedRows });

  chatState.lastSyncReason = reason;
  chatState.transcriptRows = mergedRows;
  chatState.lastQueuedTurns = queuedTurns;
  chatState.lastHasRunning = effectiveHasRunning;
  chatState.lastConversationID = conversationID;
  chatState.runningTurnId = effectiveHasRunning
    ? (runningTurnId || findLatestRunningTurnId(mergedRows) || chatState.runningTurnId || '')
    : (runningTurnId || findLatestRunningTurnId(mergedRows));

  const transcriptEmpty = !Array.isArray(turns) || turns.length === 0;
  const shouldPreserveTerminalLiveRows = transcriptEmpty && normalizedLiveRows.length > 0;
  const shouldFinalizeActiveStream = (!effectiveHasRunning && !activeStreamRow && !shouldPreserveTerminalLiveRows)
    || transcriptConfirmsOwnedTurnTerminal;
  if (shouldFinalizeActiveStream) {
    chatState.liveRows = [];
    chatState.liveOwnedConversationID = '';
    chatState.liveOwnedTurnIds = [];
    chatState.activeStreamPrompt = '';
    chatState.activeStreamTurnId = '';
    chatState.activeStreamStartedAt = 0;
  }

  if (effectiveHasRunning) {
    setStage({
      phase: 'executing',
      text: 'Assistant executing…',
      startedAt: resolveRunningStartedAt(mergedRows, chatState.runningTurnId)
    });
  } else if (queuedTurns.length > 0) {
    setStage({ phase: 'waiting', text: `Queued turns: ${queuedTurns.length}` });
  } else if (reason === 'poll' || reason === 'fetch') {
    setStage({ phase: 'ready', text: 'Ready' });
  }

  return {
    transcriptRows: mergedRows,
    liveRows: shouldFinalizeActiveStream ? [] : normalizedLiveRows,
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
  let turns = await fetchTranscript(conversationID, since);
  const currentID = String(conversationsDS?.peekFormData?.()?.id || '').trim();
  if (currentID && currentID !== conversationID) {
    return;
  }
  if (since && turns.length === 0) {
    if (!shouldRecoverWithFullTranscript(chatState)) {
      return;
    }
    turns = await fetchTranscript(conversationID, '');
    if (currentID && String(conversationsDS?.peekFormData?.()?.id || '').trim() !== conversationID) {
      return;
    }
    if (turns.length === 0) {
      return;
    }
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
