import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./agentlyClient', () => ({
  client: {
    executeTool: vi.fn(),
    downloadWorkspaceFile: vi.fn(),
  },
}));

vi.mock('./toolFeedBus', () => ({
  getFeedData: vi.fn(() => ({
    data: {
      output: {
        roots: [
          { id: 'local', uri: 'file://localhost/' },
        ],
      },
    },
  })),
  updateFeedData: vi.fn(),
  makeFeedKey: vi.fn((feedId, conversationId = '') => conversationId ? `${conversationId}::${feedId}` : feedId),
}));

vi.mock('./httpClient', () => ({
  showToast: vi.fn(),
}));

vi.mock('../utils/dialogBus', () => ({
  openFileViewDialog: vi.fn(),
  updateFileViewDialog: vi.fn(),
  openCodeDiffDialog: vi.fn(),
  updateCodeDiffDialog: vi.fn(),
}));

import { client } from './agentlyClient';
import { updateFeedData } from './toolFeedBus';
import {
  resolveSubmitAgent,
  resolveComposerProps,
  renderFeed,
  explorerSearch,
  explorerSearchInputChanged,
  taskStatusIcon,
  prepareChangeFiles,
  onChangedFileSelect,
  onFetchMeta,
  onFetchMessages,
  onFetchQueuedTurns,
  openResourceFeedPath,
} from './chatService';
import {
  openFileViewDialog,
  openCodeDiffDialog,
  updateFileViewDialog,
  updateCodeDiffDialog,
} from '../utils/dialogBus';

describe('resolveSubmitAgent', () => {
  it('prefers explicit selected auto agent over stale conversation/default agent', () => {
    expect(resolveSubmitAgent({
      selectedAgent: 'auto',
      persistedAgent: '',
      metaForm: { agent: 'chatter', defaults: { agent: 'chatter' } },
      convForm: { agent: 'chatter' }
    })).toBe('auto');
  });

  it('prefers persisted auto agent over stale conversation/default agent', () => {
    expect(resolveSubmitAgent({
      selectedAgent: '',
      persistedAgent: 'auto',
      metaForm: { agent: 'chatter', defaults: { agent: 'chatter' } },
      convForm: { agent: 'chatter' }
    })).toBe('auto');
  });
});

describe('renderFeed', () => {
  it('falls back to the conversations form id when explicit conversationId is empty', () => {
    const context = {
      identity: { windowId: 'chat/new' },
      resources: { chat: { activeConversationID: '' } },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-from-form' }),
              },
            },
          };
        }
        return null;
      },
    };

    const element = renderFeed({ conversationId: '', context });
    expect(element?.props?.conversationId).toBe('conv-from-form');
  });

  it('does not forward legacy messages into the canonical feed component', () => {
    const context = {
      identity: { windowId: 'chat/new' },
      resources: { chat: { activeConversationID: '' } },
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ id: 'conv-from-form' }),
              },
            },
          };
        }
        return null;
      },
    };

    const element = renderFeed({ conversationId: '', context, messages: [{ id: 'legacy-row' }] });
    expect(element?.props?.conversationId).toBe('conv-from-form');
    expect(Object.prototype.hasOwnProperty.call(element?.props || {}, 'messages')).toBe(false);
  });
});

describe('resolveComposerProps', () => {
  it('projects command-center state from meta/conversation data sources', () => {
    const metaState = {
      agent: 'coder',
      model: 'openai_gpt-5-mini',
      reasoningEffort: 'medium',
      tool: ['system/exec'],
      defaults: {
        agent: 'chatter',
        model: 'openai_gpt-5-mini',
        autoSelectTools: true,
      },
      agentOptions: [{ value: 'coder', label: 'Coder' }],
      modelOptions: [{ value: 'openai_gpt-5-mini', label: 'GPT-5 Mini' }],
      modelInfo: { 'openai_gpt-5-mini': { title: 'GPT-5 Mini' } },
      starterTasks: [{ id: 's1', title: 'Start' }],
    };
    const metaDS = {
      state: metaState,
      peekFormData: () => metaDS.state,
      setFormData: ({ values }) => { metaDS.state = values; },
      setFormField: ({ item, value }) => { metaDS.state = { ...metaDS.state, [item.id]: value }; },
    };
    const convDS = {
      state: { agent: 'coder', model: 'openai_gpt-5-mini' },
      peekFormData: () => convDS.state,
      setFormData: ({ values }) => { convDS.state = values; },
      setFormField: ({ item, value }) => { convDS.state = { ...convDS.state, [item.id]: value }; },
    };
    const context = {
      resources: {},
      Context(name) {
        if (name === 'meta') return { handlers: { dataSource: metaDS } };
        if (name === 'conversations') return { handlers: { dataSource: convDS } };
        return null;
      },
    };

    const state = resolveComposerProps({
      context,
      container: { chat: { commandCenter: true } },
    });

    expect(state.commandCenter).toBe(true);
    expect(state.agentValue).toBe('coder');
    expect(state.modelValue).toBe('openai_gpt-5-mini');
    expect(state.reasoningValue).toBe('medium');
    expect(state.selectedTools).toEqual(['system/exec']);
    expect(state.starterTasks).toEqual([{ id: 's1', title: 'Start' }]);

    state.onReasoningChange('high');
    expect(metaDS.state.reasoningEffort).toBe('high');

    state.onToolsChange(['system/os']);
    expect(metaDS.state.tool).toEqual(['system/os']);

    state.onAutoSelectToolsChange(false);
    expect(metaDS.state.autoSelectTools).toBe(false);
    expect(convDS.state.autoSelectTools).toBe(false);
  });

  it('keeps the current agent/model visible when metadata option arrays are empty', () => {
    const metaState = {
      agent: 'steward',
      model: 'openai_gpt-5-mini',
      defaults: {
        agent: 'steward',
        model: 'openai_gpt-5-mini',
      },
      agentOptions: [],
      modelOptions: [],
      agentInfo: {
        steward: { id: 'steward', name: 'Steward' }
      },
      modelInfo: {
        'openai_gpt-5-mini': { id: 'openai_gpt-5-mini', title: 'GPT-5 Mini' }
      },
      starterTasks: [],
    };
    const metaDS = {
      state: metaState,
      peekFormData: () => metaDS.state,
      setFormData: ({ values }) => { metaDS.state = values; },
      setFormField: ({ item, value }) => { metaDS.state = { ...metaDS.state, [item.id]: value }; },
    };
    const convDS = {
      state: { id: 'conv-1' },
      peekFormData: () => convDS.state,
      setFormData: ({ values }) => { convDS.state = values; },
    };
    const context = {
      resources: {},
      Context(name) {
        if (name === 'meta') return { handlers: { dataSource: metaDS } };
        if (name === 'conversations') return { handlers: { dataSource: convDS } };
        return null;
      },
    };

    const state = resolveComposerProps({
      context,
      container: { chat: { commandCenter: true } },
    });

    expect(state.agentOptions).toEqual(
      expect.arrayContaining([{ value: 'steward', label: 'Steward' }])
    );
    expect(state.modelOptions).toEqual(
      expect.arrayContaining([{ value: 'openai_gpt-5-mini', label: 'GPT-5 Mini' }])
    );
  });
});

describe('fetch handlers', () => {
  it('onFetchMeta normalizes Forge payload-shaped results', async () => {
    const metaDS = {
      state: {},
      setFormData: ({ values }) => { metaDS.state = values; },
    };
    const convDS = {
      state: {},
      peekFormData: () => convDS.state,
      setFormData: ({ values }) => { convDS.state = values; },
    };
    const context = {
      Context(name) {
        if (name === 'meta') return { handlers: { dataSource: metaDS } };
        if (name === 'conversations') return { handlers: { dataSource: convDS } };
        if (name === 'messages') return { handlers: { dataSource: { setCollection: vi.fn(), setError: vi.fn() } } };
        return null;
      },
    };

    await onFetchMeta({
      context,
      payload: {
        appName: 'Agently',
        defaultAgent: 'coder',
        defaultModel: 'openai_gpt-5.4',
        agents: ['coder', 'chatter'],
        models: ['openai_gpt-5.4'],
        capabilities: { agentAutoSelection: true },
      },
    });

    expect(metaDS.state.defaults.agent).toBe('coder');
    expect(Array.isArray(metaDS.state.agentOptions)).toBe(true);
    expect(metaDS.state.agentOptions.length).toBeGreaterThan(0);
    expect(convDS.state.agent).toBe('coder');
    expect(convDS.state.model).toBe('openai_gpt-5.4');
  });

  it('onFetchMeta unwraps singleton collection payloads from datasource extract', async () => {
    const metaDS = {
      state: {},
      setFormData({ values }) {
        this.state = values;
      },
      peekFormData: () => metaDS.state,
    };
    const convDS = {
      state: {},
      setFormData({ values }) {
        this.state = values;
      },
      peekFormData: () => convDS.state,
    };
    const context = {
      Context(name) {
        if (name === 'meta') return { handlers: { dataSource: metaDS } };
        if (name === 'conversations') return { handlers: { dataSource: convDS } };
        return null;
      }
    };
    await onFetchMeta({
      context,
      collection: [{
        workspaceRoot: '/tmp/ws',
        defaults: { agent: 'coder', model: 'openai_gpt-5.4', embedder: 'openai_text' },
        capabilities: { agentAutoSelection: true, compactConversation: true },
        agentInfos: [{ id: 'coder', name: 'Coder', modelRef: 'openai_gpt-5.4' }],
        modelInfos: [{ id: 'openai_gpt-5.4', name: 'GPT-5.4' }],
      }],
    });
    expect(metaDS.state.defaults).toEqual(expect.objectContaining({ agent: 'coder', model: 'openai_gpt-5.4' }));
    expect(metaDS.state.capabilities).toEqual(expect.objectContaining({ agentAutoSelection: true, compactConversation: true }));
    expect(metaDS.state.agentOptions).toEqual(
      expect.arrayContaining([expect.objectContaining({ value: 'coder', label: 'Coder' })])
    );
    expect(metaDS.state.modelOptions).toEqual(
      expect.arrayContaining([expect.objectContaining({ value: 'openai_gpt-5.4' })])
    );
    expect(convDS.state).toEqual(expect.objectContaining({ agent: 'coder', model: 'openai_gpt-5.4', embedder: 'openai_text' }));
  });

  it('onFetchMessages accepts collection-shaped rows from Forge', async () => {
    const syncSpy = vi.spyOn(await import('./chatRuntime'), 'syncMessagesSnapshot').mockResolvedValue([]);
    const fetchPendingSpy = vi.spyOn(await import('./chatRuntime'), 'fetchPendingElicitations').mockResolvedValue([]);
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return { handlers: { dataSource: { peekFormData: () => ({ id: 'conv-1' }) } } };
        }
        return null;
      },
    };

    const turns = [{ id: 'turn-1' }];
    await onFetchMessages({ context, collection: turns });

    expect(fetchPendingSpy).toHaveBeenCalledWith('conv-1');
    expect(syncSpy).toHaveBeenCalledWith(context, turns, 'fetch', []);
    syncSpy.mockRestore();
    fetchPendingSpy.mockRestore();
  });

  it('onFetchQueuedTurns accepts collection-shaped transcript rows', () => {
    const queueDS = { setCollection: vi.fn() };
    const context = {
      Context(name) {
        if (name === 'queueTurns') return { handlers: { dataSource: queueDS } };
        return null;
      },
    };

    const turns = [{ id: 'msg-1', role: 'assistant', content: 'hello' }];
    const queued = onFetchQueuedTurns({ context, collection: turns });

    expect(Array.isArray(queued)).toBe(true);
    expect(queueDS.setCollection).toHaveBeenCalledWith(queued);
  });
});

describe('taskStatusIcon', () => {
  it('maps common statuses to expected icons', () => {
    expect(taskStatusIcon({ status: 'completed' })).toBe('tick');
    expect(taskStatusIcon({ status: 'in_progress' })).toBe('play');
    expect(taskStatusIcon({ status: 'pending' })).toBe('time');
    expect(taskStatusIcon({ status: 'unknown' })).toBe('dot');
  });
});

describe('explorerSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('executes grep with a bounded limit and updates explorer feed data', async () => {
    client.executeTool.mockResolvedValue(JSON.stringify({
      files: [{ Path: 'bitset.go', Matches: 3 }],
    }));

    const searchDS = {
      peekFormData: () => ({ pattern: 'SetBit' }),
      getFormData: () => ({ pattern: 'SetBit' }),
    };
    const resultsDS = {
      peekSelection: () => ({
        selected: {
          path: '/Users/awitas/go/src/github.com/viant/igo/exec',
          rootId: 'local',
        },
      }),
      getSelection: () => ({
        selected: {
          path: '/Users/awitas/go/src/github.com/viant/igo/exec',
          rootId: 'local',
        },
      }),
    };

    const context = {
      Context(name) {
        if (name === 'search') return { handlers: { dataSource: searchDS } };
        if (name === 'results') return { handlers: { dataSource: resultsDS } };
        if (name === 'conversations') return { handlers: { dataSource: { peekFormData: () => ({ id: 'conv-1' }) } } };
        return null;
      },
    };

    const ok = await explorerSearch({ context });

    expect(ok).toBe(true);
    expect(client.executeTool).toHaveBeenCalledWith('resources-grepFiles', expect.objectContaining({
      root: 'file://localhost/',
      path: '/Users/awitas/go/src/github.com/viant/igo/exec',
      pattern: 'SetBit',
      maxFiles: 20,
      maxBlocks: 40,
      lines: 24,
      bytes: 512,
    }));
    expect(updateFeedData).toHaveBeenCalledWith('explorer', expect.objectContaining({
      data: expect.objectContaining({
        input: expect.objectContaining({ pattern: 'SetBit', maxFiles: 20 }),
        output: expect.objectContaining({ files: [{ Path: 'bitset.go', Matches: 3 }] }),
      }),
    }), 'conv-1');
  });

  it('debounces inline input search and forwards the typed pattern', async () => {
    vi.useFakeTimers();
    client.executeTool.mockResolvedValue(JSON.stringify({
      files: [{ Path: 'bitset.go', Matches: 3 }],
    }));

    const searchDS = {
      peekFormData: () => ({ pattern: 'SetBit' }),
      getFormData: () => ({ pattern: 'SetBit' }),
    };
    const resultsDS = {
      peekSelection: () => ({
        selected: {
          path: '/Users/awitas/go/src/github.com/viant/igo/exec',
          rootId: 'local',
        },
      }),
      getSelection: () => ({
        selected: {
          path: '/Users/awitas/go/src/github.com/viant/igo/exec',
          rootId: 'local',
        },
      }),
    };
    const context = {
      Context(name) {
        if (name === 'search') return { handlers: { dataSource: searchDS } };
        if (name === 'results') return { handlers: { dataSource: resultsDS } };
        if (name === 'conversations') return { handlers: { dataSource: { peekFormData: () => ({ id: 'conv-1' }) } } };
        return null;
      },
    };

    explorerSearchInputChanged({ context, value: 'SetBit' });
    await vi.advanceTimersByTimeAsync(260);

    expect(client.executeTool).toHaveBeenCalledWith('resources-grepFiles', expect.objectContaining({
      pattern: 'SetBit',
    }));
    vi.useRealTimers();
  });
});

describe('prepareChangeFiles', () => {
  it('builds a tree for multiple changed files under the same workdir', () => {
    const context = {
      handlers: { dataSource: { dataSourceRef: 'snapshot' } },
      Context(name) {
        if (name === 'snapshot') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ workdir: '/repo' }),
                getFormData: () => ({ workdir: '/repo' }),
              },
            },
          };
        }
        return null;
      },
    };

    const tree = prepareChangeFiles({
      context,
      collection: [
        { url: '/repo/alpha_test.go', diff: 'a', kind: 'create' },
        { url: '/repo/nested/beta_test.go', origUrl: '/repo/nested/beta_test.go', diff: 'b', kind: 'modify' },
      ],
    });

    expect(tree).toHaveLength(2);
    const fileNode = tree.find((entry) => entry.name === 'alpha_test.go');
    const folderNode = tree.find((entry) => entry.name === 'nested');
    expect(fileNode?.name).toBe('alpha_test.go');
    expect(folderNode?.name).toBe('nested');
    expect(folderNode?.childNodes[0].name).toBe('beta_test.go');
    expect(folderNode?.childNodes[0].origUri).toBe('/repo/nested/beta_test.go');
  });

  it('collapses single-child folder chains so the changed leaf stays visible', () => {
    const context = {
      handlers: { dataSource: { dataSourceRef: 'snapshot' } },
      Context(name) {
        if (name === 'snapshot') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ workdir: '/repo' }),
                getFormData: () => ({ workdir: '/repo' }),
              },
            },
          };
        }
        return null;
      },
    };

    const tree = prepareChangeFiles({
      context,
      collection: [
        { url: '/repo/deep/path/to/folder/PROOF_FEED_NOTE.md', diff: 'a', kind: 'modify' },
      ],
    });

    expect(tree).toHaveLength(1);
    expect(tree[0].name).toBe('deep/path/to/folder');
    expect(tree[0].childNodes).toHaveLength(1);
    expect(tree[0].childNodes[0].name).toBe('PROOF_FEED_NOTE.md');
    expect(tree[0].childNodes[0].uri).toBe('/repo/deep/path/to/folder/PROOF_FEED_NOTE.md');
  });
});

describe('onChangedFileSelect', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('loads current and previous content for an updated file diff', async () => {
    client.downloadWorkspaceFile
      .mockResolvedValueOnce('current file body')
      .mockResolvedValueOnce('previous file body');

    const ok = await onChangedFileSelect({
      row: {
        uri: '/repo/second_test.go',
        origUri: '/repo/second_test.go',
        diff: '--- before\n+++ after',
      },
    });

    expect(ok).toBe(true);
    expect(openCodeDiffDialog).toHaveBeenCalledWith(expect.objectContaining({
      currentUri: '/repo/second_test.go',
      prevUri: '/repo/second_test.go',
      hasPrev: true,
    }));
    expect(updateCodeDiffDialog).toHaveBeenCalledWith(expect.objectContaining({
      current: 'current file body',
      prev: 'previous file body',
      diff: '--- before\n+++ after',
      hasPrev: true,
      loading: false,
    }));
  });

  it('supports file-browser item payloads and derives the dialog title from the changed asset path', async () => {
    client.downloadWorkspaceFile
      .mockResolvedValueOnce('current asset body')
      .mockResolvedValueOnce('previous asset body');

    const ok = await onChangedFileSelect({
      item: {
        url: '/repo/nested/beta_test.go',
        origUrl: '/repo/nested/beta_test.go',
        diff: '@@ change @@',
      },
    });

    expect(ok).toBe(true);
    expect(openCodeDiffDialog).toHaveBeenCalledWith(expect.objectContaining({
      title: 'beta_test.go',
      currentUri: '/repo/nested/beta_test.go',
      prevUri: '/repo/nested/beta_test.go',
      hasPrev: true,
      loading: true,
    }));
    expect(updateCodeDiffDialog).toHaveBeenCalledWith(expect.objectContaining({
      current: 'current asset body',
      prev: 'previous asset body',
      diff: '@@ change @@',
      hasPrev: true,
      loading: false,
    }));
  });
});

describe('openResourceFeedPath', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('opens a file-view dialog for resource feed rows using path/rootId', async () => {
    client.executeTool.mockResolvedValueOnce({ content: 'resource body' });

    const ok = await openResourceFeedPath({
      row: {
        path: '/Users/awitas/go/src/github.com/viant/agently-core/recovery.md',
        rootId: 'local',
      },
    });

    expect(ok).toBe(true);
    expect(openFileViewDialog).toHaveBeenCalledWith(expect.objectContaining({
      title: 'recovery.md',
      uri: '/Users/awitas/go/src/github.com/viant/agently-core/recovery.md',
      loading: true,
    }));
    expect(client.executeTool).toHaveBeenCalledWith('resources-read', expect.objectContaining({
      path: '/Users/awitas/go/src/github.com/viant/agently-core/recovery.md',
      rootId: 'local',
      maxBytes: 200000,
    }));
    expect(updateFileViewDialog).toHaveBeenCalledWith(expect.objectContaining({
      title: 'recovery.md',
      uri: '/Users/awitas/go/src/github.com/viant/agently-core/recovery.md',
      loading: false,
      content: 'resource body',
    }));
  });
});
