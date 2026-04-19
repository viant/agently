/**
 * services/chatStore.js — the single client chat state store.
 *
 * Owns one `ClientConversationState` per conversation, fed by:
 *   - submit()      → applyLocalSubmit (bootstrap + queued follow-up)
 *   - steer()       → applyLocalSubmit (explicit mid-turn steering)
 *   - onSSE()       → applyEvent       (live deltas, streaming.Event shape)
 *   - onTranscript()→ applyTranscript  (persisted api.ConversationState)
 *
 * Exposes:
 *   - getState(conversationId)          → ClientConversationState
 *   - getProjection(conversationId)     → RenderRow[]   (cached per version)
 *   - subscribe(listener)               → unsubscribe
 *   - hooks for React: useChatProjection, useChatIsRunning, useChatIsQueued,
 *     useChatUsers
 *
 * Nothing in this file invents a new wire shape. SSE events pass through as
 * `streaming.Event`; transcript snapshots pass through as
 * `api.ConversationState`. See ui-improvement.md §0 core principle.
 */

import { useSyncExternalStore } from 'react';
import {
    chatStoreApplyEvent as applyEvent,
    chatStoreApplyLocalSubmit as applyLocalSubmit,
    chatStoreApplyTranscript as applyTranscript,
    chatStoreNewConversationState as newConversationState,
    chatStoreProjectConversation as projectConversation,
} from 'agently-core-ui-sdk';

// ─── Per-conversation state ───────────────────────────────────────────────────

/**
 * Map<conversationId, {
 *   state: ClientConversationState,
 *   projection: RenderRow[],
 *   version: number,
 * }>
 *
 * The projection is recomputed lazily on getProjection() and cached until
 * the next mutation bumps the version. React sees a new projection array
 * reference whenever anything changed (and the same reference otherwise),
 * so useSyncExternalStore behaves correctly.
 */
const conversations = new Map();

/** Global subscriber set — any mutation to any conversation notifies all. */
const listeners = new Set();

function entry(conversationId) {
    let e = conversations.get(conversationId);
    if (!e) {
        e = {
            state: newConversationState(conversationId),
            projection: null,
            version: 0,
        };
        conversations.set(conversationId, e);
    }
    return e;
}

function bump(e) {
    e.version += 1;
    e.projection = null;      // invalidate projection cache
    for (const listener of listeners) {
        try { listener(); } catch (_) { /* ignore listener errors */ }
    }
}

// ─── Public API ────────────────────────────────────────────────────────────────

/** Return the raw client canonical state for the given conversation. */
export function getState(conversationId) {
    return entry(conversationId).state;
}

/**
 * Return the render-row projection for the conversation. The returned array
 * reference is stable between mutations — React can compare by identity.
 */
export function getProjection(conversationId) {
    const e = entry(conversationId);
    if (e.projection === null) {
        e.projection = projectConversation(e.state);
    }
    return e.projection;
}

/** Subscribe to any mutation of any conversation. Returns unsubscribe. */
export function subscribe(listener) {
    listeners.add(listener);
    return () => listeners.delete(listener);
}

/**
 * Submit a user message locally. Creates a pending turn or appends a
 * steering user segment to the currently-active turn (§4.1 / §6.8).
 *
 * Throws on duplicate clientRequestId or conversation mismatch (§3.2).
 */
export function submit({ conversationId, clientRequestId, content, createdAt, attachments }) {
    const e = entry(conversationId);
    applyLocalSubmit(e.state, {
        conversationId,
        clientRequestId,
        content,
        createdAt: createdAt ?? new Date().toISOString(),
        attachments,
        mode: 'submit',
    });
    bump(e);
}

/** Append a user message to the currently-active live turn as an explicit steer. */
export function steer({ conversationId, clientRequestId, content, createdAt, attachments }) {
    const e = entry(conversationId);
    applyLocalSubmit(e.state, {
        conversationId,
        clientRequestId,
        content,
        createdAt: createdAt ?? new Date().toISOString(),
        attachments,
        mode: 'steer',
    });
    bump(e);
}

/** Apply one SSE event to the store. Event is the backend streaming.Event shape. */
export function onSSE(conversationId, event) {
    if (!event || !conversationId) return;
    // Guard against cross-conversation events (reducer would also check).
    if (event.conversationId && event.conversationId !== conversationId) return;
    const e = entry(conversationId);
    applyEvent(e.state, event);
    bump(e);
}

/**
 * Apply one transcript snapshot to the store. `snapshot` is the backend
 * canonical api.ConversationState shape. `applyTranscript` is idempotent;
 * calling with the same snapshot twice is a no-op.
 */
export function onTranscript(conversationId, snapshot) {
    if (!snapshot || !conversationId) return;
    if (snapshot.conversationId && snapshot.conversationId !== conversationId) return;
    const e = entry(conversationId);
    applyTranscript(e.state, snapshot);
    bump(e);
}

/**
 * For tests or conversation-switch flows: drop all state for a conversation.
 * The next read creates a fresh empty state.
 */
export function reset(conversationId) {
    if (conversations.has(conversationId)) {
        conversations.delete(conversationId);
        for (const listener of listeners) {
            try { listener(); } catch (_) { /* ignore */ }
        }
    }
}

/** For tests: drop everything. */
export function __resetAll() {
    conversations.clear();
    for (const listener of listeners) {
        try { listener(); } catch (_) { /* ignore */ }
    }
}

// ─── Derived selectors ────────────────────────────────────────────────────────

/**
 * Iteration rows from the projection — convenience selector since Chat.jsx
 * mostly wants to know "is there a running turn?" / "queued?".
 */
function selectIterationRows(conversationId) {
    return getProjection(conversationId).filter((r) => r.kind === 'iteration');
}

/** `true` iff any turn in the conversation has lifecycle ∈ {pending, running}. */
export function isRunning(conversationId) {
    for (const row of selectIterationRows(conversationId)) {
        if (row.lifecycle === 'pending' || row.lifecycle === 'running') return true;
    }
    return false;
}

/**
 * `true` iff there is a pending-bootstrap turn that has not yet been echoed
 * by the server. This is the "queued during an already-running turn" case.
 * The first pending turn is the one the user is waiting on; additional
 * pending turns indicate queued follow-ups.
 */
export function isQueued(conversationId) {
    const rows = selectIterationRows(conversationId);
    const liveTurns = rows.filter((row) => row.lifecycle === 'pending' || row.lifecycle === 'running');
    return liveTurns.length >= 2;
}

// ─── React hooks ──────────────────────────────────────────────────────────────

/**
 * Subscribe to the projection for a given conversation. Returns the current
 * RenderRow[]. Re-renders when the projection changes (new array reference).
 */
export function useChatProjection(conversationId) {
    const snap = () => (conversationId ? getProjection(conversationId) : EMPTY_PROJECTION);
    return useSyncExternalStore(subscribe, snap, snap);
}

export function useChatIsRunning(conversationId) {
    const snap = () => (conversationId ? isRunning(conversationId) : false);
    return useSyncExternalStore(subscribe, snap, snap);
}

export function useChatIsQueued(conversationId) {
    const snap = () => (conversationId ? isQueued(conversationId) : false);
    return useSyncExternalStore(subscribe, snap, snap);
}

const EMPTY_PROJECTION = Object.freeze([]);
