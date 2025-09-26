import {useSyncExternalStore} from 'react';

// Minimal global bus for composer state (busy/disabled) decoupled from DS loading.

let state = { busy: false };
const listeners = new Set();

function subscribe(cb) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

function getSnapshot() { return state; }

export function setComposerBusy(busy) {
  state = { busy: !!busy };
  for (const l of listeners) l();
}

export function useComposerBusy() {
  return useSyncExternalStore(subscribe, getSnapshot);
}

