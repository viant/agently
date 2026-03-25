import { mergeRowSnapshots } from './rowMerge';

const COMPOSER_DRAFTS_KEY = 'agently.composerDrafts.v1';

function readDraftMap() {
  if (typeof window === 'undefined') return {};
  try {
    const raw = window.sessionStorage?.getItem(COMPOSER_DRAFTS_KEY) || '{}';
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (_) {
    return {};
  }
}

function writeDraftMap(value = {}) {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage?.setItem(COMPOSER_DRAFTS_KEY, JSON.stringify(value || {}));
  } catch (_) {}
}

function seedComposerDraftFromTranscript(conversationID = '', rows = []) {
  const id = String(conversationID || '').trim();
  if (!id || typeof window === 'undefined') return;
  const map = readDraftMap();
  if (String(map[id] || '').trim()) return;
  const firstUser = (Array.isArray(rows) ? rows : []).find((row) => String(row?.role || '').toLowerCase() === 'user' && String(row?.content || '').trim());
  const prompt = String(firstUser?.content || '').trim();
  if (!prompt) return;
  map[id] = prompt;
  writeDraftMap(map);
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
  seedComposerDraftFromTranscript(conversationID, mergedRows);
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
