/**
 * chatStore.js — integration-ish tests on the JS wrapper.
 * Contract unit tests live on the SDK side (agently-core-ui-sdk). This file
 * covers the JS-specific concerns: module-level caching, per-conversation
 * isolation, subscribe/notify behaviour, hook-shape invariants.
 */

import { describe, it, expect, beforeEach } from 'vitest';

import {
    __resetAll,
    getProjection,
    getState,
    isQueued,
    isRunning,
    onSSE,
    onTranscript,
    reset,
    steer,
    submit,
    subscribe,
} from './chatStore.js';

const CONV = 'conv_t';

beforeEach(() => { __resetAll(); });

describe('chatStore — per-conversation isolation', () => {
    it('fresh conversation yields empty state and empty projection', () => {
        expect(getState(CONV).turns).toEqual([]);
        expect(getProjection(CONV)).toEqual([]);
    });

    it('submit creates a pending turn; getProjection shows [user, iteration]', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_1',
            content: 'hello',
            createdAt: '2025-01-01T00:00:00Z',
        });
        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration']);
        expect(getState(CONV).turns[0].lifecycle).toBe('pending');
    });

    it('state is isolated per conversation', () => {
        submit({ conversationId: 'conv_A', clientRequestId: 'a1', content: 'one', createdAt: '2025-01-01T00:00:00Z' });
        submit({ conversationId: 'conv_B', clientRequestId: 'b1', content: 'two', createdAt: '2025-01-01T00:00:00Z' });
        expect(getState('conv_A').turns[0].users[0].content).toBe('one');
        expect(getState('conv_B').turns[0].users[0].content).toBe('two');
    });
});

describe('chatStore — projection caching', () => {
    it('getProjection returns the same array reference between mutations', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'a', createdAt: '2025-01-01T00:00:00Z' });
        const p1 = getProjection(CONV);
        const p2 = getProjection(CONV);
        expect(p1).toBe(p2);
    });

    it('mutation invalidates the projection cache (new reference after submit)', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'a', createdAt: '2025-01-01T00:00:00Z' });
        const p1 = getProjection(CONV);
        submit({ conversationId: CONV, clientRequestId: '2', content: 'b', createdAt: '2025-01-01T00:00:01Z' });
        const p2 = getProjection(CONV);
        expect(p2).not.toBe(p1);
    });
});

describe('chatStore — subscribe / notify', () => {
    it('listeners fire on submit, onSSE, onTranscript', () => {
        let n = 0;
        const off = subscribe(() => { n += 1; });

        submit({ conversationId: CONV, clientRequestId: '1', content: 'a', createdAt: '2025-01-01T00:00:00Z' });
        expect(n).toBe(1);

        onSSE(CONV, { type: 'turn_started', conversationId: CONV, turnId: 'tn_1', createdAt: '2025-01-01T00:00:00.100Z' });
        expect(n).toBe(2);

        onTranscript(CONV, { conversationId: CONV, turns: [{ turnId: 'tn_1', status: 'running' }] });
        expect(n).toBe(3);

        off();
    });

    it('cross-conversation events are ignored', () => {
        onSSE(CONV, { type: 'turn_started', conversationId: 'other', turnId: 'tn_X' });
        expect(getState(CONV).turns).toEqual([]);
    });
});

describe('chatStore — reset', () => {
    it('reset drops state for the given conversation', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'a', createdAt: '2025-01-01T00:00:00Z' });
        expect(getState(CONV).turns.length).toBe(1);
        reset(CONV);
        expect(getState(CONV).turns).toEqual([]);
    });
});

describe('chatStore — isRunning / isQueued selectors', () => {
    it('isRunning is true for a pending local turn', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'a', createdAt: '2025-01-01T00:00:00Z' });
        expect(isRunning(CONV)).toBe(true);
    });

    it('isRunning is false after terminal turn_completed', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'a', createdAt: '2025-01-01T00:00:00Z' });
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_1',
            userMessageId: 'msg_1',
            clientRequestId: '1',
            createdAt: '2025-01-01T00:00:00.100Z',
        });
        onSSE(CONV, {
            type: 'turn_completed',
            conversationId: CONV,
            turnId: 'tn_1',
            createdAt: '2025-01-01T00:00:05.000Z',
        });
        expect(isRunning(CONV)).toBe(false);
    });

    it('isQueued is true when a second normal local submit stacks behind the first live turn', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'first', createdAt: '2025-01-01T00:00:00Z' });
        submit({ conversationId: CONV, clientRequestId: '2', content: 'second', createdAt: '2025-01-01T00:00:00.050Z' });
        expect(getState(CONV).turns.length).toBe(2);
        expect(isQueued(CONV)).toBe(true);
    });

    it('explicit steer does not create a queued follow-up turn', () => {
        submit({ conversationId: CONV, clientRequestId: '1', content: 'first', createdAt: '2025-01-01T00:00:00Z' });
        steer({ conversationId: CONV, clientRequestId: '2', content: 'follow-up', createdAt: '2025-01-01T00:00:00.050Z' });
        expect(getState(CONV).turns.length).toBe(1);
        expect(getState(CONV).turns[0].users.length).toBe(2);
        expect(isQueued(CONV)).toBe(false);
    });
});

describe('chatStore — identity stability across bootstrap → SSE echo → transcript', () => {
    it('user renderKey and iteration renderKey are stable across all three mutations', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_S',
            content: 'hello',
            createdAt: '2025-01-01T00:00:00Z',
        });
        const rowsBeforeSSE = getProjection(CONV);
        const userKey0 = rowsBeforeSSE.find((r) => r.kind === 'user').renderKey;
        const iterKey0 = rowsBeforeSSE.find((r) => r.kind === 'iteration').renderKey;

        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_S',
            userMessageId: 'msg_S',
            clientRequestId: 'crid_S',
            createdAt: '2025-01-01T00:00:00.100Z',
        });
        const rowsAfterSSE = getProjection(CONV);
        const userKey1 = rowsAfterSSE.find((r) => r.kind === 'user').renderKey;
        const iterKey1 = rowsAfterSSE.find((r) => r.kind === 'iteration').renderKey;
        expect(userKey1).toBe(userKey0);
        expect(iterKey1).toBe(iterKey0);

        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_S',
                status: 'running',
                user: { messageId: 'msg_S', content: 'hello', clientRequestId: 'crid_S' },
            }],
        });
        const rowsAfterTranscript = getProjection(CONV);
        const userKey2 = rowsAfterTranscript.find((r) => r.kind === 'user').renderKey;
        const iterKey2 = rowsAfterTranscript.find((r) => r.kind === 'iteration').renderKey;
        expect(userKey2).toBe(userKey0);
        expect(iterKey2).toBe(iterKey0);

        // Exactly one user row — no duplicate.
        expect(rowsAfterTranscript.filter((r) => r.kind === 'user').length).toBe(1);
    });
});
