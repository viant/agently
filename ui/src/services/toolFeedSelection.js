let expandedFeeds = new Set();
let selectedFeedByConversation = new Map();
let lastSelectedFeedId = '';
const expandListeners = new Set();
const selectedFeedListeners = new Set();
let feedDataLoader = null;

function splitFeedKey(feedKey = '') {
  const raw = String(feedKey || '').trim();
  if (!raw) return { feedId: '', conversationId: '' };
  const idx = raw.indexOf('::');
  if (idx === -1) return { feedId: raw, conversationId: '' };
  return {
    conversationId: raw.slice(0, idx).trim(),
    feedId: raw.slice(idx + 2).trim()
  };
}

function selectionScope(feedId = '', conversationId = '') {
  const directConversationId = String(conversationId || '').trim();
  if (directConversationId) return directConversationId;
  return String(splitFeedKey(feedId).conversationId || '').trim();
}

function firstExpandedFeedForConversation(conversationId = '') {
  const targetConversationId = String(conversationId || '').trim();
  for (const feedId of expandedFeeds) {
    if (selectionScope(feedId) === targetConversationId) {
      return feedId;
    }
  }
  return '';
}

function notifyExpand() {
  for (const fn of expandListeners) fn(new Set(expandedFeeds));
}

function notifySelectedFeed() {
  for (const fn of selectedFeedListeners) fn(lastSelectedFeedId);
}

function fetchFeedIfNeeded(feedId, conversationId) {
  if (conversationId && typeof feedDataLoader === 'function') {
    feedDataLoader(feedId, conversationId);
  }
}

export function getExpandedFeedIds() {
  return new Set(expandedFeeds);
}

export function isFeedExpanded(feedId) {
  return expandedFeeds.has(feedId);
}

export function getSelectedFeedId(conversationId = '') {
  const scope = String(conversationId || '').trim();
  if (!scope) return lastSelectedFeedId;
  return String(selectedFeedByConversation.get(scope) || '').trim();
}

export function onFeedExpansionChange(fn) {
  expandListeners.add(fn);
  return () => expandListeners.delete(fn);
}

export function onSelectedFeedChange(fn) {
  selectedFeedListeners.add(fn);
  return () => selectedFeedListeners.delete(fn);
}

export function registerFeedDataLoader(fn) {
  feedDataLoader = typeof fn === 'function' ? fn : null;
}

export function reconcileFeedSelection(feeds = []) {
  const ids = new Set(
    (Array.isArray(feeds) ? feeds : [])
      .map((feed) => String(feed?.feedId || '').trim())
      .filter(Boolean)
  );
  const scopes = new Set(
    Array.from(ids)
      .map((feedId) => selectionScope(feedId))
  );
  if (ids.size === 0) {
    if (expandedFeeds.size > 0) {
      expandedFeeds = new Set();
      notifyExpand();
    }
    if (selectedFeedByConversation.size > 0 || lastSelectedFeedId) {
      selectedFeedByConversation = new Map();
      lastSelectedFeedId = '';
      notifySelectedFeed();
    }
    return;
  }

  const nextExpanded = new Set(Array.from(expandedFeeds).filter((feedId) => ids.has(feedId)));
  if (nextExpanded.size !== expandedFeeds.size) {
    expandedFeeds = nextExpanded;
    notifyExpand();
  }

  let selectionChanged = false;
  const nextSelected = new Map();
  for (const [scope, feedId] of selectedFeedByConversation.entries()) {
    if (ids.has(feedId)) {
      nextSelected.set(scope, feedId);
    } else {
      selectionChanged = true;
    }
  }
  for (const scope of scopes) {
    if (nextSelected.has(scope)) continue;
    const fallback = firstExpandedFeedForConversation(scope)
      || Array.from(ids).find((feedId) => selectionScope(feedId) === scope)
      || '';
    if (!fallback) continue;
    nextSelected.set(scope, fallback);
    if (!expandedFeeds.has(fallback)) {
      expandedFeeds = new Set([...expandedFeeds, fallback]);
      notifyExpand();
    }
    selectionChanged = true;
  }
  const nextLastSelectedFeedId = Array.from(nextSelected.values()).slice(-1)[0] || '';
  if (selectionChanged || nextLastSelectedFeedId !== lastSelectedFeedId || nextSelected.size !== selectedFeedByConversation.size) {
    selectedFeedByConversation = nextSelected;
    lastSelectedFeedId = nextLastSelectedFeedId;
    notifySelectedFeed();
  }
}

export function expandFeed(feedId, conversationId) {
  if (!feedId) return;
  if (!expandedFeeds.has(feedId)) {
    expandedFeeds.add(feedId);
    notifyExpand();
  }
  const scope = selectionScope(feedId, conversationId);
  if (String(selectedFeedByConversation.get(scope) || '').trim() !== feedId || lastSelectedFeedId !== feedId) {
    if (scope) {
      selectedFeedByConversation.set(scope, feedId);
    }
    lastSelectedFeedId = feedId;
    notifySelectedFeed();
  }
  fetchFeedIfNeeded(feedId, conversationId);
}

export function activateExclusiveFeed(feedId, conversationId) {
  if (!feedId) return;
  const scope = selectionScope(feedId, conversationId);
  expandedFeeds = new Set(
    Array.from(expandedFeeds).filter((candidateId) => selectionScope(candidateId) !== scope)
  );
  expandedFeeds.add(feedId);
  notifyExpand();
  if (String(selectedFeedByConversation.get(scope) || '').trim() !== feedId || lastSelectedFeedId !== feedId) {
    if (scope) {
      selectedFeedByConversation.set(scope, feedId);
    }
    lastSelectedFeedId = feedId;
    notifySelectedFeed();
  }
  fetchFeedIfNeeded(feedId, conversationId);
}

export function collapseFeed(feedId) {
  if (!feedId) return;
  if (!expandedFeeds.has(feedId)) return;
  expandedFeeds.delete(feedId);
  const scope = selectionScope(feedId);
  if (String(selectedFeedByConversation.get(scope) || '').trim() === feedId) {
    const fallback = firstExpandedFeedForConversation(scope);
    if (fallback) {
      selectedFeedByConversation.set(scope, fallback);
      lastSelectedFeedId = fallback;
    } else {
      selectedFeedByConversation.delete(scope);
      if (lastSelectedFeedId === feedId) {
        lastSelectedFeedId = '';
      }
    }
    notifySelectedFeed();
  }
  notifyExpand();
}

export function toggleFeedExpanded(feedId, conversationId) {
  if (expandedFeeds.has(feedId)) {
    collapseFeed(feedId);
  } else {
    expandFeed(feedId, conversationId);
  }
}

export function clearFeedSelection() {
  const hadExpanded = expandedFeeds.size > 0;
  const hadSelection = selectedFeedByConversation.size > 0 || lastSelectedFeedId !== '';
  expandedFeeds = new Set();
  selectedFeedByConversation = new Map();
  lastSelectedFeedId = '';
  if (hadExpanded) {
    notifyExpand();
  }
  if (hadSelection) {
    notifySelectedFeed();
  }
}

export function clearFeedSelectionForConversation(conversationId = '') {
  const scope = String(conversationId || '').trim();
  if (!scope) return;
  const nextExpanded = new Set(
    Array.from(expandedFeeds).filter((feedId) => selectionScope(feedId) !== scope)
  );
  const hadExpandedChange = nextExpanded.size !== expandedFeeds.size;
  expandedFeeds = nextExpanded;

  const nextSelected = new Map(selectedFeedByConversation);
  const hadSelected = nextSelected.delete(scope);
  selectedFeedByConversation = nextSelected;
  if (hadSelected || lastSelectedFeedId) {
    const selectedValues = Array.from(nextSelected.values());
    const nextLast = selectedValues[selectedValues.length - 1] || '';
    const hadLastSelectedChange = nextLast !== lastSelectedFeedId;
    lastSelectedFeedId = nextLast;
    if (hadSelected || hadLastSelectedChange) {
      notifySelectedFeed();
    }
  }
  if (hadExpandedChange) {
    notifyExpand();
  }
}

export function __resetToolFeedSelectionForTest() {
  clearFeedSelection();
  feedDataLoader = null;
}
