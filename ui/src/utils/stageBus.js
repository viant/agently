import {useSyncExternalStore} from 'react';

// Very small global pub/sub for the current conversation stage.
// We avoid bringing full-blown state libraries; React 18 hook keeps things simple.

let currentStage = {phase: 'ready'}; // default state
const listeners = new Set();

export function normalizeStagePhase(phase) {
    const raw = String(phase || '').trim();
    const p = raw.toLowerCase();
    if (!p) return 'ready';
    switch (p) {
        case 'ready':
        case 'idle':
            return 'ready';
        case 'thinking':
            return 'thinking';
        case 'running':
        case 'open':
        case 'pending':
        case 'processing':
        case 'streaming':
        case 'retrying':
        case 'executing':
            return 'executing';
        case 'waiting_for_user':
        case 'waiting-for-user':
        case 'elicitation':
            return 'elicitation';
        case 'succeeded':
        case 'success':
        case 'accepted':
        case 'completed':
        case 'done':
            return 'done';
        case 'failed':
        case 'error':
        case 'rejected':
            return 'error';
        case 'canceled':
        case 'cancelled':
        case 'aborted':
        case 'terminated':
        case 'timed_out':
        case 'timeout':
        case 'deadline_exceeded':
            return 'terminated';
        default:
            return p;
    }
}

// Subscribe function required by useSyncExternalStore.
function subscribe(cb) {
    listeners.add(cb);
    return () => listeners.delete(cb);
}

function getSnapshot() {
    return currentStage;
}

export function setStage(stage) {
    const next = stage || {phase: 'ready'};
    currentStage = {
        ...next,
        phase: normalizeStagePhase(next.phase),
    };
    for (const l of listeners) l();
}

export function useStage() {
    return useSyncExternalStore(subscribe, getSnapshot);
}
