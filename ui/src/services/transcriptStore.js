import { mergeRowSnapshots } from './rowMerge';
import { isFeedInactive as defaultIsFeedInactive } from './toolFeedBus';
import { isStreamDebugEnabled } from './debugFlags';
import { deriveWorkspaceStateFromTranscriptTurns, ensureWorkspaceWindowForConversation, hasScopedWorkspaceState, seedScopedWorkspaceState } from './conversationWindow';

const RUNNING_TURN_STATUSES = new Set(['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress']);

function transcriptDebugEnabled() {
  return isStreamDebugEnabled();
}

function logTranscriptDebug(event, detail = {}) {
  if (!transcriptDebugEnabled()) return;
  // eslint-disable-next-line no-console
  console.log('[agently-transcript-store]', {
    event,
    ts: new Date().toISOString(),
    ...detail
  });
}

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

function filterSingleTurnRows(rows = [], turnID = '') {
  const targetTurnID = String(turnID || '').trim();
  if (!targetTurnID) return Array.isArray(rows) ? rows : [];
  return (Array.isArray(rows) ? rows : []).filter((row) => String(row?.turnId || '').trim() !== targetTurnID);
}

function latestTranscriptTurnId(turns = []) {
  for (let index = (Array.isArray(turns) ? turns.length : 0) - 1; index >= 0; index -= 1) {
    const turn = turns[index] || {};
    const turnId = String(turn?.turnId || turn?.id || turn?.TurnID || '').trim();
    if (turnId) return turnId;
  }
  return '';
}

function transcriptHasSettledAssistantRow(rows = [], turnId = '') {
  const targetTurnId = String(turnId || '').trim();
  if (!targetTurnId) return false;
  return (Array.isArray(rows) ? rows : []).some((row) => {
    if (String(row?.turnId || '').trim() !== targetTurnId) return false;
    if (String(row?.role || '').trim().toLowerCase() !== 'assistant') return false;
    if (Number(row?.interim ?? row?.Interim ?? 0) !== 0) return false;
    if (String(row?.content || '').trim() !== '') return true;
    const executionGroups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    return executionGroups.some((group) => {
      if (!group?.finalResponse) return false;
      return String(group?.content || '').trim() !== '' || (Array.isArray(group?.toolSteps) && group.toolSteps.length > 0);
    });
  });
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
  applyFeedEvent,
  isFeedInactive = defaultIsFeedInactive,
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
  const pendingLocalLiveBootstrap = sameLiveConversation
    && String(chatState.activeStreamPrompt || '').trim() !== '';
  const originalOwnedTurnIds = Array.isArray(chatState.liveOwnedTurnIds)
    ? chatState.liveOwnedTurnIds.map((item) => String(item || '').trim()).filter(Boolean)
    : [];
  const hasRunning = turns.some((turn) => {
    const status = String(turn?.status || turn?.Status || '').trim().toLowerCase();
    return RUNNING_TURN_STATUSES.has(status);
  });
  const effectiveHasRunning = hasRunning;
  const latestTurnId = latestTranscriptTurnId(turns);
  const latestRunningTurnIdFromTranscript = effectiveHasRunning ? latestTurnId : '';
  const latestTurnStatus = String(
    [...(Array.isArray(turns) ? turns : [])]
      .reverse()
      .find((turn) => String(turn?.turnId || turn?.id || turn?.TurnID || '').trim() === latestTurnId)
      ?.status || ''
  ).trim().toLowerCase();
  const shouldReleaseLatestOwnedTurn = sameLiveConversation
    && latestTurnId !== ''
    && originalOwnedTurnIds.includes(latestTurnId)
    && !RUNNING_TURN_STATUSES.has(latestTurnStatus)
    && !activeStreamRow
    && transcriptHasSettledAssistantRow(rows, latestTurnId)
    && !pendingLocalLiveBootstrap;
  const effectiveOwnedTurnIds = shouldReleaseLatestOwnedTurn
    ? originalOwnedTurnIds.filter((turnId) => turnId !== latestTurnId)
    : originalOwnedTurnIds;
  if (shouldReleaseLatestOwnedTurn) {
    chatState.liveOwnedTurnIds = effectiveOwnedTurnIds;
    if (effectiveOwnedTurnIds.length === 0) {
      chatState.liveOwnedConversationID = '';
    }
    logTranscriptDebug('release_live_owned_turn', {
      conversationID,
      releasedTurnId: latestTurnId,
      remainingOwnedTurnIds: effectiveOwnedTurnIds
    });
  }
  let filteredRows = filterOwnedTurnRows(rows, conversationID, chatState.liveOwnedConversationID, chatState.liveOwnedTurnIds);
  const sameConversation = String(chatState.lastConversationID || '').trim() === conversationID;
  let previousTranscriptRows = filterOwnedTurnRows(
    Array.isArray(chatState.transcriptRows) ? chatState.transcriptRows : [],
    conversationID,
    chatState.liveOwnedConversationID,
    chatState.liveOwnedTurnIds
  );
  const effectiveRunningTurnId = String(
    runningTurnId
    || findLatestRunningTurnId(filteredRows)
    || findLatestRunningTurnId(previousTranscriptRows)
    || latestRunningTurnIdFromTranscript
    || ''
  ).trim();
  if (effectiveHasRunning && conversationID && effectiveRunningTurnId) {
    chatState.liveOwnedConversationID = conversationID;
    const existingOwned = new Set(
      (Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [])
        .map((item) => String(item || '').trim())
        .filter(Boolean)
    );
    existingOwned.add(effectiveRunningTurnId);
    chatState.liveOwnedTurnIds = Array.from(existingOwned);
    logTranscriptDebug('claim_live_owned_turn', {
      conversationID,
      runningTurnId: effectiveRunningTurnId,
      liveOwnedTurnIds: chatState.liveOwnedTurnIds
    });
  }
  if (effectiveHasRunning && conversationID && effectiveRunningTurnId) {
    logTranscriptDebug('filter_running_turn_from_transcript', {
      conversationID,
      runningTurnId: effectiveRunningTurnId,
      transcriptRowsBefore: filteredRows.length,
      previousRowsBefore: previousTranscriptRows.length
    });
    filteredRows = filterSingleTurnRows(filteredRows, effectiveRunningTurnId);
    previousTranscriptRows = filterSingleTurnRows(previousTranscriptRows, effectiveRunningTurnId);
  }
  if (pendingLocalLiveBootstrap && effectiveRunningTurnId) {
    filteredRows = filteredRows.filter((row) => String(row?.turnId || '').trim() !== effectiveRunningTurnId);
    previousTranscriptRows = previousTranscriptRows.filter((row) => String(row?.turnId || '').trim() !== effectiveRunningTurnId);
  }
  const finalMergedRows = reason === 'poll' && sameConversation && previousTranscriptRows.length > 0
    ? mergeRowSnapshots(previousTranscriptRows, filteredRows)
    : filteredRows;
  logTranscriptDebug('sync_complete', {
    conversationID,
    reason,
    transcriptTurnCount: Array.isArray(turns) ? turns.length : 0,
    transcriptRowCount: rows.length,
    filteredRowCount: filteredRows.length,
    finalRowCount: finalMergedRows.length,
    hasRunning: effectiveHasRunning,
    runningTurnId: effectiveRunningTurnId,
    liveOwnedConversationID: chatState.liveOwnedConversationID,
    liveOwnedTurnIds: chatState.liveOwnedTurnIds
  });

  const ownedTurnIds = new Set((Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : []).map((item) => String(item || '').trim()).filter(Boolean));
  const liveOwnsLatestTurn = sameLiveConversation
    && ownedTurnIds.size > 0
    && latestTurnId !== ''
    && ownedTurnIds.has(latestTurnId);

  conversationsDS.setFormData?.({
    values: {
      ...convForm,
      running: effectiveHasRunning
    }
  });

  const transcriptFeedsByConversation = chatState.lastTranscriptFeedsByConversation || {};
  const transcriptFeeds = Array.isArray(transcriptFeedsByConversation[conversationID])
    ? transcriptFeedsByConversation[conversationID]
    : [];
  if (conversationID && typeof applyFeedEvent === 'function' && transcriptFeeds.length > 0) {
    for (const feed of transcriptFeeds) {
      const feedId = String(feed?.feedId || '').trim();
      if (!feedId) continue;
      if (typeof isFeedInactive === 'function' && isFeedInactive(feedId, conversationID)) continue;
      applyFeedEvent({
        type: 'tool_feed_active',
        feedId,
        feedTitle: feed.title || feedId,
        feedItemCount: feed.itemCount || 0,
        feedData: feed.data || null,
        conversationId: conversationID,
      });
    }
  }

  chatState.lastSyncReason = reason;
  chatState.transcriptRows = finalMergedRows;
  chatState.lastQueuedTurns = queuedTurns;
  chatState.lastHasRunning = effectiveHasRunning;
  chatState.lastConversationID = conversationID;
  if (conversationID && !hasScopedWorkspaceState(conversationID)) {
    const derivedWorkspaceState = deriveWorkspaceStateFromTranscriptTurns(turns);
    if (derivedWorkspaceState) {
      seedScopedWorkspaceState(conversationID, derivedWorkspaceState);
      ensureWorkspaceWindowForConversation(conversationID);
    }
  }
  chatState.runningTurnId = effectiveHasRunning
    ? (runningTurnId || findLatestRunningTurnId(finalMergedRows) || chatState.runningTurnId || '')
    : (runningTurnId || findLatestRunningTurnId(finalMergedRows));

  const transcriptEmpty = !Array.isArray(turns) || turns.length === 0;
  const shouldPreserveTerminalLiveRows = transcriptEmpty && normalizedLiveRows.length > 0;
  const shouldFinalizeActiveStream = !liveOwnsLatestTurn
    && !effectiveHasRunning
    && !activeStreamRow
    && !shouldPreserveTerminalLiveRows;
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
      startedAt: resolveRunningStartedAt(finalMergedRows, chatState.runningTurnId)
    });
  } else if (queuedTurns.length > 0) {
    setStage({ phase: 'waiting', text: `Queued turns: ${queuedTurns.length}` });
  } else if (reason === 'poll' || reason === 'fetch') {
    setStage({ phase: 'ready', text: 'Ready' });
  }

  return {
    transcriptRows: finalMergedRows,
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
  const transcriptOptions = options?.transcript && typeof options.transcript === 'object'
    ? options.transcript
    : {};
  const hasPrefetchedTurns = Array.isArray(options?.prefetchedTranscriptTurns);
  let turns = hasPrefetchedTurns
    ? options.prefetchedTranscriptTurns
    : await fetchTranscript(conversationID, since, transcriptOptions);
  const currentID = String(conversationsDS?.peekFormData?.()?.id || '').trim();
  if (currentID && currentID !== conversationID) {
    return;
  }
  if (!hasPrefetchedTurns && since && turns.length === 0) {
    if (!shouldRecoverWithFullTranscript(chatState)) {
      return;
    }
    turns = await fetchTranscript(conversationID, '', transcriptOptions);
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
  const pendingElicitations = Array.isArray(options?.prefetchedPendingElicitations)
    ? options.prefetchedPendingElicitations
    : await fetchPendingElicitations(conversationID);
  return doSync({ context, turns, pendingElicitations, reason: 'poll' });
}

export function resetTranscriptState({
  context,
  ensureContextResources,
  getCurrentConversationID
}) {
  const chatState = ensureContextResources(context);
  chatState.lastSinceCursor = '';
  chatState.transcriptRows = [];
  chatState.lastTranscriptFeedsByConversation = {};
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
  const schedule = typeof window !== 'undefined' && typeof window.setTimeout === 'function'
    ? window.setTimeout.bind(window)
    : (typeof globalThis.setTimeout === 'function' ? globalThis.setTimeout.bind(globalThis) : null);
  const clearSchedule = typeof window !== 'undefined' && typeof window.clearTimeout === 'function'
    ? window.clearTimeout.bind(window)
    : (typeof globalThis.clearTimeout === 'function' ? globalThis.clearTimeout.bind(globalThis) : null);
  if (!schedule || !clearSchedule) {
    return null;
  }
  if (resetSince) {
    doReset({ context });
  }
  if (chatState.refreshTimer) {
    clearSchedule(chatState.refreshTimer);
    chatState.refreshTimer = null;
  }
  chatState.refreshTimer = schedule(async () => {
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
