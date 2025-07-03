import {useSyncExternalStore} from 'react';

// Very small global pub/sub for the current conversation stage.
// We avoid bringing full-blown state libraries; React 18 hook keeps things simple.

let currentStage = {phase: 'ready'}; // default state
const listeners = new Set();

// Subscribe function required by useSyncExternalStore.
function subscribe(cb) {
    listeners.add(cb);
    return () => listeners.delete(cb);
}

function getSnapshot() {
    return currentStage;
}

export function setStage(stage) {
    currentStage = stage || {phase: 'ready'};
    for (const l of listeners) l();
}

export function useStage() {
    return useSyncExternalStore(subscribe, getSnapshot);
}
