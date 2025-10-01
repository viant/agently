import {useSyncExternalStore} from 'react';

// Global visibility for execution details and tool feed
let state = { execution: true, toolFeed: true };
const listeners = new Set();

function subscribe(cb) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

function getSnapshot() { return state; }

export function setExecutionDetailsEnabled(enabled) {
  state = { ...state, execution: !!enabled };
  for (const l of listeners) l();
}

export function setToolFeedEnabled(enabled) {
  state = { ...state, toolFeed: !!enabled };
  for (const l of listeners) l();
}

export function getExecutionDetailsEnabled() {
  return !!state.execution;
}

export function getToolFeedEnabled() {
  return !!state.toolFeed;
}

export function useExecVisibility() {
  return useSyncExternalStore(subscribe, getSnapshot);
}
