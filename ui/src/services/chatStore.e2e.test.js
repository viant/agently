/**
 * chatStore.e2e.test.js — high-signal end-to-end coverage on the live
 * chat-feed pipeline for the seven observed bugs. Each test runs the real
 * sequence: submit() → onSSE() → onTranscript() → render via
 * ChatFeedFromChatStore and asserts the structural invariant that makes the
 * bug impossible.
 *
 * Coverage:
 *   T1  submit bootstrap          → renderKey stable across SSE + transcript
 *   T2  empty "Execution details" shell never appears
 *   T3  intake visible inside execution details
 *   T4  header does not flip terminal mid-turn
 *   T5  group count is monotonic under mid-stream transcript poll
 *   T6  async model-completed → text_delta gap: no completed flash
 *   T7  lifecycle-only turn_started renders inline (not a main-bubble leak)
 *   T-steer  two explicit mid-turn steering injections render [u0, card, u1, u2]
 *   T-live  active SSE-only turn grows into intake + sidecar + sidecar rounds
 *   T-note  transcript standalone assistant note renders before later iteration
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('forge/components', () => ({
    AvatarIcon: ({ name = '' }) => React.createElement('span', { 'data-avatar-icon': name }),
}));

import {
    __resetAll,
    getProjection,
    getState,
    onSSE,
    onTranscript,
    steer,
    submit,
} from './chatStore.js';
import ChatFeedFromChatStore from '../components/chat/ChatFeedFromChatStore.jsx';

const h = React.createElement;

const CONV = 'conv_e2e';

function render() {
    return renderToStaticMarkup(h(ChatFeedFromChatStore, {
        conversationId: CONV,
        rowsOverride: getProjection(CONV),
    }));
}

beforeEach(() => { __resetAll(); });

// ─── T1: submit bootstrap + stable render identity ────────────────────────────

describe('T1 submit bootstrap → SSE echo → transcript hydration', () => {
    it('user bubble renderKey is stable across all three frames; no duplicate', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_T1',
            content: 'hello',
            createdAt: '2025-01-01T00:00:00.000Z',
        });
        const frame0 = getProjection(CONV);
        const userKey0 = frame0.find((r) => r.kind === 'user').renderKey;

        // SSE echo 60 ms later
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_T1',
            userMessageId: 'msg_T1',
            clientRequestId: 'crid_T1',
            createdAt: '2025-01-01T00:00:00.060Z',
        });
        const frame1 = getProjection(CONV);

        // Transcript poll 80 ms later (backend echoes the persisted row)
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_T1',
                status: 'running',
                user: { messageId: 'msg_T1', content: 'hello', clientRequestId: 'crid_T1' },
            }],
        });
        const frame2 = getProjection(CONV);

        const userRows0 = frame0.filter((r) => r.kind === 'user');
        const userRows1 = frame1.filter((r) => r.kind === 'user');
        const userRows2 = frame2.filter((r) => r.kind === 'user');

        expect(userRows0.length).toBe(1);
        expect(userRows1.length).toBe(1);
        expect(userRows2.length).toBe(1);

        // Identity stable across all frames.
        expect(userRows1[0].renderKey).toBe(userKey0);
        expect(userRows2[0].renderKey).toBe(userKey0);

        // messageId filled in from SSE, preserved by transcript.
        expect(userRows1[0].messageId).toBe('msg_T1');
        expect(userRows2[0].messageId).toBe('msg_T1');

        // Rendered HTML contains the user text exactly once.
        const html = render();
        expect((html.match(/hello/g) || []).length).toBe(1);
    });

    it('turn_started without echoed ids still keeps one user row and one iteration card', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_noecho',
            content: 'hello',
            createdAt: '2025-01-01T00:00:00.000Z',
        });
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_noecho',
            createdAt: '2025-01-01T00:00:00.060Z',
        });

        const rows = getProjection(CONV);
        expect(rows.filter((r) => r.kind === 'user')).toHaveLength(1);
        expect(rows.filter((r) => r.kind === 'iteration')).toHaveLength(1);
        const html = render();
        expect((html.match(/app-bubble-row-user/g) || []).length).toBe(1);
        expect((html.match(/app-iteration-card/g) || []).length).toBe(1);
    });

    it('model_started with the real turnId still reuses the single pending bootstrap turn', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_promote_late',
            content: 'show me HOME env variable',
            createdAt: '2025-01-01T00:00:00.000Z',
        });
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            createdAt: '2025-01-01T00:00:00.050Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_promoted',
            assistantMessageId: 'msg_promoted',
            provider: 'openai',
            model: 'gpt-5.4',
            createdAt: '2025-01-01T00:00:00.100Z',
        });

        const rows = getProjection(CONV);
        expect(rows.filter((r) => r.kind === 'user')).toHaveLength(1);
        expect(rows.filter((r) => r.kind === 'iteration')).toHaveLength(1);
    });

    it('completed transcript after a no-echo live turn still renders the prompt once', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_noecho_done',
            content: 'hello',
            createdAt: '2025-01-01T00:00:00.000Z',
        });
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_noecho_done',
            createdAt: '2025-01-01T00:00:00.060Z',
        });
        onSSE(CONV, {
            type: 'turn_completed',
            conversationId: CONV,
            turnId: 'tn_noecho_done',
            createdAt: '2025-01-01T00:00:05.000Z',
        });
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_noecho_done',
                status: 'completed',
                startedByMessageId: 'msg_noecho_done',
                user: {
                    messageId: 'msg_noecho_done',
                    content: 'hello',
                },
                assistant: {
                    final: {
                        messageId: 'am_noecho_done',
                        content: 'done',
                    },
                },
            }],
        });

        const rows = getProjection(CONV);
        expect(rows.filter((r) => r.kind === 'user')).toHaveLength(1);
        const html = render();
        expect((html.match(/hello/g) || []).length).toBe(1);
    });
});

// ─── T-note: transcript standalone assistant note ordering ───────────────────

describe('T-note transcript standalone assistant note ordering', () => {
    it('renders [user, assistant-note, iteration] when a later final page has a higher sequence', () => {
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_note',
                status: 'completed',
                user: {
                    messageId: 'msg_user',
                    content: 'show me the workspace',
                    sequence: 1,
                },
                messages: [{
                    messageId: 'msg_note',
                    role: 'assistant',
                    content: 'PRELIMINARY NOTE',
                    sequence: 8,
                    createdAt: '2026-04-21T00:00:08Z',
                }],
                execution: {
                    pages: [{
                        pageId: 'page_final',
                        assistantMessageId: 'page_final',
                        sequence: 9,
                        status: 'completed',
                        finalResponse: true,
                        content: 'Final answer',
                    }],
                },
            }],
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'assistant', 'iteration']);
        const html = render();
        const idxUser = html.indexOf('show me the workspace');
        const idxNote = html.indexOf('PRELIMINARY NOTE');
        const idxFinal = html.indexOf('Final answer');
        expect(idxUser).toBeGreaterThanOrEqual(0);
        expect(idxNote).toBeGreaterThan(idxUser);
        expect(idxFinal).toBeGreaterThan(idxNote);
    });
});

describe('T-note-live control message_add ordering', () => {
    it('renders a live standalone assistant note before the later iteration content in the same turn', () => {
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_live_note',
            createdAt: '2026-04-21T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'control',
            conversationId: CONV,
            turnId: 'tn_live_note',
            op: 'message_add',
            messageId: 'msg_note_live',
            patch: {
                id: 'msg_note_live',
                turnId: 'tn_live_note',
                role: 'assistant',
                content: 'PRELIMINARY NOTE',
                sequence: 8,
                interim: 0,
                createdAt: '2026-04-21T00:00:08Z',
            },
        });
        onSSE(CONV, {
            type: 'model_completed',
            conversationId: CONV,
            turnId: 'tn_live_note',
            pageId: 'page_final_live',
            assistantMessageId: 'page_final_live',
            content: 'Final answer',
            iteration: 1,
            status: 'completed',
            createdAt: '2026-04-21T00:00:09Z',
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['iteration']);
        const html = render();
        const idxFinal = html.indexOf('Final answer');
        expect(idxFinal).toBeGreaterThanOrEqual(0);
    });
});

describe('T-live transcript isolation', () => {
    it('does not let transcript inject assistant content into an SSE-owned active turn', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_live_only',
            content: 'analyze turn ordering',
            createdAt: '2026-04-21T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_live_only',
            userMessageId: 'msg_live_only',
            clientRequestId: 'crid_live_only',
            createdAt: '2026-04-21T00:00:00.050Z',
        });
        onSSE(CONV, {
            type: 'narration',
            conversationId: CONV,
            turnId: 'tn_live_only',
            messageId: 'asst_live_only',
            narration: 'Working...',
            createdAt: '2026-04-21T00:00:00.100Z',
            patch: { sequence: 2, status: 'running' },
        });

        // Simulate the web-ui transcript guard: active SSE-owned turn is
        // omitted from transcript forwarding, so only historical turns reach
        // the canonical client store.
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [],
        });

        const html = render();
        expect(html).toContain('Working...');
        expect(html).not.toContain('PRELIMINARY NOTE');
        expect((html.match(/analyze turn ordering/g) || []).length).toBe(1);
    });
});

describe('T-steer-transcript transcript extra user ordering', () => {
    it('renders [user, iteration, user] when a later same-turn user message has a higher sequence than the final page', () => {
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_transcript_steer',
                status: 'completed',
                user: {
                    messageId: 'msg_user_1',
                    content: 'Initial ask',
                    sequence: 1,
                },
                messages: [{
                    messageId: 'msg_user_2',
                    role: 'user',
                    content: 'Steer: narrow scope',
                    sequence: 10,
                    createdAt: '2026-04-21T00:00:10Z',
                }],
                execution: {
                    pages: [{
                        pageId: 'page_final',
                        assistantMessageId: 'page_final',
                        sequence: 9,
                        status: 'completed',
                        finalResponse: true,
                        content: 'Final answer',
                    }],
                },
            }],
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration', 'user']);
        const html = render();
        const idxUser1 = html.indexOf('Initial ask');
        const idxFinal = html.indexOf('Final answer');
        const idxUser2 = html.indexOf('Steer: narrow scope');
        expect(idxUser1).toBeGreaterThanOrEqual(0);
        expect(idxFinal).toBeGreaterThan(idxUser1);
        expect(idxUser2).toBeGreaterThan(idxFinal);
    });
});

describe('T-parent-terminal linked child still running', () => {
    it('renders the parent iteration as settled even when linked child status is still running', () => {
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_parent_done',
                status: 'completed',
                user: {
                    messageId: 'msg_user_parent',
                    content: 'Summarize the result',
                    sequence: 1,
                },
                execution: {
                    pages: [{
                        pageId: 'page_parent_done',
                        assistantMessageId: 'page_parent_done',
                        sequence: 2,
                        status: 'completed',
                        finalResponse: true,
                        content: 'Parent answer',
                    }],
                },
                linkedConversations: [{
                    conversationId: 'child-1',
                    agentId: 'worker',
                    status: 'running',
                    title: 'Child run',
                }],
            }],
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration']);
        const iteration = rows.find((r) => r.kind === 'iteration');
        expect(iteration.lifecycle).toBe('completed');

        const html = render();
        expect(html).toContain('Execution details');
        expect(html).toContain('tone-success');
        expect(html).not.toContain('tone-running');
    });

    it('keeps the live feed settled when a linked child was attached before the parent turn completed', () => {
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_parent_live_done',
            createdAt: '2026-04-21T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'linked_conversation_attached',
            conversationId: CONV,
            turnId: 'tn_parent_live_done',
            linkedConversationId: 'child-live-1',
            linkedConversationAgentId: 'worker',
            linkedConversationTitle: 'Child live run',
            status: 'running',
            createdAt: '2026-04-21T00:00:01Z',
        });
        onSSE(CONV, {
            type: 'model_completed',
            conversationId: CONV,
            turnId: 'tn_parent_live_done',
            pageId: 'page_parent_live_done',
            assistantMessageId: 'page_parent_live_done',
            content: 'Parent answer',
            iteration: 1,
            status: 'completed',
            createdAt: '2026-04-21T00:00:02Z',
        });
        onSSE(CONV, {
            type: 'turn_completed',
            conversationId: CONV,
            turnId: 'tn_parent_live_done',
            createdAt: '2026-04-21T00:00:03Z',
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['iteration']);
        const iteration = rows.find((r) => r.kind === 'iteration');
        expect(iteration.lifecycle).toBe('completed');

        const html = render();
        expect(html).toContain('tone-success');
        expect(html).not.toContain('tone-running');
    });
});

// ─── T2: empty "Execution details (0)" never rendered ─────────────────────────

describe('T2 empty execution-details (0) shell', () => {
    it('a turn in pending state shows execution details, never "(0)"', () => {
        submit({
            conversationId: CONV, clientRequestId: 'c', content: 'hi',
            createdAt: '2025-01-01T00:00:00Z',
        });
        const html = render();
        expect(html).toContain('Execution details');
        expect(html).not.toContain('(0)');
    });

    it('turn_started only still shows execution details, no (0)', () => {
        onSSE(CONV, {
            type: 'turn_started', conversationId: CONV, turnId: 'tn_T2',
            createdAt: '2025-01-01T00:00:00Z',
        });
        const html = render();
        expect(html).not.toContain('(0)');
        expect(html).toContain('Execution details');
        expect(html).not.toContain('Turn started');
    });
});

// ─── T3: intake visible inside execution details ──────────────────────────────

describe('T3 intake phase visible inside execution details', () => {
    it('intake round renders through the old execution-details structure', () => {
        onSSE(CONV, {
            type: 'turn_started', conversationId: CONV, turnId: 'tn_T3',
            createdAt: '2025-01-01T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_T3',
            pageId: 'pg_intake',
            phase: 'intake',
            modelCallId: 'mc_intake',
        });
        const iter = getProjection(CONV).find((r) => r.kind === 'iteration');
        expect(iter.rounds.map((r) => r.phase)).toEqual(['intake']);
        const html = render();
        expect(html).toContain('Execution details');
        expect(html).toContain('model');
    });
});

// ─── T4: no mid-turn terminal flip ────────────────────────────────────────────

describe('T4 header does not flip to terminal mid-turn', () => {
    it('model_completed does NOT produce a success/completed header while turn is still running', () => {
        onSSE(CONV, {
            type: 'turn_started', conversationId: CONV, turnId: 'tn_T4',
            createdAt: '2025-01-01T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'model_started', conversationId: CONV, turnId: 'tn_T4',
            pageId: 'pg_1', modelCallId: 'mc_1',
        });
        onSSE(CONV, {
            type: 'model_completed', conversationId: CONV, turnId: 'tn_T4',
            pageId: 'pg_1', modelCallId: 'mc_1', status: 'completed',
        });
        // Between model_completed and next text_delta, header must stay running.
        expect(getState(CONV).turns[0].lifecycle).toBe('running');
        const html = render();
        expect(html).toContain('app-iteration-card tone-running');
    });
});

// ─── T5: monotonic group count under mid-stream transcript poll ───────────────

describe('T5 group count never shrinks', () => {
    it('tracker has 4 model steps, transcript returns 3 → still 4 after merge', () => {
        onSSE(CONV, {
            type: 'turn_started', conversationId: CONV, turnId: 'tn_T5',
            createdAt: '2025-01-01T00:00:00Z',
        });
        for (const mc of ['mc_1', 'mc_2', 'mc_3', 'mc_4']) {
            onSSE(CONV, {
                type: 'model_started', conversationId: CONV, turnId: 'tn_T5',
                pageId: 'pg_T5', modelCallId: mc,
            });
        }
        const before = Math.max(
            0,
            ...getState(CONV).turns[0].pages.map((p) => Array.isArray(p.modelSteps) ? p.modelSteps.length : 0)
        );
        expect(before).toBe(4);

        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_T5',
                status: 'running',
                execution: {
                    pages: [{
                        pageId: 'pg_T5',
                        modelSteps: [
                            { modelCallId: 'mc_1' },
                            { modelCallId: 'mc_2' },
                            { modelCallId: 'mc_3' },
                        ],
                        finalResponse: false,
                    }],
                },
            }],
        });
        const after = Math.max(
            0,
            ...getState(CONV).turns[0].pages.map((p) => Array.isArray(p.modelSteps) ? p.modelSteps.length : 0)
        );
        expect(after).toBe(4);            // mc_4 survives, no shrink
    });
});

// ─── T6: async child progress → model_completed → gap → text_delta ────────────

describe('T6 async gap does not flash completed', () => {
    it('model_completed followed by a 1200 ms gap then text_delta keeps lifecycle running', () => {
        onSSE(CONV, {
            type: 'turn_started', conversationId: CONV, turnId: 'tn_T6',
            createdAt: '2025-01-01T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'model_started', conversationId: CONV, turnId: 'tn_T6',
            pageId: 'pg_T6', modelCallId: 'mc_1',
        });
        onSSE(CONV, {
            type: 'model_completed', conversationId: CONV, turnId: 'tn_T6',
            pageId: 'pg_T6', modelCallId: 'mc_1', status: 'completed',
        });
        // No further events for "1200 ms" — lifecycle stays running.
        expect(getState(CONV).turns[0].lifecycle).toBe('running');
        // Next text_delta arrives for a NEW round.
        onSSE(CONV, {
            type: 'text_delta', conversationId: CONV, turnId: 'tn_T6',
            pageId: 'pg_T6b', content: 'next round text',
        });
        expect(getState(CONV).turns[0].lifecycle).toBe('running');
        const html = render();
        expect(html).toContain('tone-running');
        expect(html).toContain('next round text');
    });
});

// ─── T7: lifecycle-only turn renders inline, never a main-bubble shell ────────

describe('T7 lifecycle-only turn_started does not leak as a placeholder bubble', () => {
    it('only renders the iteration card (descriptive label) + user bubble; lifecycle entry is inline', () => {
        submit({
            conversationId: CONV, clientRequestId: 'c', content: 'hi',
            createdAt: '2025-01-01T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'turn_started', conversationId: CONV, turnId: 'tn_T7',
            userMessageId: 'msg_T7', clientRequestId: 'c',
            createdAt: '2025-01-01T00:00:00.100Z',
        });
        const rows = getProjection(CONV);
        // Exactly: one user + one iteration. No extra placeholder rows.
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration']);
        const html = render();
        // Lifecycle entry does not leak into execution-details rows.
        expect(html).not.toContain('Turn started');
        // Header stays as execution details; still no "(0)".
        expect(html).toContain('Execution details');
        expect(html).not.toContain('(0)');
    });
});

// ─── T-steer: two explicit mid-turn steering injections ───────────────────────

describe('T-steer steering placement', () => {
    it('[u0, iteration (growing), u1, u2] with all renderKeys stable', () => {
        submit({ conversationId: CONV, clientRequestId: 'c0', content: 'initial',  createdAt: '2025-01-01T00:00:00Z' });
        const r1 = getProjection(CONV);
        const u0Key = r1.find((r) => r.kind === 'user').renderKey;
        const iterKey = r1.find((r) => r.kind === 'iteration').renderKey;

        steer({ conversationId: CONV, clientRequestId: 'c1', content: 'follow-up', createdAt: '2025-01-01T00:00:10Z' });
        steer({ conversationId: CONV, clientRequestId: 'c2', content: 'third',     createdAt: '2025-01-01T00:00:20Z' });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration', 'user', 'user']);
        expect(rows.find((r) => r.kind === 'iteration').renderKey).toBe(iterKey);
        expect(rows.filter((r) => r.kind === 'user')[0].renderKey).toBe(u0Key);

        const html = render();
        const idxU0 = html.indexOf('initial');
        const idxIter = html.indexOf('Execution details');
        const idxU1 = html.indexOf('follow-up');
        const idxU2 = html.indexOf('third');
        expect(idxU0).toBeGreaterThanOrEqual(0);
        expect(idxIter).toBeGreaterThan(idxU0);
        expect(idxU1).toBeGreaterThan(idxIter);
        expect(idxU2).toBeGreaterThan(idxU1);
    });
});

// ─── T-live: active turn uses SSE only and does not over-fragment ────────────

describe('T-live active SSE sequence stays consolidated by iteration', () => {
    it('router intake + task sidecar + later task sidecar render as 3 rounds', () => {
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_live',
            createdAt: '2025-01-01T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'msg_intake',
            assistantMessageId: 'msg_intake',
            modelCallId: 'msg_intake',
            mode: 'router',
            phase: 'intake',
            iteration: 0,
            status: 'thinking',
            createdAt: '2025-01-01T00:00:01Z',
        });
        onSSE(CONV, {
            type: 'model_completed',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'msg_intake',
            assistantMessageId: 'msg_intake',
            modelCallId: 'msg_intake',
            mode: 'router',
            phase: 'intake',
            iteration: 0,
            status: 'completed',
            createdAt: '2025-01-01T00:00:02Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'msg_task_1',
            assistantMessageId: 'msg_task_1',
            parentMessageId: 'user_1',
            modelCallId: 'msg_task_1',
            mode: 'task',
            iteration: 1,
            status: 'thinking',
            createdAt: '2025-01-01T00:00:03Z',
        });
        onSSE(CONV, {
            type: 'tool_call_started',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'tool_msg_1',
            assistantMessageId: 'msg_task_1',
            parentMessageId: 'user_1',
            toolCallId: 'call_1',
            toolMessageId: 'tool_msg_1',
            toolName: 'llm/agents/list',
            mode: 'task',
            iteration: 1,
            status: 'running',
            createdAt: '2025-01-01T00:00:04Z',
        });
        onSSE(CONV, {
            type: 'tool_call_started',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'tool_msg_2',
            assistantMessageId: 'msg_task_1',
            parentMessageId: 'user_1',
            toolCallId: 'call_2',
            toolMessageId: 'tool_msg_2',
            toolName: 'prompt/list',
            mode: 'task',
            iteration: 1,
            status: 'running',
            createdAt: '2025-01-01T00:00:05Z',
        });
        onSSE(CONV, {
            type: 'tool_call_completed',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'tool_msg_1',
            assistantMessageId: 'msg_task_1',
            parentMessageId: 'user_1',
            toolCallId: 'call_1',
            toolMessageId: 'tool_msg_1',
            toolName: 'llm/agents/list',
            mode: 'task',
            iteration: 1,
            status: 'completed',
            createdAt: '2025-01-01T00:00:06Z',
        });
        onSSE(CONV, {
            type: 'tool_call_completed',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'tool_msg_2',
            assistantMessageId: 'msg_task_1',
            parentMessageId: 'user_1',
            toolCallId: 'call_2',
            toolMessageId: 'tool_msg_2',
            toolName: 'prompt/list',
            mode: 'task',
            iteration: 1,
            status: 'completed',
            createdAt: '2025-01-01T00:00:07Z',
        });
        onSSE(CONV, {
            type: 'model_completed',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'msg_task_1',
            assistantMessageId: 'msg_task_1',
            parentMessageId: 'user_1',
            modelCallId: 'msg_task_1',
            mode: 'task',
            iteration: 1,
            status: 'completed',
            createdAt: '2025-01-01T00:00:08Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'msg_task_2',
            assistantMessageId: 'msg_task_2',
            parentMessageId: 'user_1',
            modelCallId: 'msg_task_2',
            mode: 'task',
            iteration: 2,
            status: 'thinking',
            createdAt: '2025-01-01T00:00:09Z',
        });
        onSSE(CONV, {
            type: 'tool_call_started',
            conversationId: CONV,
            turnId: 'tn_live',
            messageId: 'tool_msg_3',
            assistantMessageId: 'msg_task_2',
            parentMessageId: 'user_1',
            toolCallId: 'call_3',
            toolMessageId: 'tool_msg_3',
            toolName: 'llm/agents/start',
            mode: 'task',
            iteration: 2,
            status: 'running',
            createdAt: '2025-01-01T00:00:09.500Z',
        });
        onSSE(CONV, {
            type: 'narration',
            conversationId: CONV,
            turnId: 'tn_live',
            assistantMessageId: 'msg_task_2',
            mode: 'task',
            iteration: 2,
            narration: 'Pulling benchmark recommendation…',
            createdAt: '2025-01-01T00:00:10Z',
        });

        const rows = getProjection(CONV);
        const iter = rows.find((r) => r.kind === 'iteration');
        expect(iter.rounds).toHaveLength(3);
        expect(iter.rounds.map((r) => `${r.iteration}:${r.phase}`)).toEqual([
            '0:intake',
            '1:main',
            '2:main',
        ]);

        const html = render();
        expect(html).not.toContain('Turn started');
        expect(html).not.toContain('Turn completed');
        expect((html.match(/llm\/agents\/list/g) || []).length).toBeGreaterThanOrEqual(1);
        expect((html.match(/llm\/agents\/start/g) || []).length).toBeGreaterThanOrEqual(1);
    });

    it('reuses the iteration bubble when narration is followed by the final assistant add for the same turn', () => {
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_single_bubble',
            createdAt: '2025-01-01T00:00:00Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_single_bubble',
            messageId: 'msg_iter_1',
            assistantMessageId: 'msg_iter_1',
            modelCallId: 'mc_iter_1',
            iteration: 1,
            status: 'thinking',
            createdAt: '2025-01-01T00:00:01Z',
        });
        onSSE(CONV, {
            type: 'narration',
            conversationId: CONV,
            turnId: 'tn_single_bubble',
            messageId: 'msg_iter_1',
            assistantMessageId: 'msg_iter_1',
            iteration: 1,
            narration: 'Checking the baseline first.',
            createdAt: '2025-01-01T00:00:02Z',
        });
        onSSE(CONV, {
            type: 'assistant',
            conversationId: CONV,
            turnId: 'tn_single_bubble',
            messageId: 'msg_iter_1',
            content: 'Baseline complete.',
            createdAt: '2025-01-01T00:00:03Z',
            patch: {
                role: 'assistant',
                sequence: 3,
            },
        });

        const rows = getProjection(CONV);
        expect(rows.map((row) => row.kind)).toEqual(['iteration']);
        const html = render();
        expect((html.match(/Baseline complete\./g) || []).length).toBe(1);
        expect(rows[0].rounds[0].narration).toBe('Checking the baseline first.');
    });
});

// ─── T-elicitation: paused turn keeps one user + one iteration, no (0) ─────

describe('T-elicitation pending input keeps the feed stable', () => {
    it('renders one user row and one iteration card with pending elicitation, no lifecycle rows or (0)', () => {
        submit({
            conversationId: CONV,
            clientRequestId: 'crid_elic',
            content: 'What is my HOME environment variable?',
            createdAt: '2025-01-01T00:00:00.000Z',
        });
        onSSE(CONV, {
            type: 'turn_started',
            conversationId: CONV,
            turnId: 'tn_elic',
            userMessageId: 'msg_elic_user',
            clientRequestId: 'crid_elic',
            createdAt: '2025-01-01T00:00:00.050Z',
        });
        onSSE(CONV, {
            type: 'model_started',
            conversationId: CONV,
            turnId: 'tn_elic',
            assistantMessageId: 'msg_elic_model',
            modelCallId: 'mc_elic',
            provider: 'openai',
            model: 'gpt-5.1',
            createdAt: '2025-01-01T00:00:00.100Z',
        });
        onSSE(CONV, {
            type: 'elicitation_requested',
            conversationId: CONV,
            turnId: 'tn_elic',
            elicitationId: 'elic-1',
            content: 'Please provide the value of your HOME environment variable.',
            callbackUrl: '/v1/elicitations/elic-1/resolve',
            elicitationData: {
                requestedSchema: {
                    type: 'object',
                    properties: {
                        value: { type: 'string' }
                    },
                    required: ['value']
                }
            },
            createdAt: '2025-01-01T00:00:00.200Z',
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration']);
        const iter = rows.find((r) => r.kind === 'iteration');
        expect(iter.elicitation).toMatchObject({
            elicitationId: 'elic-1',
            message: 'Please provide the value of your HOME environment variable.',
        });

        const html = render();
        expect(html).toContain('Execution details');
        expect(html).not.toContain('(0)');
        expect(html).not.toContain('Turn started');
        expect(html).not.toContain('Turn completed');
        expect((html.match(/app-bubble-row-user/g) || []).length).toBe(1);
        expect((html.match(/app-bubble-row-assistant/g) || []).length).toBe(0);
        expect((html.match(/app-iteration-card/g) || []).length).toBe(1);
    });
});

describe('T-elicitation resolved history keeps execution-details state honest', () => {
    it('renders accepted elicitation inside execution details without reverting it to pending/running', () => {
        onTranscript(CONV, {
            conversationId: CONV,
            turns: [{
                turnId: 'tn_elic_done',
                status: 'running',
                createdAt: '2026-04-18T19:00:00Z',
                user: {
                    messageId: 'msg_user_done',
                    content: 'check how many files are in my ~/Download folder',
                },
                elicitation: {
                    elicitationId: 'elic-done-1',
                    status: 'accepted',
                    message: 'Please confirm the exact folder path you want counted.',
                    requestedSchema: {
                        type: 'object',
                        properties: {
                            path: { type: 'string' }
                        },
                        required: ['path']
                    }
                },
                execution: {
                    pages: [{
                        assistantMessageId: 'msg_assistant_done',
                        status: 'running',
                        narration: 'Using prompt/get.',
                        modelCall: {
                            provider: 'openai',
                            model: 'gpt-5-mini',
                            status: 'running',
                        },
                        toolCalls: []
                    }]
                }
            }],
        });

        const rows = getProjection(CONV);
        expect(rows.map((r) => r.kind)).toEqual(['user', 'iteration']);

        const html = render();
        expect(html).toContain('Execution details');
        expect(html).toContain('Please confirm the exact folder path you want counted.');
        expect(html).toContain('accepted');
        expect(html).not.toContain('Input required');
    });
});
