import { useSyncExternalStore } from 'react';

export const PLAN_FEED_ANCHORS = {
  COMPOSER_TOP: 'composer_top',
  SIDEBAR_TOP: 'sidebar_top',
  SIDEBAR_BOTTOM: 'sidebar_bottom'
};

const DEFAULT_PLAN_FEED_ANCHOR = PLAN_FEED_ANCHORS.COMPOSER_TOP;
const listeners = new Set();

let currentPlanFeed = {
  anchor: readStoredAnchor(),
  conversationId: '',
  explanation: '',
  steps: [],
  source: null,
  updatedAt: 0
};

function readStoredAnchor() {
  if (typeof window === 'undefined') return DEFAULT_PLAN_FEED_ANCHOR;
  return normalizePlanFeedAnchor(window.localStorage?.getItem('agently.planFeedAnchor'));
}

export function normalizePlanFeedAnchor(value = '') {
  switch (String(value || '').trim().toLowerCase()) {
    case PLAN_FEED_ANCHORS.SIDEBAR_TOP:
      return PLAN_FEED_ANCHORS.SIDEBAR_TOP;
    case PLAN_FEED_ANCHORS.SIDEBAR_BOTTOM:
      return PLAN_FEED_ANCHORS.SIDEBAR_BOTTOM;
    default:
      return PLAN_FEED_ANCHORS.COMPOSER_TOP;
  }
}

function notify() {
  for (const listener of listeners) listener();
}

function subscribe(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return currentPlanFeed;
}

export function peekPlanFeed() {
  return getSnapshot();
}

function parseMaybeJSON(value) {
  if (!value) return null;
  if (typeof value === 'string') {
    const text = value.trim();
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch (_) {
      return null;
    }
  }
  if (typeof value !== 'object') return null;
  const inlineBody = typeof value?.inlineBody === 'string' ? value.inlineBody.trim() : '';
  if (inlineBody) {
    try {
      return JSON.parse(inlineBody);
    } catch (_) {
      return null;
    }
  }
  return value;
}

function normalizeToolKey(value = '') {
  return String(value || '').toLowerCase().replace(/[^a-z0-9]/g, '');
}

function isPlanTool(step = {}) {
  const key = normalizeToolKey(step?.toolName || '');
  return key.includes('orchestration') && key.includes('updateplan');
}

function normalizeStep(step = {}, index = 0) {
  const title = String(step?.step || step?.title || step?.name || '').trim();
  const status = String(step?.status || '').trim().toLowerCase() || 'pending';
  if (!title) return null;
  return {
    id: String(step?.id || `${index}:${title}`),
    step: title,
    status
  };
}

function extractPlanPayload(payload = null) {
  const parsed = parseMaybeJSON(payload);
  if (!parsed || typeof parsed !== 'object') return null;
  const explanation = String(
    parsed?.explanation
    || parsed?.Explanation
    || parsed?.output?.explanation
    || parsed?.output?.Explanation
    || ''
  ).trim();
  const rawPlan = parsed?.plan
    || parsed?.Plan
    || parsed?.output?.plan
    || parsed?.output?.Plan
    || parsed?.result?.plan
    || parsed?.result?.Plan
    || [];
  const steps = Array.isArray(rawPlan)
    ? rawPlan.map((step, index) => normalizeStep(step, index)).filter(Boolean)
    : [];
  if (!explanation && steps.length === 0) return null;
  return { explanation, steps, raw: parsed };
}

function extractLatestPlan(rows = []) {
  for (let rowIndex = rows.length - 1; rowIndex >= 0; rowIndex--) {
    const row = rows[rowIndex];
    const candidateSteps = [];
    const executions = Array.isArray(row?.executions) ? row.executions : [];
    for (let execIndex = executions.length - 1; execIndex >= 0; execIndex--) {
      const execution = executions[execIndex];
      const steps = Array.isArray(execution?.steps) ? execution.steps : [];
      for (let stepIndex = steps.length - 1; stepIndex >= 0; stepIndex--) {
        candidateSteps.push(steps[stepIndex]);
      }
    }
    const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    for (let groupIndex = groups.length - 1; groupIndex >= 0; groupIndex--) {
      const group = groups[groupIndex];
      const toolSteps = Array.isArray(group?.toolSteps) ? group.toolSteps : [];
      for (let stepIndex = toolSteps.length - 1; stepIndex >= 0; stepIndex--) {
        candidateSteps.push(toolSteps[stepIndex]);
      }
    }
    for (const step of candidateSteps) {
      if (!isPlanTool(step)) continue;
      const parsed = extractPlanPayload(step?.responsePayload)
        || extractPlanPayload(step?.requestPayload);
      if (!parsed) continue;
      return {
        explanation: parsed.explanation,
        steps: parsed.steps,
        source: step,
        raw: parsed.raw,
        createdAt: String(row?.createdAt || '').trim()
      };
    }
  }
  return null;
}

export function setPlanFeedAnchor(anchor = DEFAULT_PLAN_FEED_ANCHOR) {
  const normalized = normalizePlanFeedAnchor(anchor);
  if (typeof window !== 'undefined') {
    try {
      window.localStorage?.setItem('agently.planFeedAnchor', normalized);
    } catch (_) {}
  }
  currentPlanFeed = {
    ...currentPlanFeed,
    anchor: normalized,
    updatedAt: Date.now()
  };
  notify();
}

export function publishPlanFeed({ conversationId = '', rows = [] } = {}) {
  const normalizedConversationId = String(conversationId || '').trim();
  const latest = extractLatestPlan(Array.isArray(rows) ? rows : []);
  const sameConversation = normalizedConversationId !== ''
    && normalizedConversationId === String(currentPlanFeed?.conversationId || '').trim();
  if (!latest && sameConversation) {
    return;
  }
  currentPlanFeed = {
    ...currentPlanFeed,
    conversationId: normalizedConversationId,
    explanation: String(latest?.explanation || '').trim(),
    steps: Array.isArray(latest?.steps) ? latest.steps : [],
    source: latest?.source || null,
    updatedAt: Date.now()
  };
  notify();
}

export function clearPlanFeed(conversationId = '') {
  currentPlanFeed = {
    ...currentPlanFeed,
    conversationId: String(conversationId || '').trim(),
    explanation: '',
    steps: [],
    source: null,
    updatedAt: Date.now()
  };
  notify();
}

export function usePlanFeed() {
  return useSyncExternalStore(subscribe, getSnapshot);
}
