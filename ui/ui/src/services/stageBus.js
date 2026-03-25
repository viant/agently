import { useSyncExternalStore } from 'react';

let currentStage = {
  phase: 'ready',
  text: 'Ready',
  updatedAt: Date.now(),
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

export function setStage(next = {}) {
  const phase = normalizeStagePhase(next.phase || currentStage.phase);
  currentStage = {
    ...currentStage,
    ...next,
    phase,
    text: String(next.text || PHASE_LABELS[phase] || ''),
    updatedAt: Date.now()
  };
  for (const listener of listeners) listener();
}

export function useStage() {
  return useSyncExternalStore(subscribe, getSnapshot);
}
