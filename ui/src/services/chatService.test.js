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
  explorerSearch,
  explorerSearchInputChanged,
  taskStatusIcon,
  prepareChangeFiles,
  onChangedFileSelect,
} from './chatService';
import {
  openCodeDiffDialog,
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
});
