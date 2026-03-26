import { describe, expect, it, vi } from 'vitest';
import { activeWindows } from 'forge/core';

import { bootstrapConversationSelection, createNewConversation, ensureConversation, fetchTranscript, handleStreamEvent, mapTranscriptToRows, normalizeMetaResponse, renderMergedRowsForContext, resolveLastTranscriptCursor, resolveStarterTasks, shouldUseLiveStream } from './chatRuntime';
import { client } from './agentlyClient';

vi.mock('./agentlyClient', () => ({
  client: {
    getWorkspaceMetadata: vi.fn(),
    createConversation: vi.fn(),
    getConversation: vi.fn(),
    getTranscript: vi.fn()
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
  it('does not reference a raw MessageEvent when given a parsed SSE payload', () => {
    const chatState = { liveRows: [], lastHasRunning: false };
    expect(() => handleStreamEvent(chatState, {}, 'conv-1', {
      type: 'unknown_event',
      conversationId: 'conv-1',
      content: 'hello'
    })).not.toThrow();
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
});

describe('createNewConversation', () => {
  it('prefers persisted auto agent for a fresh draft conversation', async () => {
    const conversationState = { values: { id: 'old', agent: 'chatter', model: 'openai_gpt-5.4' } };
    const metaState = { values: { agent: 'chatter', defaults: { agent: 'chatter', model: 'openai_gpt-5.4', embedder: 'openai_text' } } };
    const context = {
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

    expect(messageState.collection).toHaveLength(1);
    expect(messageState.collection[0]).toMatchObject({
      _type: 'starter',
      subtitle: 'Auto-select agent'
    });
    expect(messageState.collection[0].starterTasks).toHaveLength(2);
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
    expect(messageState.collection).toHaveLength(1);
    expect(messageState.collection[0]).toMatchObject({
      _type: 'starter',
    });
    expect(messageState.collection[0].starterTasks).toHaveLength(1);
    expect(messageState.collection[0].starterTasks[0]).toMatchObject({
      title: 'Analyze campaign performance',
    });
  });
});

describe('mapTranscriptToRows', () => {
  it('keeps canonical iteration-0 summary pages out of the visible assistant message and execution pages', async () => {
    client.getTranscript.mockResolvedValueOnce({
      turns: [
        {
          turnId: 'turn-1',
          status: 'completed',
          createdAt: '2026-01-01T10:00:00Z',
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
    });

    const turns = await fetchTranscript('conv-1');
    expect(turns).toHaveLength(1);
    expect(turns[0].executionGroups).toHaveLength(1);
    expect(turns[0].executionGroups[0]).toMatchObject({
      pageId: 'page-final',
      iteration: 11
    });
    expect(turns[0].message).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'page-final',
          role: 'assistant',
          content: '## Highlights\n- Campaign pacing is slightly behind target.'
        }),
        expect.objectContaining({
          id: 'page-summary',
          role: 'assistant',
          mode: 'summary'
        })
      ])
    );
  });

  it('preserves backend executionGroup data on assistant rows', () => {
    const turns = [
      {
        id: 'turn-1',
        status: 'completed',
        executionGroups: [
          {
            parentMessageId: 'm1',
            modelMessageId: 'm1',
            sequence: 1,
            preamble: 'Inspecting repository layout.',
            finalResponse: false,
            modelCall: {
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'completed'
            },
            toolCalls: [
              {
                messageId: 'tm1',
                toolName: 'resources-list',
                status: 'completed'
              }
            ]
          }
        ],
        message: [
          {
            id: 'm1',
            role: 'assistant',
            interim: 1,
            content: 'Inspecting repository layout.',
            createdAt: '2026-03-14T12:00:00Z',
            modelCall: {
              messageId: 'm1',
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'completed'
            },
            toolMessage: [
              {
                id: 'tm1',
                parentMessageId: 'm1',
                createdAt: '2026-03-14T12:00:01Z',
                toolCall: {
                  messageId: 'tm1',
                  toolName: 'resources-list',
                  status: 'completed'
                }
              }
            ]
          }
        ]
      }
    ];

    const { rows } = mapTranscriptToRows(turns);
    expect(rows).toHaveLength(1);
    expect(rows[0].executionGroup).toMatchObject({
      parentMessageId: 'm1',
      sequence: 1
    });
    expect(rows[0].executionGroups).toHaveLength(1);
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

  it('propagates turn execution groups onto tool rows linked to the group', () => {
    const turns = [
      {
        id: 'turn-1',
        status: 'running',
        executionGroups: [
          {
            parentMessageId: 'm1',
            modelMessageId: 'm1',
            sequence: 1,
            preamble: 'Inspecting repository layout.',
            finalResponse: false,
            status: 'running',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'running'
            },
            toolMessages: [
              {
                id: 'tm1',
                linkedConversationId: 'child-1'
              }
            ],
            toolCalls: [
              {
                messageId: 'tm1',
                toolName: 'llm/agents:run',
                status: 'running'
              }
            ]
          }
        ],
        message: [
          {
            id: 'tm1',
            role: 'tool',
            type: 'tool_op',
            createdAt: '2026-03-15T10:00:00Z',
            toolName: 'llm/agents/run',
            linkedConversationId: 'child-1'
          }
        ]
      }
    ];

    const { rows } = mapTranscriptToRows(turns);
    expect(rows).toHaveLength(1);
    expect(rows[0].executionGroup).toMatchObject({
      parentMessageId: 'm1'
    });
    expect(rows[0].executionGroups).toHaveLength(1);
    expect(rows[0].executionGroups[0]).toMatchObject({
      parentMessageId: 'm1'
    });
  });

  it('does not attach execution groups to user rows', () => {
    const turns = [
      {
        id: 'turn-1',
        status: 'succeeded',
        executionGroups: [
          {
            parentMessageId: 'a1',
            modelMessageId: 'a1',
            sequence: 1,
            finalResponse: true,
            content: 'Hi!',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'completed'
            }
          }
        ],
        message: [
          {
            id: 'u1',
            role: 'user',
            rawContent: 'hi',
            createdAt: '2026-03-15T10:00:00Z'
          },
          {
            id: 'a1',
            role: 'assistant',
            interim: 0,
            content: 'Hi!',
            createdAt: '2026-03-15T10:00:01Z'
          }
        ]
      }
    ];

    const { rows } = mapTranscriptToRows(turns);
    expect(rows[0]).toMatchObject({
      id: 'u1',
      role: 'user'
    });
    expect(rows[0].executionGroup).toBeNull();
    expect(rows[0].executionGroups).toEqual([]);
  });

  it('extracts queued turn overrides from transcript turn fields and starter tags', () => {
    const turns = [
      {
        id: 'turn-q1',
        conversationId: 'conv-1',
        status: 'queued',
        queueSeq: 7,
        agentIdUsed: 'chatter',
        modelOverride: 'openai_gpt-5.2',
        startedByMessageId: 'msg-q1',
        createdAt: '2026-03-17T12:00:00Z',
        message: [
          {
            id: 'msg-q1',
            role: 'user',
            content: 'please review the last patch',
            tags: 'agently:queued_request:{"agent":"chatter","model":"openai_gpt-5.2","tools":["resources/list","resources/read"]}'
          }
        ]
      }
    ];

    const { queuedTurns } = mapTranscriptToRows(turns);
    expect(queuedTurns).toHaveLength(1);
    expect(queuedTurns[0]).toMatchObject({
      id: 'turn-q1',
      conversationId: 'conv-1',
      queueSeq: 7,
      content: 'please review the last patch',
      preview: 'please review the last patch'
    });
    expect(queuedTurns[0].overrides).toMatchObject({
      agent: 'chatter',
      model: 'openai_gpt-5.2',
      tools: ['resources/list', 'resources/read']
    });
  });

  it('keeps pending queue-like turns out of transcript rows', () => {
    const { rows, queuedTurns } = mapTranscriptToRows([
      {
        id: 'turn-p1',
        conversationId: 'conv-1',
        status: 'pending',
        startedByMessageId: 'msg-p1',
        message: [
          {
            id: 'msg-p1',
            role: 'user',
            content: 'check code smell'
          }
        ]
      }
    ]);

    expect(rows).toHaveLength(0);
    expect(queuedTurns).toHaveLength(1);
    expect(queuedTurns[0]).toMatchObject({
      id: 'turn-p1',
      preview: 'check code smell'
    });
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
  it('appends a synthetic queue row when queued turns are present on the conversation form', () => {
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

    expect(collection.some((row) => row?._type === 'queue')).toBe(true);
    expect(collection.find((row) => row?._type === 'queue')).toMatchObject({
      running: true,
      queuedTurns: [{ id: 'turn-q1', preview: 'queued follow-up' }]
    });
  });
});
