import { useSyncExternalStore } from 'react';

let currentStage = {
  phase: 'ready',
  text: 'Ready',
  updatedAt: Date.now(),
  startedAt: 0,
  completedAt: 0,
  detail: ''
};

const listeners = new Set();

const PHASE_LABELS = {
  ready: 'Ready',
  waiting: 'Waiting',
  thinking: 'Thinking…',
  executing: 'Executing…',
  streaming: 'Streaming response…',
  done: 'Done',
  error: 'Error',
  terminated: 'Terminated',
  offline: 'Service unavailable'
};

export function normalizeStagePhase(phase = '') {
  const raw = String(phase || '').trim().toLowerCase();
  if (!raw) return 'ready';
  switch (raw) {
    case 'idle':
      return 'ready';
    case 'running':
    case 'processing':
    case 'in_progress':
      return 'executing';
    case 'succeeded':
    case 'completed':
    case 'success':
      return 'done';
    case 'failed':
      return 'error';
    case 'cancelled':
    case 'canceled':
    case 'aborted':
      return 'terminated';
    default:
      return raw;
  }
}

function subscribe(cb) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

function getSnapshot() {
  return currentStage;
}

function normalizeStageTimestamp(value) {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value;
  const text = String(value || '').trim();
  if (!text) return 0;
  const parsed = Date.parse(text);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function setStage(next = {}) {
  const phase = normalizeStagePhase(next.phase || currentStage.phase);
  const startedAt = normalizeStageTimestamp(next.startedAt);
  const completedAt = normalizeStageTimestamp(next.completedAt);
  const isActivePhase = phase === 'waiting' || phase === 'thinking' || phase === 'executing' || phase === 'streaming';
  currentStage = {
    ...currentStage,
    ...next,
    phase,
    text: String(next.text || PHASE_LABELS[phase] || ''),
    updatedAt: Date.now(),
    startedAt: startedAt || (isActivePhase ? currentStage.startedAt : 0),
    completedAt: completedAt || (isActivePhase ? 0 : currentStage.completedAt)
  };
  if (startedAt) {
    currentStage.startedAt = startedAt;
  }
  if (completedAt) {
    currentStage.completedAt = completedAt;
  }
  if (!isActivePhase && !completedAt && (phase === 'ready' || phase === 'offline')) {
    currentStage.startedAt = 0;
    currentStage.completedAt = 0;
  }
  for (const listener of listeners) listener();
}

export function useStage() {
  return useSyncExternalStore(subscribe, getSnapshot);
}
