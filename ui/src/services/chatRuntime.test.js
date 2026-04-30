import { beforeEach, describe, expect, it, vi } from 'vitest';
import { activeWindows } from 'forge/core';

import { bootstrapConversationSelection, createNewConversation, dsTick, ensureContextResources, ensureConversation, fetchTranscript, filterCanonicalConversationForLiveOwnedTurns, handleStreamEvent, installChatStoreMirror, latestAssistantRowForTurn, mapTranscriptToRows, normalizeMetaResponse, queueTranscriptRefresh, renderMergedRowsForContext, resolveLastTranscriptCursor, resolveStarterTasks, resolveStreamEventConversationID, shouldProcessStreamEvent, shouldUseLiveStream, startPolling, stopPolling, switchConversation, syncMessagesSnapshot } from './chatRuntime';
import { client } from './agentlyClient';

vi.mock('./agentlyClient', () => ({
  client: {
    getWorkspaceMetadata: vi.fn(),
    createConversation: vi.fn(),
    getConversation: vi.fn(),
    getTranscript: vi.fn(),
    listGeneratedFiles: vi.fn().mockResolvedValue([]),
    listPendingElicitations: vi.fn().mockResolvedValue([])
  }
}));

function createStorage() {
  const store = new Map();
  return {
    getItem(key) {
      return store.has(key) ? store.get(key) : null;
    },
    setItem(key, value) {
      store.set(String(key), String(value));
    },
    removeItem(key) {
      store.delete(String(key));
    }
  };
}

describe('normalizeMetaResponse', () => {
  it('uses backend capabilities to decide auto-select options', () => {
    const got = normalizeMetaResponse({
      defaults: { agent: 'coder', model: 'openai_gpt-5.2', autoSelectTools: true },
      capabilities: {
        agentAutoSelection: true,
        modelAutoSelection: false,
        compactConversation: true,
        pruneConversation: true
      },
      agentInfos: [{
        id: 'coder',
        name: 'Coder',
        starterTasks: [{ id: 'analyze', title: 'Analyze', prompt: 'Analyze this repo.' }]
      }],
      modelInfos: [
        { id: 'openai_gpt-5.2', name: 'GPT-5.2' },
        { id: 'openai_o3', name: 'o3 (OpenAI)' }
      ]
    });

    expect(got.defaults.autoSelectTools).toBe(true);
    expect(got.capabilities.agentAutoSelection).toBe(true);
    expect(got.capabilities.modelAutoSelection).toBe(false);
    expect(got.agentOptions[0]).toMatchObject({ value: 'auto', label: 'Auto-select agent' });
    expect(got.agentOptions[1]).toMatchObject({ value: 'coder', label: 'Coder' });
    expect(got.modelOptions[0]).toMatchObject({ value: 'openai_gpt-5.2', label: 'GPT-5.2' });
    expect(got.modelOptions[1]).toMatchObject({ value: 'openai_o3', label: 'o3 (OpenAI)' });
    expect(got.modelOptions.some((entry) => entry?.value === 'auto')).toBe(false);
    expect(got.agentInfos[0].starterTasks[0]).toMatchObject({ id: 'analyze', title: 'Analyze' });
  });
});

describe('resolveStarterTasks', () => {
  it('merges starter tasks across all agents for auto-select', () => {
    const got = resolveStarterTasks({
      selectedAgent: 'auto',
      agentInfos: [
        { id: 'coder', name: 'Coder', starterTasks: [{ id: 'analyze', title: 'Analyze', prompt: 'Analyze repo.' }] },
        { id: 'chatter', name: 'Chatter', starterTasks: [{ id: 'summarize', title: 'Summarize', prompt: 'Summarize this.' }] }
      ]
    });

    expect(got).toHaveLength(2);
    expect(got[0]).toMatchObject({ id: 'analyze', agentId: 'coder', agentName: 'Coder' });
    expect(got[1]).toMatchObject({ id: 'summarize', agentId: 'chatter', agentName: 'Chatter' });
  });
});

describe('handleStreamEvent', () => {
  it('updates the sidecar stream tracker with live event identity', () => {
    const chatState = ensureContextResources({ resources: {} });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    handleStreamEvent(chatState, context, 'conv-1', {
      type: 'model_started',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      status: 'thinking',
      model: { provider: 'openai', model: 'gpt-5.4' }
    });

    expect(chatState.streamTracker?.canonicalState).toMatchObject({
      conversationId: 'conv-1',
      activeTurnId: 'turn-1',
      bufferedMessages: [
        expect.objectContaining({
          id: 'msg-1',
          turnId: 'turn-1'
        })
      ]
    });
    expect(chatState.runningTurnId).toBe('turn-1');
  });

  it('syncs local turn state from tracker turn lifecycle events without message ids', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const chatState = ensureContextResources({ resources: {} });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'turn_started',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        status: 'running'
      });
      expect(chatState.streamTracker?.canonicalState?.activeTurnId).toBe('turn-1');
      expect(chatState.runningTurnId).toBe('turn-1');

      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'turn_completed',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        status: 'completed'
      });
      expect(chatState.streamTracker?.canonicalState?.activeTurnId).toBeNull();
      expect(chatState.runningTurnId).toBe('');
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('anchors turn_started live rows from activeStreamStartedAt when the control event omits createdAt', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeStreamStartedAt = Date.parse('2026-03-16T10:00:00.000Z');
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'control',
        op: 'turn_started',
        conversationId: 'conv-1',
        patch: { turnId: 'turn-1', status: 'running' }
      });

      const assistantRow = chatState.liveRows.find((row) => row?.role === 'assistant');
      expect(assistantRow).toMatchObject({
        id: 'turn:turn-1',
        turnId: 'turn-1',
        status: 'running',
      });
      expect(assistantRow?.executionGroups?.[0]?.toolSteps?.[0]).toMatchObject({
        kind: 'turn',
        reason: 'turn_started',
        status: 'running',
      });
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('anchors model_started live rows from activeStreamStartedAt when the event omits createdAt', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeStreamStartedAt = Date.parse('2026-03-16T10:00:00.000Z');
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'model_started',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        assistantMessageId: 'msg-1',
        status: 'thinking',
        model: { provider: 'openai', model: 'gpt-5.4' }
      });

      const assistantRow = chatState.liveRows.find((row) => row?.role === 'assistant');
      expect(assistantRow?.createdAt).toBe('');
      expect(assistantRow?.sequence).toBeNull();
      expect(assistantRow?.id).toBe('msg-1');
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('updates conversation form live state from stream lifecycle events', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const conversationState = { values: { id: 'conv-1', running: false, stage: '', status: '' } };
    const context = {
      resources: { chat: ensureContextResources({ resources: {} }) },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(context.resources.chat, context, 'conv-1', {
        type: 'model_started',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        assistantMessageId: 'msg-1',
        status: 'thinking',
        model: { provider: 'openai', model: 'gpt-5.4' }
      });
      expect(conversationState.values).toMatchObject({
        running: true,
        stage: 'thinking',
        status: 'thinking'
      });

      handleStreamEvent(context.resources.chat, context, 'conv-1', {
        type: 'tool_call_started',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        assistantMessageId: 'msg-1',
        toolCallId: 'call-1',
        toolMessageId: 'tool-msg-1',
        toolName: 'orchestration/updatePlan',
        status: 'running'
      });
      expect(conversationState.values).toMatchObject({
        running: true,
        stage: 'executing',
        status: 'running'
      });

      handleStreamEvent(context.resources.chat, context, 'conv-1', {
        type: 'linked_conversation_attached',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        assistantMessageId: 'msg-1',
        toolCallId: 'call-1',
        linkedConversationId: 'child-conv-1'
      });
      expect(conversationState.values).toMatchObject({
        running: true,
        stage: 'executing',
        status: 'running'
      });

      handleStreamEvent(context.resources.chat, context, 'conv-1', {
        type: 'elicitation_requested',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        assistantMessageId: 'msg-1',
        elicitationId: 'elic-1',
        status: 'pending',
        content: 'Need input',
        elicitationData: { requestedSchema: { type: 'object' } }
      });
      expect(conversationState.values).toMatchObject({
        running: true,
        stage: 'eliciting',
        status: 'pending'
      });

      handleStreamEvent(context.resources.chat, context, 'conv-1', {
        type: 'turn_completed',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        status: 'succeeded'
      });
      expect(conversationState.values).toMatchObject({
        running: false,
        stage: 'done',
        status: 'succeeded'
      });
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('requires conversation ids on stream events instead of deriving them from the subscription', () => {
    expect(resolveStreamEventConversationID({ type: 'text_delta' }, 'conv-1')).toBe('');
    expect(shouldProcessStreamEvent({
      payload: { type: 'text_delta' },
      subscribedConversationID: 'conv-1',
      visibleConversationID: 'conv-1'
    })).toBe(false);
  });

  it('ignores events for conversations outside the active subscriber window', () => {
    expect(shouldProcessStreamEvent({
      payload: { type: 'text_delta', conversationId: 'conv-2' },
      subscribedConversationID: 'conv-1',
      visibleConversationID: 'conv-1'
    })).toBe(false);
  });

  it('ignores old-stream events while switching to another conversation', () => {
    expect(shouldProcessStreamEvent({
      payload: { type: 'text_delta', conversationId: 'conv-old' },
      subscribedConversationID: 'conv-old',
      visibleConversationID: 'conv-old',
      switchingConversationID: 'conv-new'
    })).toBe(false);
    expect(shouldProcessStreamEvent({
      payload: { type: 'text_delta', conversationId: 'conv-new' },
      subscribedConversationID: 'conv-new',
      visibleConversationID: 'conv-old',
      switchingConversationID: 'conv-new'
    })).toBe(true);
  });

  it('ignores summary-mode execution events so maintenance work does not reanimate the live card', () => {
    const chatState = {
      liveRows: [],
      lastHasRunning: false,
      activeConversationID: 'conv-1',
      runningTurnId: 'turn-1',
      activeStreamTurnId: 'turn-1'
    };
    const context = {
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    handleStreamEvent(chatState, context, 'conv-1', {
      type: 'model_started',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      assistantMessageId: 'summary-msg-1',
      status: 'thinking',
      mode: 'summary'
    });

    expect(chatState.liveRows).toEqual([]);
  });

  it('ignores intake-phase streaming content while still allowing non-streaming lifecycle events elsewhere', () => {
    const chatState = {
      liveRows: [],
      lastHasRunning: false,
      activeConversationID: 'conv-1',
      runningTurnId: 'turn-1',
      activeStreamTurnId: 'turn-1'
    };
    const context = {
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    handleStreamEvent(chatState, context, 'conv-1', {
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      assistantMessageId: 'intake-msg-1',
      content: 'Classifying request…',
      status: 'thinking',
      phase: 'intake'
    });

    expect(chatState.liveRows).toEqual([]);
  });

  it('does not reference a raw MessageEvent when given a parsed SSE payload', () => {
    const chatState = { liveRows: [], lastHasRunning: false };
    expect(() => handleStreamEvent(chatState, {}, 'conv-1', {
      type: 'unknown_event',
      conversationId: 'conv-1',
      content: 'hello'
    })).not.toThrow();
  });

  it('coalesces text_delta bursts into a single render frame', async () => {
    vi.useFakeTimers();
    const setCollection = vi.fn();
    const originalWindow = globalThis.window;
    globalThis.window = {
      requestAnimationFrame: (cb) => setTimeout(cb, 16),
      cancelAnimationFrame: (id) => clearTimeout(id),
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      localStorage: createStorage(),
      sessionStorage: createStorage(),
      location: { pathname: '/conversation/conv-1' }
    };
    try {
      const chatState = {
        liveRows: [],
        transcriptRows: [],
        renderRows: [],
        lastHasRunning: false,
        activeConversationID: 'conv-1',
        liveOwnedConversationID: 'conv-1',
        liveOwnedTurnIds: ['turn-1']
      };
      const context = {
        identity: { windowId: 'chat/main' },
        resources: { chat: chatState },
        Context(name) {
          if (name === 'conversations') {
            return {
              handlers: {
                dataSource: {
                  peekFormData: () => ({ id: 'conv-1' }),
                  setFormData: vi.fn()
                }
              }
            };
          }
          if (name === 'messages') {
            return {
              handlers: {
                dataSource: {
                  setCollection,
                  setError: vi.fn()
                }
              }
            };
          }
          if (name === 'meta') {
            return {
              handlers: {
                dataSource: {
                  peekFormData: () => ({ defaults: {}, agentInfos: [] })
                }
              }
            };
          }
          return null;
        }
      };

      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'text_delta',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        id: 'msg-1',
        content: 'Hello'
      });
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'text_delta',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        id: 'msg-1',
        content: ' world'
      });

      expect(setCollection).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(20);

      expect(setCollection).toHaveBeenCalledTimes(1);
      expect(chatState.liveRows[0]?.content).toBe('Hello world');
    } finally {
      globalThis.window = originalWindow;
      vi.useRealTimers();
    }
  });

  it('ignores late execution events after the turn is already terminal', () => {
    const setCollection = vi.fn();
    const setFormData = vi.fn();
    const chatState = {
      liveRows: [{
        id: 'assistant:turn-1:live',
        role: 'assistant',
        turnId: 'turn-1',
        status: 'completed',
        turnStatus: 'completed',
        content: 'Final answer',
        executionGroups: [{
          assistantMessageId: 'msg-1',
          status: 'completed',
          modelSteps: [{ modelCallId: 'msg-1', status: 'completed' }],
          toolSteps: [],
          toolCallsPlanned: []
        }],
        createdAt: '2026-03-31T10:00:00Z'
      }],
      lastHasRunning: false,
      terminalTurns: { 'turn-1': '2026-03-31T10:00:10Z' }
    };
    const context = {
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    handleStreamEvent(chatState, context, 'conv-1', {
      type: 'model_started',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      assistantMessageId: 'msg-1',
      status: 'streaming',
      model: { provider: 'openai', model: 'gpt-5.2' }
    });

    expect(chatState.liveRows[0].status).toBe('completed');
    expect(chatState.liveRows[0].turnStatus).toBe('completed');
    expect(chatState.lastHasRunning).toBe(false);
    expect(setCollection).not.toHaveBeenCalled();
    expect(setFormData).not.toHaveBeenCalled();
  });

  it('publishes sidebar activity for hidden conversation terminal events', () => {
    const chatState = { liveRows: [], lastHasRunning: false };
    const received = [];
    const eventTarget = new EventTarget();
    const mockWindow = {
      addEventListener: eventTarget.addEventListener.bind(eventTarget),
      removeEventListener: eventTarget.removeEventListener.bind(eventTarget),
      dispatchEvent: eventTarget.dispatchEvent.bind(eventTarget),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    const originalWindow = globalThis.window;
    const originalCustomEvent = globalThis.CustomEvent;
    globalThis.window = mockWindow;
    globalThis.CustomEvent = mockWindow.CustomEvent;
    const handler = (event) => received.push(event?.detail || {});
    mockWindow.addEventListener('agently:conversation-activity', handler);
    try {
      const context = {
        identity: { windowId: 'linked-child' },
        Context(name) {
          if (name === 'conversations') {
            return {
              handlers: {
                dataSource: {
                  peekFormData: () => ({ id: 'child-conv' }),
                  setFormData: vi.fn()
                }
              }
            };
          }
          if (name === 'messages') {
            return {
              handlers: {
                dataSource: {
                  setCollection: vi.fn(),
                  setError: vi.fn()
                }
              }
            };
          }
          return null;
        }
      };

      handleStreamEvent(chatState, context, 'child-conv', {
        type: 'turn_completed',
        conversationId: 'parent-conv',
        turnId: 'turn-1',
        status: 'succeeded'
      });
    } finally {
      mockWindow.removeEventListener('agently:conversation-activity', handler);
      globalThis.window = originalWindow;
      globalThis.CustomEvent = originalCustomEvent;
    }

    expect(received).toHaveLength(1);
    expect(received[0]).toMatchObject({
      id: 'parent-conv',
      type: 'turn_completed',
      turnId: 'turn-1',
      status: 'succeeded'
    });
  });

  it('publishes conversation meta updates from SSE control path', () => {
    const chatState = { liveRows: [], lastHasRunning: false, streamTracker: { applyEvent: vi.fn() } };
    const received = [];
    const eventTarget = new EventTarget();
    const mockWindow = {
      addEventListener: eventTarget.addEventListener.bind(eventTarget),
      removeEventListener: eventTarget.removeEventListener.bind(eventTarget),
      dispatchEvent: eventTarget.dispatchEvent.bind(eventTarget),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    const originalWindow = globalThis.window;
    const originalCustomEvent = globalThis.CustomEvent;
    globalThis.window = mockWindow;
    globalThis.CustomEvent = mockWindow.CustomEvent;
    const handler = (event) => received.push(event?.detail || {});
    mockWindow.addEventListener('agently:conversation-meta-updated', handler);
    try {
      const context = {
        identity: { windowId: 'chat/main' },
        Context() { return null; }
      };
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'conversation_meta_updated',
        conversationId: 'conv-1',
        patch: { title: 'Campaign 4821 Underpacing', summary: 'underpacing' }
      });
    } finally {
      mockWindow.removeEventListener('agently:conversation-meta-updated', handler);
      globalThis.window = originalWindow;
      globalThis.CustomEvent = originalCustomEvent;
    }

    expect(received).toHaveLength(1);
    expect(received[0]).toEqual({
      id: 'conv-1',
      patch: { title: 'Campaign 4821 Underpacing', summary: 'underpacing' }
    });
  });

  it('renders control message_add as a standalone assistant row', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const chatState = ensureContextResources({ resources: {} });
    const context = {
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'control',
        op: 'turn_started',
        conversationId: 'conv-1',
        patch: { turnId: 'turn-1', status: 'running' }
      });
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'control',
        op: 'message_add',
        id: 'assistant-note-1',
        conversationId: 'conv-1',
        patch: {
          role: 'assistant',
          turnId: 'turn-1',
          content: 'Preliminary investigation: constrained PMP supply.',
          interim: 0,
          mode: 'task',
          createdAt: '2026-03-16T10:00:02.000Z'
        }
      });
    } finally {
      globalThis.window = originalWindow;
    }

    expect(chatState.liveRows.some((row) => String(row?.id || '') === 'turn:turn-1')).toBe(true);
    const addedRow = chatState.liveRows.find((row) => String(row?.id || '') === 'assistant-note-1');
    expect(addedRow).toMatchObject({
      role: 'assistant',
      turnId: 'turn-1',
      content: 'Preliminary investigation: constrained PMP supply.',
      interim: 0
    });
  });

  it('uses tracker-backed assistant rows as the primary active-turn source when live placeholders exist', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveOwnedConversationID = 'conv-1';
    chatState.liveOwnedTurnIds = ['turn-1'];
    chatState.liveRows = [
      {
        id: 'assistant:turn-1:live',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:00Z',
        interim: 1,
        status: 'running',
        turnStatus: 'running',
        content: '',
        executionGroups: []
      }
    ];
    chatState.streamTracker.applyEvent({
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      content: 'Calling updatePlan.',
      status: 'running',
      createdAt: '2026-03-16T01:00:01Z',
    });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({})
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: 'msg-1',
      turnId: 'turn-1',
      content: 'Calling updatePlan.'
    });
  });

  it('prefers tracker-backed assistant rows over stale live assistant rows for the same active conversation', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveOwnedConversationID = 'conv-1';
    chatState.liveOwnedTurnIds = ['turn-1'];
    chatState.liveRows = [
      {
        id: 'assistant-stale',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:00Z',
        interim: 1,
        status: 'running',
        turnStatus: 'running',
        content: 'stale local row',
        executionGroups: []
      }
    ];
    chatState.streamTracker.applyEvent({
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      content: 'Calling updatePlan.',
      status: 'running',
      createdAt: '2026-03-16T01:00:01Z',
    });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', running: true }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: 'msg-1',
      content: 'Calling updatePlan.'
    });
    expect(rows[0].id).not.toBe('assistant-stale');
  });

  it('preserves transient stream fields from a matching live assistant row when tracker owns the canonical row', () => {
    const messageState = { collection: [] };
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveOwnedConversationID = 'conv-1';
    chatState.liveOwnedTurnIds = ['turn-1'];
    chatState.liveRows = [
      {
        id: 'msg-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:02Z',
        interim: 1,
        isStreaming: true,
        content: 'Calling updatePlan.',
        _streamContent: 'Calling updatePlan. Then streaming...',
        _streamFence: { hasLeadingFence: false, hasTrailingFence: false, language: '' }
      }
    ];
    chatState.streamTracker.applyEvent({
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      content: 'Calling updatePlan.',
      status: 'running',
      createdAt: '2026-03-16T01:00:01Z',
    });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: 'msg-1',
      isStreaming: true,
      _streamContent: 'Calling updatePlan. Then streaming...'
    });
    expect(messageState.collection).toHaveLength(1);
    expect(messageState.collection[0]).toMatchObject({
      _type: 'iteration',
      content: 'Calling updatePlan.'
    });
  });

  it('keeps explicit live assistant rows for turns not covered by tracker rows', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveOwnedConversationID = 'conv-1';
    chatState.liveOwnedTurnIds = ['turn-1', 'turn-2'];
    chatState.liveRows = [
      {
        id: 'assistant-turn-2',
        role: 'assistant',
        turnId: 'turn-2',
        createdAt: '2026-03-16T01:00:03Z',
        interim: 1,
        status: 'running',
        turnStatus: 'running',
        content: 'Second live turn',
        executionGroups: []
      }
    ];
    chatState.streamTracker.applyEvent({
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      content: 'First tracker turn',
      status: 'running',
      createdAt: '2026-03-16T01:00:01Z',
    });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', running: true }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows.map((row) => row.id)).toEqual(['msg-1', 'assistant-turn-2']);
    expect(rows[0]).toMatchObject({ turnId: 'turn-1', content: 'First tracker turn' });
    expect(rows[1]).toMatchObject({ turnId: 'turn-2', content: 'Second live turn' });
  });

  it('keeps explicit standalone assistant rows for the same turn alongside tracker iteration rows', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveOwnedConversationID = 'conv-1';
    chatState.liveOwnedTurnIds = ['turn-1'];
    chatState.liveRows = [
      {
        id: 'assistant-note-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:03Z',
        interim: 0,
        status: 'completed',
        turnStatus: 'running',
        content: 'PRELIMINARY NOTE',
        executionGroups: []
      }
    ];
    chatState.streamTracker.applyEvent({
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      content: 'First tracker turn',
      status: 'running',
      createdAt: '2026-03-16T01:00:01Z',
    });
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', running: true }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows.map((row) => row.id)).toEqual(['msg-1', 'assistant-note-1']);
    expect(rows[0]).toMatchObject({ turnId: 'turn-1', content: 'First tracker turn' });
    expect(rows[1]).toMatchObject({ turnId: 'turn-1', content: 'PRELIMINARY NOTE' });
  });

  it('renders transcript extra assistant and user messages as standalone rows alongside the iteration row', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.transcriptRows = mapTranscriptToRows([{
      turnId: 'turn-1',
      createdAt: '2026-04-21T00:00:00Z',
      status: 'completed',
      user: {
        messageId: 'user-1',
        content: 'Initial ask'
      },
      messages: [
        {
          messageId: 'user-2',
          role: 'user',
          content: 'Steer: narrow scope',
          createdAt: '2026-04-21T00:00:02Z',
          interim: 0
        },
        {
          messageId: 'assistant-note-1',
          role: 'assistant',
          content: 'PRELIMINARY NOTE',
          createdAt: '2026-04-21T00:00:03Z',
          interim: 0
        }
      ],
      execution: {
        pages: [{
          pageId: 'page-final',
          assistantMessageId: 'page-final',
          status: 'completed',
          finalResponse: true,
          content: 'Final answer',
          modelSteps: [{
            modelCallId: 'mc-final',
            assistantMessageId: 'page-final',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'completed'
          }],
          toolSteps: []
        }]
      }
    }]).rows;

    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', running: false }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows.map((row) => row.id)).toEqual(['user-1', 'user-2', 'page-final', 'assistant-note-1']);
    expect(rows.find((row) => row.id === 'assistant-note-1')).toMatchObject({
      role: 'assistant',
      content: 'PRELIMINARY NOTE'
    });
    expect(rows.find((row) => row.id === 'user-2')).toMatchObject({
      role: 'user',
      content: 'Steer: narrow scope'
    });
  });

  it('keeps a standalone transcript assistant note even when the final assistant response repeats that note', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.transcriptRows = mapTranscriptToRows([{
      turnId: 'turn-1',
      createdAt: '2026-04-21T00:00:00Z',
      status: 'completed',
      user: {
        messageId: 'user-1',
        content: 'Initial ask'
      },
      messages: [
        {
          messageId: 'assistant-note-1',
          role: 'assistant',
          content: 'PRELIMINARY NOTE',
          createdAt: '2026-04-21T00:00:03Z',
          interim: 0
        }
      ],
      execution: {
        pages: [{
          pageId: 'page-final',
          assistantMessageId: 'page-final',
          status: 'completed',
          finalResponse: true,
          content: 'PRELIMINARY NOTE\n\nFinal answer',
          modelSteps: [{
            modelCallId: 'mc-final',
            assistantMessageId: 'page-final',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'completed'
          }],
          toolSteps: []
        }]
      }
    }]).rows;

    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', running: false }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    const noteRow = rows.find((row) => row.id === 'assistant-note-1');
    const finalRow = rows.find((row) => row.id === 'page-final');
    expect(noteRow).toMatchObject({
      role: 'assistant',
      content: 'PRELIMINARY NOTE'
    });
    expect(finalRow).toMatchObject({
      role: 'assistant'
    });
  });

  it('does not apply same-turn transient overlay when multiple explicit assistant rows exist without an exact id match', () => {
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveRows = [
      {
        id: 'assistant-older',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        isStreaming: true,
        _streamContent: 'older transient stream'
      },
      {
        id: 'assistant-newer',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:03Z',
        isStreaming: true,
        _streamContent: 'newer transient stream'
      }
    ];
    chatState.streamTracker.applyEvent({
      type: 'narration',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-tracker',
      assistantMessageId: 'msg-tracker',
      content: 'Tracker canonical row',
      status: 'running',
      createdAt: '2026-03-16T01:00:02Z',
    });

    const row = latestAssistantRowForTurn(chatState, 'conv-1', 'turn-1');
    expect(row).toMatchObject({
      id: 'msg-tracker',
      content: 'Tracker canonical row'
    });
    expect(row?._streamContent).toBeUndefined();
    expect(row?.isStreaming).toBeUndefined();
  });

  it('renders linked conversation metadata from tracker-backed execution groups without requiring a live row patch', () => {
    const setCollection = vi.fn();
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-parent';
    chatState.liveOwnedConversationID = 'conv-parent';
    chatState.liveOwnedTurnIds = ['turn-1'];
    const context = {
      resources: { chat: chatState },
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-parent', running: true }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    handleStreamEvent(chatState, context, 'conv-parent', {
      type: 'model_started',
      conversationId: 'conv-parent',
      turnId: 'turn-1',
      assistantMessageId: 'mc-1',
      messageId: 'mc-1',
      status: 'thinking',
      model: { provider: 'openai', model: 'gpt-5.4' }
    });
    handleStreamEvent(chatState, context, 'conv-parent', {
      type: 'tool_call_started',
      conversationId: 'conv-parent',
      turnId: 'turn-1',
      assistantMessageId: 'mc-1',
      toolCallId: 'call-agent-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'llm/agents/run',
      status: 'running'
    });
    const assistantBeforeLink = chatState.renderRows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    expect(String(assistantBeforeLink?.executionGroups?.[0]?.toolSteps?.[0]?.linkedConversationId || '')).toBe('');

    handleStreamEvent(chatState, context, 'conv-parent', {
      type: 'linked_conversation_attached',
      conversationId: 'conv-parent',
      turnId: 'turn-1',
      assistantMessageId: 'mc-1',
      toolCallId: 'call-agent-1',
      linkedConversationId: 'child-conv-1',
      linkedConversationAgentId: 'steward-forecasting',
      linkedConversationTitle: 'Forecasting Child'
    });

    const assistant = chatState.renderRows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    const linkedStep = assistant?.executionGroups
      ?.flatMap((group) => group?.toolSteps || [])
      ?.find((step) => String(step?.toolCallId || '').trim() === 'call-agent-1');
    expect(linkedStep).toMatchObject({
      toolCallId: 'call-agent-1',
      linkedConversationId: 'child-conv-1',
      linkedConversationAgentId: 'steward-forecasting',
      linkedConversationTitle: 'Forecasting Child'
    });
  });

  it('creates a failed assistant row on terminal stream failure even without prior execution content', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const setCollection = vi.fn();
    const setFormData = vi.fn();
    const chatState = {
      liveRows: [{
        id: 'user:turn-1',
        role: 'user',
        turnId: 'turn-1',
        content: 'Forecast inventory and uniques for deal 106171723',
        createdAt: '2026-04-01T12:00:00Z'
      }],
      lastHasRunning: true,
      activeConversationID: 'conv-1',
      activeStreamTurnId: 'turn-1',
      runningTurnId: 'turn-1'
    };
    const context = {
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection,
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'turn_failed',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        status: 'failed',
        error: 'failed to send request: dial tcp: lookup api.openai.com: no such host'
      });

      const assistant = chatState.liveRows.find((row) => row?.role === 'assistant');
      expect(assistant).toBeTruthy();
      expect(assistant).toMatchObject({
        turnId: 'turn-1',
        status: 'failed',
        turnStatus: 'failed',
        errorMessage: 'failed to send request: dial tcp: lookup api.openai.com: no such host'
      });
      expect(assistant.executionGroups[0]).toMatchObject({
        status: 'failed',
        errorMessage: 'failed to send request: dial tcp: lookup api.openai.com: no such host'
      });
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('stores deterministic terminal turn markers from the terminal payload', () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      location: { pathname: '/conversation/conv-1' }
    };
    const chatState = {
      liveRows: [],
      lastHasRunning: true,
      activeConversationID: 'conv-1',
      activeStreamTurnId: 'turn-1',
      runningTurnId: 'turn-1'
    };
    const context = {
      identity: { windowId: 'chat/main' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(chatState, context, 'conv-1', {
        type: 'turn_completed',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        status: 'succeeded',
        createdAt: '2026-03-16T10:00:10Z'
      });

      expect(chatState.terminalTurns).toMatchObject({
        'turn-1': '2026-03-16T10:00:10Z'
      });
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('preserves the active conversation render after terminal events without requiring a transcript refetch', async () => {
    vi.useFakeTimers();
    client.getConversation.mockReset();
    client.getTranscript.mockReset();

    const messageState = { collection: [] };
    const conversationState = { values: { id: 'conv-1', title: 'Good morning', queuedTurns: [], running: true } };
    const eventTarget = new EventTarget();
    const originalWindow = globalThis.window;
    const originalCustomEvent = globalThis.CustomEvent;
    const mockWindow = {
      location: { pathname: '/v1/conversation/conv-1' },
      history: { state: null, replaceState: vi.fn() },
      localStorage: createStorage(),
      sessionStorage: createStorage(),
      setTimeout: globalThis.setTimeout.bind(globalThis),
      clearTimeout: globalThis.clearTimeout.bind(globalThis),
      addEventListener: eventTarget.addEventListener.bind(eventTarget),
      removeEventListener: eventTarget.removeEventListener.bind(eventTarget),
      dispatchEvent: eventTarget.dispatchEvent.bind(eventTarget),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    globalThis.window = mockWindow;
    globalThis.CustomEvent = mockWindow.CustomEvent;
    const context = {
      identity: { windowId: 'chat/main' },
      resources: {
        chat: {
          activeConversationID: 'conv-1',
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1'],
          runningTurnId: 'turn-1',
          activeStreamTurnId: 'turn-1',
          lastHasRunning: true,
          liveRows: [{
            id: 'live-a1',
            role: 'assistant',
            turnId: 'turn-1',
            content: 'Temporary live content',
            createdAt: '2026-03-31T10:00:01Z'
          }],
          transcriptRows: [],
          renderRows: []
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    try {
      handleStreamEvent(context.resources.chat, context, 'conv-1', {
        type: 'turn_completed',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        status: 'succeeded'
      });

      await vi.runAllTimersAsync();
      await Promise.resolve();
      await Promise.resolve();

      expect(messageState.collection).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            _type: 'iteration',
            content: 'Temporary live content',
            role: 'assistant'
          })
        ])
      );
      expect(conversationState.values.running).toBe(false);
      expect(client.getConversation).not.toHaveBeenCalled();
      expect(client.getTranscript).not.toHaveBeenCalled();
    } finally {
      globalThis.window = originalWindow;
      globalThis.CustomEvent = originalCustomEvent;
      vi.useRealTimers();
    }
  });

  it('ignores late post-terminal execution events for the completed turn id', () => {
    const chatState = {
      terminalTurns: {},
      liveRows: [],
      transcriptRows: [],
      streamTracker: { canonicalState: { activeTurnId: null }, applyEvent: vi.fn() },
      runningTurnId: 'turn-1',
      activeStreamTurnId: 'turn-1',
      lastHasRunning: true,
    };
    const setCollection = vi.fn();
    const setFormData = vi.fn();
    const context = {
      resources: { chat: chatState },
      Context(name) {
        if (name === 'messages') {
          return { handlers: { dataSource: { setCollection, peekFormData: () => ({}) } } };
        }
        if (name === 'conversations') {
          return { handlers: { dataSource: { peekFormData: () => ({ id: 'conv-1', running: true }), setFormData } } };
        }
        if (name === 'meta') {
          return { handlers: { dataSource: { peekFormData: () => ({}) } } };
        }
        return null;
      }
    };

    handleStreamEvent(chatState, context, 'conv-1', {
      type: 'turn_completed',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'succeeded',
      createdAt: '2026-03-16T10:00:10Z'
    });

    expect(chatState.terminalTurns).toMatchObject({
      'turn-1': '2026-03-16T10:00:10Z'
    });

    handleStreamEvent(chatState, context, 'conv-1', {
      type: 'model_started',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      assistantMessageId: 'assistant-late',
      status: 'running',
      model: { provider: 'openai', model: 'gpt-5.4' }
    });

    expect(chatState.streamTracker.applyEvent).not.toHaveBeenCalled();
  });
});

describe('dsTick', () => {
  it('does not fetch transcript for the active live-owned conversation', async () => {
    client.getTranscript.mockReset();
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1'],
          runningTurnId: 'turn-1',
          activeStreamTurnId: 'turn-1',
          lastHasRunning: true,
          transcriptRows: [{ id: 'user:turn-1', role: 'user', turnId: 'turn-1', content: 'hi' }],
          liveRows: [{ id: 'assistant:turn-1:live', role: 'assistant', turnId: 'turn-1' }],
          lastQueuedTurns: []
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    const result = await dsTick(context, { conversationID: 'conv-1' });

    expect(client.getTranscript).not.toHaveBeenCalled();
    expect(result?.deferredToLiveStream).toBe(true);
    expect(result?.conversationID).toBe('conv-1');
  });

  it('does not fetch transcript during live bootstrap before the first turn id arrives', async () => {
    client.getTranscript.mockReset();
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: [],
          runningTurnId: '',
          activeStreamTurnId: '',
          activeStreamPrompt: 'recommend frequency cap for ctv',
          lastHasRunning: false,
          transcriptRows: [],
          liveRows: [],
          lastQueuedTurns: []
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', running: false }),
                setFormData: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    const result = await dsTick(context, { conversationID: 'conv-1' });

    expect(client.getTranscript).not.toHaveBeenCalled();
    expect(result?.deferredToLiveStream).toBe(true);
    expect(result?.conversationID).toBe('conv-1');
  });

  it('does not apply transcript snapshots while the live turn is SSE-owned', () => {
    const applyTranscript = vi.fn();
    const setCollection = vi.fn();
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1'],
          runningTurnId: 'turn-1',
          activeStreamTurnId: 'turn-1',
          lastHasRunning: true,
          liveRows: [{ id: 'assistant:turn-1:live', role: 'assistant', turnId: 'turn-1', content: 'live text' }],
          renderRows: [{ id: 'assistant:turn-1:live', role: 'assistant', turnId: 'turn-1', content: 'live text' }],
          transcriptRows: [{ id: 'user:turn-1', role: 'user', turnId: 'turn-1', content: 'Analyze repo' }],
          streamTracker: { applyTranscript, canonicalState: { activeTurnId: 'turn-1' } }
        }
      },
      Context(name) {
        if (name === 'messages') {
          return { handlers: { dataSource: { setCollection, peekFormData: () => ({}) } } };
        }
        if (name === 'conversations') {
          return { handlers: { dataSource: { peekFormData: () => ({ id: 'conv-1', running: true }), setFormData: vi.fn() } } };
        }
        if (name === 'meta') {
          return { handlers: { dataSource: { peekFormData: () => ({}) } } };
        }
        return null;
      }
    };

    const result = syncMessagesSnapshot(context, [{
      turnId: 'turn-1',
      status: 'running',
      assistant: { final: { messageId: 'a1', content: 'transcript final' } }
    }], 'poll', []);

    expect(applyTranscript).not.toHaveBeenCalled();
    expect(setCollection).toHaveBeenCalled();
    expect(Array.isArray(result)).toBe(true);
    expect(context.resources.chat.liveRows[0].content).toBe('live text');
  });

  it('promotes a transcript-discovered running conversation onto live stream transport', async () => {
    client.getTranscript.mockReset();
    client.listGeneratedFiles.mockReset();
    client.listGeneratedFiles.mockResolvedValue([]);
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        conversationId: 'conv-live-promote',
        turns: [
          {
            turnId: 'turn-live-promote',
            status: 'running',
            user: { messageId: 'user-1', content: 'hi' }
          }
        ]
      },
      feeds: []
    });
    const close = vi.fn();
    client.streamEvents = vi.fn(() => ({ close }));
    const context = {
      resources: {
        chat: {
          transcriptRows: [],
          liveRows: [],
          renderRows: [],
          lastQueuedTurns: [],
          lastHasRunning: false,
          runningTurnId: '',
          activeStreamTurnId: '',
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-live-promote', running: false }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({})
              }
            }
          };
        }
        return null;
      }
    };

    await dsTick(context, { conversationID: 'conv-live-promote' });

    expect(client.streamEvents).toHaveBeenCalledWith('conv-live-promote', expect.any(Object));
  });
});

describe('createNewConversation', () => {
  it('prefers persisted auto agent for a fresh draft conversation', async () => {
    const conversationState = { values: { id: 'old', agent: 'chatter', model: 'openai_gpt-5.4' } };
    const metaState = { values: { agent: 'chatter', defaults: { agent: 'chatter', model: 'openai_gpt-5.4', embedder: 'openai_text' } } };
    const context = {
      resources: {
        chat: {
          activeConversationID: 'conv-old',
          lastConversationID: 'conv-old'
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => metaState.values,
                setFormData: ({ values }) => { metaState.values = values; }
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    globalThis.localStorage = {
      getItem: (key) => (key === 'agently.selectedAgent' ? 'auto' : ''),
      setItem: vi.fn()
    };
    await createNewConversation(context);

    expect(conversationState.values.agent).toBe('auto');
    expect(metaState.values.agent).toBe('auto');
    expect(context.resources.chat.activeConversationID).toBe('');
    expect(context.resources.chat.lastConversationID).toBe('');
    expect(context.resources.chat.explicitNewConversationRequested).toBe(true);
  });

  it('clears persisted composer drafts for current and pending conversations on explicit new conversation', async () => {
    const store = {
      'forge.composerDrafts.v1': JSON.stringify({
        'conv-old': 'stale draft',
        '__pending__': 'starter prompt',
        'conv-keep': 'keep me'
      })
    };
    const sessionStorage = {
      getItem: (key) => store[key] || null,
      setItem: (key, value) => { store[key] = String(value); }
    };
    const originalWindow = global.window;
    global.window = { sessionStorage };

    const conversationState = { values: { id: 'conv-old', agent: 'steward', queuedTurns: [] } };
    const metaState = { values: { agent: 'steward', defaults: { agent: 'steward' } } };
    const context = {
      resources: {},
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => metaState.values,
                setFormData: ({ values }) => { metaState.values = values; }
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    try {
      await createNewConversation(context);
      const parsed = JSON.parse(store['forge.composerDrafts.v1']);
      expect(parsed['conv-old']).toBeUndefined();
      expect(parsed['__pending__']).toBeUndefined();
      expect(parsed['conv-keep']).toBe('keep me');
    } finally {
      global.window = originalWindow;
    }
  });
});

describe('switchConversation', () => {
  beforeEach(() => {
    client.getConversation.mockReset();
    client.getTranscript.mockReset();
    client.listGeneratedFiles.mockReset();
    client.listGeneratedFiles.mockResolvedValue([]);
  });

  it('resets stale transcript cursor when bootstrap already set the target conversation id', async () => {
    const messageState = { collection: [{ id: 'old-msg', role: 'assistant', content: 'stale' }] };
    const conversationState = { values: { id: 'conv-target', queuedTurns: [] } };
    const context = {
      resources: {
        chat: {
          lastSinceCursor: 'msg-from-other-conversation',
          lastConversationID: 'conv-old',
          transcriptRows: [{ id: 'old-user', role: 'user', turnId: 'turn-old', content: 'old' }],
          renderRows: [],
          liveRows: [],
          lastQueuedTurns: [],
          lastHasRunning: false,
          runningTurnId: '',
          activeConversationID: 'conv-old',
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] }),
                setFormData: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    client.getConversation.mockResolvedValueOnce({ id: 'conv-target', title: 'target', status: 'succeeded' });
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        conversationId: 'conv-target',
        turns: [
          {
            turnId: 'turn-target',
            status: 'completed',
            user: { messageId: 'user-target', content: 'new conversation content' }
          }
        ]
      },
      feeds: []
    });
    client.streamEvents = vi.fn(() => ({ close: vi.fn() }));
    client.listGeneratedFiles.mockResolvedValueOnce([]);

    await switchConversation(context, 'conv-target');

    expect(client.getTranscript).toHaveBeenCalledWith(
      expect.objectContaining({
        conversationId: 'conv-target',
        since: undefined,
        includeFeeds: true
      }),
      undefined
    );
    expect(context.resources.chat.lastConversationID).toBe('conv-target');
    expect(context.resources.chat.lastSinceCursor).toBe('user-target');
    expect(messageState.collection).toEqual(expect.any(Array));
  });

  it('uses a lightweight transcript fetch when switching to a live conversation', async () => {
    const messageState = { collection: [] };
    const conversationState = { values: { id: 'conv-live-target', queuedTurns: [] } };
    const context = {
      resources: {
        chat: {
          lastSinceCursor: '',
          lastConversationID: 'conv-live-target',
          transcriptRows: [],
          renderRows: [],
          liveRows: [],
          lastQueuedTurns: [],
          lastHasRunning: false,
          runningTurnId: '',
          activeConversationID: 'conv-live-target',
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] }),
                setFormData: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    client.getConversation.mockResolvedValueOnce({ id: 'conv-live-target', title: 'live', status: 'running' });
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        conversationId: 'conv-live-target',
        turns: []
      },
      feeds: []
    });
    client.streamEvents = vi.fn(() => ({ close: vi.fn() }));
    client.listGeneratedFiles.mockResolvedValueOnce([]);

    await switchConversation(context, 'conv-live-target');

    expect(client.getTranscript).toHaveBeenCalledWith(
      expect.objectContaining({
        conversationId: 'conv-live-target',
        includeModelCalls: false,
        includeToolCalls: false,
        includeFeeds: false,
        since: undefined
      }),
      undefined
    );
    expect(messageState.collection).toEqual(expect.any(Array));
  });

  it('connects stream immediately when switching to a visible idle conversation', async () => {
    const messageState = { collection: [] };
    const conversationState = { values: { id: 'conv-idle-target', queuedTurns: [] } };
    const context = {
      resources: {
        chat: {
          lastSinceCursor: '',
          lastConversationID: 'conv-idle-target',
          transcriptRows: [],
          renderRows: [],
          liveRows: [],
          lastQueuedTurns: [],
          lastHasRunning: false,
          runningTurnId: '',
          activeConversationID: 'conv-idle-target',
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] }),
                setFormData: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    client.getConversation.mockResolvedValueOnce({ id: 'conv-idle-target', title: 'idle', status: '' });
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        conversationId: 'conv-idle-target',
        turns: []
      },
      feeds: []
    });
    client.streamEvents = vi.fn(() => ({ close: vi.fn() }));
    client.listGeneratedFiles.mockResolvedValueOnce([]);

    await switchConversation(context, 'conv-idle-target');

    expect(client.streamEvents).toHaveBeenCalledWith('conv-idle-target', expect.any(Object));
  });
});

describe('bootstrapConversationSelection', () => {
  it('hydrates a child chat window from window parameters when no scoped selection exists', () => {
    const conversationState = { values: {} };
    activeWindows.value = [{
      windowId: 'child-window',
      windowKey: 'chat/new',
      parameters: {
        conversations: {
          form: {
            id: 'conv-from-run'
          }
        },
        messages: {
          input: {
            parameters: {
              convID: 'conv-from-run'
            }
          }
        }
      }
    }];
    global.window = {
      location: { pathname: '/' },
      localStorage: createStorage()
    };

    bootstrapConversationSelection({
      identity: { windowId: 'child-window' },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        return null;
      }
    });

    expect(conversationState.values.id).toBe('conv-from-run');
  });
});

describe('renderMergedRowsForContext', () => {
  it('renders a starter row on empty chat and merges all agent tasks for auto-select', () => {
    const messageState = { collection: [] };
    const context = {
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: '', agent: 'auto', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  agent: 'auto',
                  defaults: { agent: 'coder' },
                  agentInfos: [
                    { id: 'coder', name: 'Coder', starterTasks: [{ id: 'analyze', title: 'Analyze', prompt: 'Analyze repo.' }] },
                    { id: 'chatter', name: 'Chatter', starterTasks: [{ id: 'summarize', title: 'Summarize', prompt: 'Summarize this.' }] }
                  ]
                })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    expect(messageState.collection).toHaveLength(0);
  });

  it('does not render starter tasks when a conversation id is already present', () => {
    const messageState = { collection: [] };
    const context = {
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-new', agent: 'steward', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  agent: 'steward',
                  defaults: { agent: 'steward' },
                  agentInfos: [
                    { id: 'steward', name: 'Steward', starterTasks: [{ id: 'analyze', title: 'Analyze campaign performance', prompt: 'Analyze campaign 12345 performance.' }] },
                  ]
                })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    expect(messageState.collection).toHaveLength(0);
  });

  it('preserves normalized live streaming content instead of raw stream payload', () => {
    const messageState = { collection: [] };
    const context = {
      resources: {
        chat: {
          transcriptRows: [],
          liveRows: [{
            id: 'assistant:turn-1:1',
            role: 'assistant',
            turnId: 'turn-1',
            createdAt: '2026-03-26T12:00:00Z',
            interim: 1,
            isStreaming: true,
            content: 'hello',
            _streamContent: '```markdown\nhello\n```'
          }],
          renderRows: [],
          runningTurnId: 'turn-1',
          lastHasRunning: true,
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1']
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ agentInfos: [], defaults: {} })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    expect(messageState.collection).toHaveLength(1);
    expect(messageState.collection[0].content).toBe('hello');
  });

  it('renders progressive text-delta content when the tracker owns the assistant row', () => {
    const messageState = { collection: [] };
    const chatState = ensureContextResources({ resources: {} });
    chatState.activeConversationID = 'conv-1';
    chatState.liveOwnedConversationID = 'conv-1';
    chatState.liveOwnedTurnIds = ['turn-1'];
    chatState.liveRows = [{
      id: 'msg-1',
      role: 'assistant',
      turnId: 'turn-1',
      createdAt: '2026-03-26T12:00:00Z',
      interim: 1,
      isStreaming: true,
      content: 'hello',
      _streamContent: 'hello'
    }];
    chatState.streamTracker.applyEvent({
      type: 'model_started',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      messageId: 'msg-1',
      assistantMessageId: 'msg-1',
      status: 'running',
      model: { provider: 'openai', model: 'gpt-5.4' },
      createdAt: '2026-03-26T12:00:00Z'
    });
    chatState.streamTracker.applyEvent({
      type: 'text_delta',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      id: 'msg-1',
      assistantMessageId: 'msg-1',
      content: 'hello'
    });

    const context = {
      resources: { chat: chatState },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ agentInfos: [], defaults: {} })
              }
            }
          };
        }
        return null;
      }
    };

    const rows = renderMergedRowsForContext(context);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: 'msg-1',
      content: 'hello',
      isStreaming: true,
      _streamContent: 'hello'
    });
    expect(messageState.collection).toHaveLength(1);
    expect(messageState.collection[0].content).toBe('hello');
  });

  it('keeps the same-turn user message above assistant iterations even if the user timestamp drifts later', () => {
    const messageState = { collection: [] };
    const context = {
      resources: {
        chat: {
          transcriptRows: [],
          liveRows: [
            {
              id: 'assistant-live-1',
              role: 'assistant',
              turnId: 'turn-1',
              createdAt: '2026-03-26T12:00:00Z',
              interim: 1,
              status: 'running',
              turnStatus: 'running',
              content: 'Calling updatePlan.',
              narration: 'Calling updatePlan.',
              executionGroups: [
                {
                  assistantMessageId: 'assistant-live-1',
                  iteration: 1,
                  narration: 'Calling updatePlan.',
                  status: 'running'
                }
              ]
            },
            {
              id: 'user:turn-1',
              role: 'user',
              turnId: 'turn-1',
              createdAt: '2026-03-26T12:00:03Z',
              content: 'Forecast inventory and uniques for this targeting set: ad deal 147540'
            }
          ],
          renderRows: [],
          runningTurnId: 'turn-1',
          lastHasRunning: true,
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1']
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ agentInfos: [], defaults: {} })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    expect(messageState.collection).toHaveLength(2);
    expect(messageState.collection[0]).toMatchObject({
      role: 'user',
      turnId: 'turn-1'
    });
    expect(messageState.collection[1]).toMatchObject({
      _type: 'iteration',
      role: 'assistant'
    });
  });

  it('renders normalized user task content instead of the expanded task wrapper', () => {
    const messageState = { collection: [] };
    const context = {
      resources: {
        chat: {
          transcriptRows: [],
          liveRows: [
            {
              id: 'user:turn-1',
              role: 'user',
              mode: 'task',
              turnId: 'turn-1',
              createdAt: '2026-04-09T18:05:23Z',
              content: 'User Query:\nwhat iris targeting do we have ?\nContext:\nmap[Projection:map[hiddenMessageIds:[] hiddenTurnIds:[] reason: scope:conversation tokensFreed:0]]',
              rawContent: 'what iris targeting do we have ?'
            },
            {
              id: 'assistant-live-1',
              role: 'assistant',
              turnId: 'turn-1',
              createdAt: '2026-04-09T18:05:31Z',
              interim: 1,
              status: 'running',
              turnStatus: 'running',
              executionGroups: [
                {
                  assistantMessageId: 'assistant-live-1',
                  iteration: 1,
                  narration: 'Checking targeting tree…',
                  status: 'running'
                }
              ]
            }
          ],
          renderRows: [],
          runningTurnId: 'turn-1',
          lastHasRunning: true,
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1']
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ agentInfos: [], defaults: {} })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    expect(messageState.collection[0]).toMatchObject({
      role: 'user',
      turnId: 'turn-1',
      content: 'what iris targeting do we have ?'
    });
  });

  it('keeps first and second turn execution blocks separate while normalizing task wrappers', () => {
    const messageState = { collection: [] };
    const context = {
      resources: {
        chat: {
          transcriptRows: [],
          liveRows: [
            {
              id: 'user:turn-1',
              role: 'user',
              mode: 'task',
              turnId: 'turn-1',
              createdAt: '2026-04-09T18:05:23Z',
              content: 'User Query:\nwhat iris targeting do we have ?\nContext:\nmap[Projection:map[hiddenMessageIds:[] hiddenTurnIds:[]]]',
              rawContent: 'what iris targeting do we have ?'
            },
            {
              id: 'assistant:turn-1:1',
              role: 'assistant',
              turnId: 'turn-1',
              createdAt: '2026-04-09T18:05:31Z',
              interim: 0,
              status: 'completed',
              turnStatus: 'completed',
              content: 'First answer',
              executionGroups: [
                {
                  assistantMessageId: 'assistant:turn-1:1',
                  iteration: 1,
                  finalResponse: true,
                  status: 'completed',
                  content: 'First answer'
                }
              ]
            },
            {
              id: 'user:turn-2',
              role: 'user',
              mode: 'task',
              turnId: 'turn-2',
              createdAt: '2026-04-09T18:06:23Z',
              content: 'User Query:\nforecast deal 141952\nContext:\nmap[Projection:map[hiddenMessageIds:[] hiddenTurnIds:[]]]',
              rawContent: 'forecast deal 141952'
            },
            {
              id: 'assistant:turn-2:1',
              role: 'assistant',
              turnId: 'turn-2',
              createdAt: '2026-04-09T18:06:31Z',
              interim: 1,
              status: 'running',
              turnStatus: 'running',
              content: 'Checking forecast…',
              executionGroups: [
                {
                  assistantMessageId: 'assistant:turn-2:1',
                  iteration: 1,
                  narration: 'Checking forecast…',
                  status: 'running'
                }
              ]
            }
          ],
          renderRows: [],
          runningTurnId: 'turn-2',
          lastHasRunning: true,
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: ['turn-1', 'turn-2']
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] })
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ agentInfos: [], defaults: {} })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    const userRows = messageState.collection.filter((row) => String(row?.role || '').toLowerCase() === 'user');
    const iterationRows = messageState.collection.filter((row) => row?._type === 'iteration');
    expect(userRows).toHaveLength(2);
    expect(userRows.map((row) => row.content)).toEqual([
      'what iris targeting do we have ?',
      'forecast deal 141952'
    ]);
    expect(iterationRows).toHaveLength(2);
    expect(iterationRows.map((row) => row?._iterationData?.turnId)).toEqual(['turn-1', 'turn-2']);
  });

  it('preserves assistant final responses from canonical turns even when execution pages are absent', async () => {
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        turns: [
          {
            turnId: 'turn-1',
            status: 'completed',
            createdAt: '2026-04-09T18:39:37Z',
            user: {
              messageId: 'u1',
              content: 'what iris targeting do we have ?'
            },
            assistant: {
              final: {
                messageId: 'a1',
                content: '**Top Summary**\n\n- Final answer'
              }
            },
            execution: {
              pages: []
            }
          }
        ]
      }
    });

    const turns = await fetchTranscript('conv-final-only');
    expect(turns).toHaveLength(1);
    expect(turns[0]).toMatchObject({
      turnId: 'turn-1',
      assistant: {
        final: {
          messageId: 'a1',
          content: '**Top Summary**\n\n- Final answer'
        }
      }
    });
    const { rows } = mapTranscriptToRows(turns, { holdAfterTurnId: '', pendingElicitations: [] });
    expect(rows).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'a1',
        role: 'assistant',
        content: '**Top Summary**\n\n- Final answer',
        turnId: 'turn-1'
      })
    ]));
  });

  it('preserves user, execution, and assistant rows for each completed canonical turn on reload', async () => {
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        turns: [
          {
            turnId: 'turn-1',
            status: 'completed',
            createdAt: '2026-04-09T18:39:37Z',
            user: { messageId: 'u1', content: 'what iris targeting do we have ?' },
            execution: {
              pages: [
                {
                  pageId: 'page-1',
                  assistantMessageId: 'a1',
                  turnId: 'turn-1',
                  iteration: 1,
                  status: 'completed',
                  finalResponse: true,
                  content: '**Top Summary**\n\n- First answer'
                }
              ]
            },
            assistant: {
              final: {
                messageId: 'a1',
                content: '**Top Summary**\n\n- First answer'
              }
            }
          },
          {
            turnId: 'turn-2',
            status: 'completed',
            createdAt: '2026-04-09T18:40:30Z',
            user: { messageId: 'u2', content: 'find iris options for sports' },
            execution: {
              pages: [
                {
                  pageId: 'page-2',
                  assistantMessageId: 'a2',
                  turnId: 'turn-2',
                  iteration: 1,
                  status: 'completed',
                  finalResponse: true,
                  content: '**Top Summary**\n\n- Second answer'
                }
              ]
            },
            assistant: {
              final: {
                messageId: 'a2',
                content: '**Top Summary**\n\n- Second answer'
              }
            }
          }
        ]
      }
    });

    const turns = await fetchTranscript('conv-multi-turn');
    const rows = mapTranscriptToRows(turns, { holdAfterTurnId: '', pendingElicitations: [] }).rows;

    expect(rows.map((row) => String(row?.role || ''))).toEqual([
      'user',
      'assistant',
      'user',
      'assistant'
    ]);
    expect(rows[0]).toMatchObject({ content: 'what iris targeting do we have ?', turnId: 'turn-1' });
    expect(rows[1]).toMatchObject({
      id: 'a1',
      role: 'assistant',
      turnId: 'turn-1',
      executionGroups: [expect.objectContaining({ pageId: 'page-1' })]
    });
    expect(rows[2]).toMatchObject({ content: 'find iris options for sports', turnId: 'turn-2' });
    expect(rows[3]).toMatchObject({
      id: 'a2',
      role: 'assistant',
      turnId: 'turn-2',
      executionGroups: [expect.objectContaining({ pageId: 'page-2' })]
    });
  });

  it('renders the latest active execution page instead of an earlier completed page for active multi-page turns', () => {
    const turns = [
      {
        turnId: 'turn-1',
        status: 'running',
        createdAt: '2026-04-10T08:30:00Z',
        user: { messageId: 'u1', content: 'write story with 200 sentences about cat' },
        execution: {
          pages: [
            {
              pageId: 'page-select',
              assistantMessageId: 'page-select',
              turnId: 'turn-1',
              iteration: 1,
              status: 'completed',
              finalResponse: true,
              content: '{"agentId":"chatter"}'
            },
            {
              pageId: 'page-stream',
              assistantMessageId: 'page-stream',
              turnId: 'turn-1',
              iteration: 2,
              status: 'streaming',
              finalResponse: false,
              content: '1) Miso was a small tabby cat.',
              narration: 'Writing story...'
            }
          ]
        }
      }
    ];

    const { rows } = mapTranscriptToRows(turns);
    const assistant = rows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    expect(assistant).toMatchObject({
      id: 'page-stream',
      content: '1) Miso was a small tabby cat.',
      narration: 'Writing story...',
      interim: 1,
      status: 'streaming'
    });
  });

  it('does not fall back to an earlier completed selector page while the latest auto-selected page is still streaming', () => {
    const turns = [
      {
        turnId: 'turn-1',
        status: 'running',
        createdAt: '2026-04-10T08:30:00Z',
        user: { messageId: 'u1', content: 'write story with 200 sentences about cat' },
        assistant: {
          final: {
            messageId: 'page-select',
            content: '{"agentId":"chatter"}'
          }
        },
        execution: {
          pages: [
            {
              pageId: 'page-select',
              assistantMessageId: 'page-select',
              turnId: 'turn-1',
              iteration: 1,
              status: 'completed',
              finalResponse: true,
              content: '{"agentId":"chatter"}'
            },
            {
              pageId: 'page-stream',
              assistantMessageId: 'page-stream',
              turnId: 'turn-1',
              iteration: 2,
              status: 'streaming',
              finalResponse: false,
              content: '',
              narration: ''
            }
          ]
        }
      }
    ];

    const { rows } = mapTranscriptToRows(turns);
    const assistant = rows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    expect(assistant).toMatchObject({
      id: 'page-stream',
      content: '',
      interim: 1,
      status: 'streaming'
    });
  });

});

describe('getCurrentConversationID fallback behavior', () => {
  it('uses activeConversationID from chat state when the conversation form id is still blank', async () => {
    client.getTranscript.mockReset();
    const context = {
      resources: {
        chat: {
          activeConversationID: 'conv-1',
          liveOwnedConversationID: 'conv-1',
          liveOwnedTurnIds: [],
          activeStreamPrompt: 'recommend frequency cap for ctv',
          runningTurnId: '',
          activeStreamTurnId: '',
          lastHasRunning: false,
          transcriptRows: [],
          liveRows: [],
          lastQueuedTurns: []
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: '', running: false }),
                setFormData: vi.fn()
              }
            }
          };
        }
        return null;
      }
    };

    const result = await dsTick(context);

    expect(client.getTranscript).not.toHaveBeenCalled();
    expect(result?.deferredToLiveStream).toBe(true);
    expect(result?.conversationID).toBe('conv-1');
  });
});

describe('startPolling', () => {
  it('does not poll finished conversations once transcript is already loaded', async () => {
    vi.useFakeTimers();
    const context = {
      resources: {
        chat: {
          transcriptRows: [{ id: 'm1', role: 'assistant', turnId: 'turn-1', content: 'done' }],
          liveRows: [],
          renderRows: [],
          lastHasRunning: false,
          activeStreamTurnId: '',
          runningTurnId: '',
          lastStreamEventAt: 0,
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-1', queuedTurns: [] }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {}, agentInfos: [] })
              }
            }
          };
        }
        return null;
      }
    };

    client.getTranscript.mockClear();
    startPolling(context);
    await vi.advanceTimersByTimeAsync(4500);
    expect(client.getTranscript).not.toHaveBeenCalled();
    stopPolling(context);
    vi.useRealTimers();
  });
});

describe('createNewConversation', () => {
  it('restores starter tasks immediately after switching away from a populated conversation', async () => {
    const messageState = { collection: [{ id: 'old-msg', role: 'assistant', content: 'existing' }] };
    const conversationState = { values: { id: 'conv-old', agent: 'steward', queuedTurns: [] } };
    const metaState = {
      values: {
        agent: 'steward',
        defaults: { agent: 'steward' },
        agentInfos: [
          { id: 'steward', name: 'Steward', starterTasks: [{ id: 'analyze', title: 'Analyze campaign performance', prompt: 'Analyze campaign 12345 performance.' }] },
        ]
      }
    };

    const context = {
      resources: {},
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: (rows) => { messageState.collection = rows; },
                setError: vi.fn()
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => conversationState.values,
                setFormData: ({ values }) => { conversationState.values = values; }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => metaState.values,
                setFormData: ({ values }) => { metaState.values = values; }
              }
            }
          };
        }
        return null;
      }
    };

    await createNewConversation(context);

    expect(conversationState.values.id).toBe('');
    expect(messageState.collection).toHaveLength(0);
    expect(metaState.values.starterTasks).toHaveLength(1);
    expect(metaState.values.starterTasks[0]).toMatchObject({
      title: 'Analyze campaign performance',
    });
  });

  it('ignores a persisted agent from another workspace so current workspace starter tasks still appear', async () => {
    const originalWindow = global.window;
    const originalLocalStorage = global.localStorage;
    const storage = {
      getItem: (key) => (key === 'agently.selectedAgent' ? 'chatter' : ''),
      setItem: vi.fn(),
      removeItem: vi.fn()
    };
    global.window = {
      localStorage: storage
    };
    global.localStorage = storage;

    try {
      const messageState = { collection: [{ id: 'old-msg', role: 'assistant', content: 'existing' }] };
      const conversationState = { values: { id: 'conv-old', agent: '', queuedTurns: [] } };
      const metaState = {
        values: {
          agent: 'steward',
          defaults: { agent: 'steward' },
          agentInfos: [
            { id: 'steward', name: 'Steward', starterTasks: [{ id: 'analyze', title: 'Analyze campaign performance', prompt: 'Analyze campaign 12345 performance.' }] },
          ]
        }
      };

      const context = {
        resources: {},
        Context(name) {
          if (name === 'messages') {
            return {
              handlers: {
                dataSource: {
                  setCollection: (rows) => { messageState.collection = rows; },
                  setError: vi.fn()
                }
              }
            };
          }
          if (name === 'conversations') {
            return {
              handlers: {
                dataSource: {
                  peekFormData: () => conversationState.values,
                  setFormData: ({ values }) => { conversationState.values = values; }
                }
              }
            };
          }
          if (name === 'meta') {
            return {
              handlers: {
                dataSource: {
                  peekFormData: () => metaState.values,
                  setFormData: ({ values }) => { metaState.values = values; }
                }
              }
            };
          }
          return null;
        }
      };

      await createNewConversation(context);

      expect(conversationState.values.agent).toBe('steward');
      expect(messageState.collection).toHaveLength(0);
      expect(metaState.values.starterTasks?.[0]).toMatchObject({
        title: 'Analyze campaign performance'
      });
    } finally {
      global.window = originalWindow;
      global.localStorage = originalLocalStorage;
    }
  });
});

describe('mapTranscriptToRows', () => {
  it('keeps canonical iteration-0 summary pages out of the visible assistant message but includes them in execution pages', async () => {
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        turns: [
          {
            turnId: 'turn-1',
            status: 'completed',
            createdAt: '2026-01-01T10:00:00Z',
            linkedConversations: [
              {
                conversationId: 'child-1',
                agentId: 'steward-forecasting',
                title: 'Forecasting Child',
                status: 'completed',
                response: 'Forecast completed.',
                createdAt: '2026-01-01T10:01:00Z'
              }
            ],
            user: {
              messageId: 'u1',
              content: 'Analyze campaign 547754 performance'
            },
            execution: {
              pages: [
                {
                  pageId: 'page-final',
                  assistantMessageId: 'page-final',
                  turnId: 'turn-1',
                  iteration: 11,
                  status: 'completed',
                  finalResponse: true,
                  content: '## Highlights\n- Campaign pacing is slightly behind target.'
                },
                {
                  pageId: 'page-summary',
                  assistantMessageId: 'page-summary',
                  turnId: 'turn-1',
                  iteration: 0,
                  status: 'completed',
                  finalResponse: true,
                  content: 'Title: Campaign 547754 Performance Analysis and Recommended Next Actions\n\n- Saved 3 actionable recommendations'
                }
              ]
            }
          }
        ]
      }
    });

    const turns = await fetchTranscript('conv-1');
    expect(turns).toHaveLength(1);
    expect(turns[0].execution.pages).toHaveLength(2);
    expect(turns[0].execution.pages[0]).toMatchObject({
      pageId: 'page-final',
      iteration: 11
    });
    expect(turns[0].execution.pages[1]).toMatchObject({
      pageId: 'page-summary',
      iteration: 0
    });
    expect(turns[0].linkedConversations).toEqual([
      expect.objectContaining({
        conversationId: 'child-1',
        agentId: 'steward-forecasting',
        title: 'Forecasting Child',
        status: 'completed'
      })
    ]);
    const { rows } = mapTranscriptToRows(turns);
    expect(rows).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'page-summary',
        role: 'assistant',
        content: 'Title: Campaign 547754 Performance Analysis and Recommended Next Actions\n\n- Saved 3 actionable recommendations',
        executionGroups: expect.arrayContaining([
          expect.objectContaining({
            pageId: 'page-final'
          }),
          expect.objectContaining({
            pageId: 'page-summary',
            iteration: 0
          })
        ])
      }),
      expect.objectContaining({
        id: 'linked:child-1',
        role: 'tool',
        reason: 'link',
        toolName: 'llm/agents/run',
        linkedConversationId: 'child-1',
        linkedConversationAgentId: 'steward-forecasting',
        linkedConversationTitle: 'Forecasting Child',
        status: 'completed',
        response: 'Forecast completed.'
      })
    ]));
  });

  it('keeps failed canonical execution pages visible and carries turn error text', async () => {
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        turns: [
          {
            turnId: 'turn-failed',
            status: 'failed',
            createdAt: '2026-04-01T12:00:00Z',
            errorMessage: 'failed to stream: dial tcp: lookup api.openai.com: no such host',
            user: {
              messageId: 'u-failed',
              content: 'Forecast inventory'
            },
            execution: {
              pages: [
                {
                  pageId: 'page-failed',
                  assistantMessageId: 'page-failed',
                  turnId: 'turn-failed',
                  iteration: 1,
                  status: 'failed',
                  finalResponse: false,
                  errorMessage: 'failed to stream: dial tcp: lookup api.openai.com: no such host'
                }
              ]
            }
          }
        ]
      }
    });

    const turns = await fetchTranscript('conv-failed');
    expect(turns).toHaveLength(1);
    expect(turns[0]).toMatchObject({
      turnId: 'turn-failed',
      status: 'failed',
      errorMessage: 'failed to stream: dial tcp: lookup api.openai.com: no such host',
      execution: {
        pages: [expect.objectContaining({ pageId: 'page-failed', status: 'failed' })]
      }
    });
    const { rows } = mapTranscriptToRows(turns);
    expect(rows).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'page-failed',
        role: 'assistant',
        status: 'failed',
        errorMessage: 'failed to stream: dial tcp: lookup api.openai.com: no such host'
      })
    ]));
  });

  it('hydrates transcript elicitation rows from embedded assistant JSON and suppresses the raw JSON bubble', async () => {
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        turns: [
          {
            turnId: 'turn-elic',
            status: 'completed',
            createdAt: '2026-04-01T12:00:00Z',
            user: {
              messageId: 'u-elic',
              content: 'Use system_os-getEnv to tell me an environment variable.'
            },
            execution: {
              pages: [
                {
                  pageId: 'page-elic',
                  assistantMessageId: 'page-elic',
                  turnId: 'turn-elic',
                  iteration: 1,
                  status: 'completed',
                  finalResponse: true,
                  content: '{"type":"elicitation","message":"Please provide the environment variable name for system_os-getEnv.","requestedSchema":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}}{"type":"elicitation","message":"Please provide the environment variable name for system_os-getEnv.","requestedSchema":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}}'
                }
              ]
            },
            assistant: {
              final: {
                messageId: 'page-elic',
                content: '{"type":"elicitation","message":"Please provide the environment variable name for system_os-getEnv.","requestedSchema":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}}'
              }
            },
            elicitation: {
              elicitationId: 'elic-1',
              status: 'pending',
              message: '{"name":""}'
            }
          }
        ]
      }
    });

    const turns = await fetchTranscript('conv-elic');
    expect(turns).toHaveLength(1);
    const { rows } = mapTranscriptToRows(turns);
    expect(rows).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'page-elic',
        role: 'assistant',
        content: ''
      }),
      expect.objectContaining({
        id: 'elicitation:elic-1',
        role: 'assistant',
        elicitationId: 'elic-1',
        content: 'Please provide the environment variable name for system_os-getEnv.',
        elicitation: expect.objectContaining({
          elicitationId: 'elic-1',
          message: 'Please provide the environment variable name for system_os-getEnv.',
          requestedSchema: {
            type: 'object',
            properties: {
              name: { type: 'string' }
            },
            required: ['name']
          }
        })
      })
    ]));
  });

  it('ensureConversation reuses the scoped active conversation when the form id is transiently empty', async () => {
    const existingConversation = {
      id: 'conv-existing',
      title: 'Existing conversation'
    };
    client.getConversation.mockResolvedValueOnce(existingConversation);

    const context = {
      identity: { windowId: 'chat/main' },
      resources: {
        chat: {
          activeConversationID: 'conv-existing'
        }
      },
      Context: (name) => {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: '', title: 'New conversation', agent: 'steward', model: 'openai_gpt-5_4' }),
                setFormData: vi.fn()
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: { agent: 'steward', model: 'openai_gpt-5_4' } })
              }
            }
          };
        }
        return { handlers: { dataSource: {} } };
      }
    };

    const originalWindow = global.window;
    global.window = {
      location: { pathname: '/conversation/conv-existing' },
      localStorage: { getItem: vi.fn(() => '') },
      dispatchEvent: vi.fn()
    };

    try {
      const id = await ensureConversation(context);
      expect(id).toBe('conv-existing');
      expect(client.createConversation).not.toHaveBeenCalled();
    } finally {
      global.window = originalWindow;
    }
  });

  it('ensureConversation creates a fresh conversation after explicit new conversation reset', async () => {
    vi.clearAllMocks();
    client.createConversation.mockResolvedValueOnce({
      id: 'conv-new',
      title: 'New chat'
    });

    const setFormData = vi.fn();
    const context = {
      identity: { windowId: 'chat/main' },
      resources: {
        chat: {
          activeConversationID: 'conv-existing',
          explicitNewConversationRequested: true
        }
      },
      Context: (name) => {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: '', title: 'New conversation', agent: 'steward', model: 'openai_gpt-5_4' }),
                setFormData
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: { agent: 'steward', model: 'openai_gpt-5_4' } })
              }
            }
          };
        }
        return { handlers: { dataSource: {} } };
      }
    };

    const originalWindow = global.window;
    global.window = {
      location: { pathname: '/conversation/conv-existing' },
      localStorage: {
        getItem: vi.fn(() => ''),
        setItem: vi.fn(),
        removeItem: vi.fn()
      },
      history: { state: null, replaceState: vi.fn() },
      dispatchEvent: vi.fn()
    };

    try {
      const id = await ensureConversation(context);
      expect(id).toBe('conv-new');
      expect(client.getConversation).not.toHaveBeenCalled();
      expect(client.createConversation).toHaveBeenCalledTimes(1);
      expect(context.resources.chat.activeConversationID).toBe('conv-new');
      expect(context.resources.chat.explicitNewConversationRequested).toBe(false);
    } finally {
      global.window = originalWindow;
    }
  });

});

describe('shouldUseLiveStream', () => {
  it('uses stream only for conversations owned by the current live session', () => {
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: 'conv-live',
          liveOwnedTurnIds: ['turn-1']
        }
      }
    };

    expect(shouldUseLiveStream(context, 'conv-live')).toBe(true);
    expect(shouldUseLiveStream(context, 'conv-transcript')).toBe(false);
  });

  it('uses stream for the visible conversation even before it is marked running', () => {
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-visible', running: false })
              }
            }
          };
        }
        return null;
      }
    };

    expect(shouldUseLiveStream(context, 'conv-visible')).toBe(true);
    expect(shouldUseLiveStream(context, 'conv-other')).toBe(false);
  });

  it('uses stream for the visible conversation when stage says it is still live', () => {
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-visible', running: false, stage: 'executing', status: 'running' })
              }
            }
          };
        }
        return null;
      }
    };

    expect(shouldUseLiveStream(context, 'conv-visible')).toBe(true);
  });

  it('uses stream for the visible conversation once submit has already claimed live ownership', () => {
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: 'conv-visible',
          liveOwnedTurnIds: [],
          runningTurnId: '',
          activeStreamTurnId: ''
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-visible', running: false, stage: 'idle', status: 'idle' })
              }
            }
          };
        }
        return null;
      }
    };

    expect(shouldUseLiveStream(context, 'conv-visible')).toBe(true);
  });

  it('does not use stream for a selected finished conversation', () => {
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: '',
          liveOwnedTurnIds: [],
          runningTurnId: '',
          activeStreamTurnId: ''
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-finished', running: false })
              }
            }
          };
        }
        return null;
      }
    };

    expect(shouldUseLiveStream(context, 'conv-finished')).toBe(true);
  });

  it('allows forced transcript refresh scheduling even while live owns the turn', () => {
    const originalWindow = global.window;
    global.window = {
      setTimeout,
      clearTimeout,
      localStorage: createStorage(),
      location: { pathname: '/conversation/conv-live' }
    };
    const context = {
      resources: {
        chat: {
          liveOwnedConversationID: 'conv-live',
          liveOwnedTurnIds: ['turn-1'],
          runningTurnId: 'turn-1',
          activeStreamTurnId: 'turn-1',
          lastHasRunning: true
        }
      },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-live', running: true })
              }
            }
          };
        }
        return null;
      }
    };

    try {
      const deferred = queueTranscriptRefresh(context, { delay: 1000 });
      expect(deferred).toBeNull();

      const forced = queueTranscriptRefresh(context, { delay: 1000, force: true });
      expect(forced).not.toBeNull();
      clearTimeout(forced);
    } finally {
      global.window = originalWindow;
    }
  });

  it('forwards only static transcript rows into chatStore while the latest turn is live-owned', async () => {
    const onTranscript = vi.fn();
    installChatStoreMirror({ onTranscript });
    const originalWindow = global.window;
    global.window = {
      __agentlyActiveChatState: {
        liveOwnedConversationID: 'conv-live',
        liveOwnedTurnIds: ['turn-1'],
        runningTurnId: 'turn-1',
        activeStreamTurnId: '',
        lastHasRunning: true,
      }
    };
    client.getTranscript.mockResolvedValueOnce({
      conversation: {
        conversationId: 'conv-live',
        turns: [{ turnId: 'turn-1', status: 'running' }],
        feeds: [{ feedId: 'plan', title: 'Plan', itemCount: 2, data: { note: 'transcript' } }],
      }
    });

    try {
      const turns = await fetchTranscript('conv-live');
      expect(turns).toHaveLength(1);
      expect(onTranscript).not.toHaveBeenCalled();
      expect(global.window.__agentlyActiveChatState.lastTranscriptFeedsByConversation).toBeUndefined();
    } finally {
      installChatStoreMirror(null);
      global.window = originalWindow;
    }
  });
});

describe('resolveLastTranscriptCursor', () => {
  it('skips synthetic linked conversation rows', () => {
    const turns = [
      {
        id: 'turn-1',
        message: [
          { id: 'm1', role: 'assistant' },
          { id: 'linked:child-1', role: 'tool', linkedConversationId: 'child-1' }
        ]
      }
    ];

    expect(resolveLastTranscriptCursor(turns)).toBe('m1');
  });
});

describe('renderMergedRowsForContext', () => {
  it('does not append a synthetic queue row when queued turns are present on the conversation form', () => {
    let collection = [];
    const context = {
      resources: {
        chat: {
          transcriptRows: [
            { id: 'u1', role: 'user', createdAt: '2026-03-17T12:00:00Z', content: 'hi' }
          ],
          liveRows: [],
          renderRows: [],
          runningTurnId: '',
          lastHasRunning: false,
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection(next) {
                  collection = next;
                }
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    id: 'conv-1',
                    running: true,
                    queuedTurns: [
                      { id: 'turn-q1', preview: 'queued follow-up' }
                    ]
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);

    expect(collection.some((row) => row?._type === 'queue')).toBe(false);
  });

  it('keeps rendered rows deterministic across rerenders when queued turns are present', () => {
    let firstCollection = [];
    let secondCollection = [];
    const context = {
      resources: {
        chat: {
          transcriptRows: [],
          liveRows: [],
          renderRows: [],
          runningTurnId: '',
          lastHasRunning: false,
          liveOwnedConversationID: '',
          liveOwnedTurnIds: []
        }
      },
      Context(name) {
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection(next) {
                  if (firstCollection.length === 0) {
                    firstCollection = next;
                  } else {
                    secondCollection = next;
                  }
                }
              }
            }
          };
        }
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    id: '',
                    agent: 'auto',
                    running: true,
                    queuedTurns: [{ id: 'turn-q1', preview: 'queued follow-up' }]
                  };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  agent: 'auto',
                  defaults: { agent: 'coder' },
                  agentInfos: [
                    { id: 'coder', name: 'Coder', starterTasks: [{ id: 'analyze', title: 'Analyze', prompt: 'Analyze repo.' }] }
                  ]
                })
              }
            }
          };
        }
        return null;
      }
    };

    renderMergedRowsForContext(context);
    renderMergedRowsForContext(context);

    expect(secondCollection).toEqual(firstCollection);
  });
});
