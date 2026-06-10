const GOAL_DRAFT_OPEN_EVENT = 'agently:goal-draft-open';

export function requestGoalDraftOpen(detail = {}) {
  if (typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new CustomEvent(GOAL_DRAFT_OPEN_EVENT, { detail: detail || {} }));
  } catch (_) {}
}

export function onGoalDraftOpen(handler) {
  if (typeof window === 'undefined' || typeof handler !== 'function') {
    return () => {};
  }
  const listener = (event) => {
    try {
      handler(event?.detail || {});
    } catch (_) {}
  };
  window.addEventListener(GOAL_DRAFT_OPEN_EVENT, listener);
  return () => window.removeEventListener(GOAL_DRAFT_OPEN_EVENT, listener);
}

