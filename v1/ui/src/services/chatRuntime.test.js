import { describe, expect, it, vi } from 'vitest';

import { createNewConversation, handleStreamEvent, mapTranscriptToRows, normalizeMetaResponse, renderMergedRowsForContext, resolveLastTranscriptCursor, resolveStarterTasks, shouldUseLiveStream } from './chatRuntime';
import { client } from './agentlyClient';

vi.mock('./agentlyClient', () => ({
  client: {
    getWorkspaceMetadata: vi.fn(),
    createConversation: vi.fn(),
    getConversation: vi.fn()
  }
}));

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
    expect(got.modelOptions[1]).toMatchObject({ value: 'openai_o3', label: 'o3' });
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
});

describe('mapTranscriptToRows', () => {
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
