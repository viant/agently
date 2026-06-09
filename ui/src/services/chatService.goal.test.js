import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./agentlyClient', () => ({
  client: {
    getGoal: vi.fn(),
    createGoal: vi.fn(),
    updateGoal: vi.fn(),
    clearGoal: vi.fn(),
    query: vi.fn(),
    getTranscript: vi.fn(),
  },
}));

vi.mock('./chatStore', () => ({
  onTranscript: vi.fn(),
  reset: vi.fn(),
  submit: vi.fn(),
  steer: vi.fn(),
}));

vi.mock('./chatRuntime', () => ({
  applyIterationVisibility: vi.fn(),
  bindConversationWindowEvents: vi.fn(),
  bootstrapConversationSelection: vi.fn(),
  cacheSettledConversationBootstrapSnapshot: vi.fn(),
  clearPendingConversationBootstrap: vi.fn(),
  createNewConversation: vi.fn(),
  dsTick: vi.fn(),
  disconnectStream: vi.fn(),
  ensureContextResources: vi.fn(),
  ensureConversation: vi.fn(),
  clearSettledConversationBootstrapSnapshot: vi.fn(),
  fetchConversation: vi.fn(),
  fetchPendingElicitations: vi.fn(),
  getSettledConversationBootstrapSnapshot: vi.fn(() => null),
  getVisibleIterations: vi.fn(),
  hasPendingConversationBootstrap: vi.fn(() => false),
  hydrateMeta: vi.fn(),
  hydrateConversationFromBootstrapSnapshot: vi.fn(() => false),
  isConversationLiveish: vi.fn(),
  logExecutorDebug: vi.fn(),
  logStreamDebug: vi.fn(),
  markPendingConversationBootstrap: vi.fn(),
  mapTranscriptToRows: vi.fn(() => ({ queuedTurns: [] })),
  normalizeMetaResponse: vi.fn(),
  publishActiveConversation: vi.fn(),
  publishConversationMetaUpdated: vi.fn(),
  refreshGoalFeed: vi.fn(),
  renderMergedRowsForContext: vi.fn(),
  rememberSeedTitle: vi.fn(),
  resolveUserID: vi.fn(),
  sanitizeAutoSelection: vi.fn((value) => value),
  syncConversationTransport: vi.fn(),
  startPolling: vi.fn(),
  stopPolling: vi.fn(),
  syncMessagesSnapshot: vi.fn(),
  unbindConversationWindowEvents: vi.fn(),
}));

vi.mock('./stageBus', () => ({
  setStage: vi.fn(),
}));

vi.mock('./httpClient', () => ({
  showToast: vi.fn(),
}));

vi.mock('../components/lookups/client.js', () => ({
  listLookupRegistry: vi.fn(),
}));

vi.mock('./toolFeedBus', () => ({
  getFeedData: vi.fn(),
  updateFeedData: vi.fn(),
}));

vi.mock('../utils/dialogBus', () => ({
  openCodeDiffDialog: vi.fn(),
  openFileViewDialog: vi.fn(),
  updateCodeDiffDialog: vi.fn(),
  updateFileViewDialog: vi.fn(),
}));

import { client } from './agentlyClient';
import { showToast } from './httpClient';
import { submit as submitToChatStore } from './chatStore';
import { dsTick, ensureConversation, refreshGoalFeed } from './chatRuntime';
import {
  clearConversationGoal,
  clearGoalFeed,
  handleGoalCommand,
  parseGoalCommand,
  pauseConversationGoal,
  pauseGoalFeed,
  resumeConversationGoal,
  resumeGoalFeed,
  saveGoalFeedForm,
  setConversationGoal,
  submitMessage,
} from './chatService';

function makeContext(conversationID) {
  return {
    Context(name) {
      if (name === 'conversations') {
        return {
          handlers: {
            dataSource: {
              peekFormData: () => (conversationID ? { id: conversationID } : {}),
              setFormData: vi.fn(),
            },
          },
        };
      }
      if (name === 'meta') {
        return {
          handlers: {
            dataSource: {
              peekFormData: () => ({
                capabilities: { goals: true },
              }),
            },
          },
        };
      }
      return null;
    },
  };
}

function makeContextWithoutGoals(conversationID) {
  return {
    Context(name) {
      if (name === 'conversations') {
        return {
          handlers: {
            dataSource: {
              peekFormData: () => (conversationID ? { id: conversationID } : {}),
              setFormData: vi.fn(),
            },
          },
        };
      }
      if (name === 'meta') {
        return {
          handlers: {
            dataSource: {
              peekFormData: () => ({
                capabilities: { goals: false },
              }),
            },
          },
        };
      }
      return null;
    },
  };
}

describe('parseGoalCommand', () => {
  it('returns null for non-goal text', () => {
    expect(parseGoalCommand('hello world')).toBeNull();
    expect(parseGoalCommand('/goalkeeper stats')).toBeNull();
  });

  it('parses bare /goal as show', () => {
    expect(parseGoalCommand('/goal')).toEqual({ action: 'show' });
    expect(parseGoalCommand('/goal show')).toEqual({ action: 'show' });
    expect(parseGoalCommand('/goal status')).toEqual({ action: 'show' });
  });

  it('parses lifecycle verbs', () => {
    expect(parseGoalCommand('/goal pause')).toEqual({ action: 'pause' });
    expect(parseGoalCommand('/goal resume')).toEqual({ action: 'resume' });
    expect(parseGoalCommand('/goal clear')).toEqual({ action: 'clear' });
    expect(parseGoalCommand('/goal help')).toEqual({ action: 'help' });
  });

  it('parses set with explicit verb and as free text', () => {
    expect(parseGoalCommand('/goal set ship the release')).toEqual({ action: 'set', objective: 'ship the release' });
    expect(parseGoalCommand('/goal ship the release')).toEqual({ action: 'set', objective: 'ship the release' });
  });

  it('is case insensitive on the command keyword', () => {
    expect(parseGoalCommand('/GOAL Pause')).toEqual({ action: 'pause' });
  });
});

describe('handleGoalCommand', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('warns and does not call the API without a conversation', async () => {
    const handled = await handleGoalCommand({ context: makeContext(''), command: { action: 'show' } });
    expect(handled).toBe(true);
    expect(client.getGoal).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalled();
  });

  it('shows the current goal', async () => {
    client.getGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    await handleGoalCommand({ context: makeContext('conv-1'), command: { action: 'show' } });
    expect(client.getGoal).toHaveBeenCalledWith('conv-1');
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('ship'), expect.anything());
  });

  it('creates a goal on set', async () => {
    client.createGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    await handleGoalCommand({ context: makeContext('conv-1'), command: { action: 'set', objective: 'ship' } });
    expect(client.createGoal).toHaveBeenCalledWith('conv-1', { objective: 'ship' });
  });

  it('falls back to update when a goal already exists', async () => {
    client.createGoal.mockRejectedValue(new Error('goal already exists for current conversation'));
    client.updateGoal.mockResolvedValue({ objective: 'ship v2', status: 'active' });
    await handleGoalCommand({ context: makeContext('conv-1'), command: { action: 'set', objective: 'ship v2' } });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-1', { objective: 'ship v2' });
  });

  it('maps pause and resume to status updates', async () => {
    client.updateGoal.mockResolvedValue({ objective: 'ship', status: 'paused' });
    await handleGoalCommand({ context: makeContext('conv-1'), command: { action: 'pause' } });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-1', { status: 'paused' });

    client.updateGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    await handleGoalCommand({ context: makeContext('conv-1'), command: { action: 'resume' } });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-1', { status: 'active' });
  });

  it('clears the goal', async () => {
    client.clearGoal.mockResolvedValue(undefined);
    await handleGoalCommand({ context: makeContext('conv-1'), command: { action: 'clear' } });
    expect(client.clearGoal).toHaveBeenCalledWith('conv-1');
  });
});

describe('goal helper actions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('setConversationGoal refreshes goal state after create', async () => {
    client.createGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    const goal = await setConversationGoal({ context: makeContext('conv-1'), objective: 'ship' });
    expect(goal).toEqual({ objective: 'ship', status: 'active' });
    expect(client.createGoal).toHaveBeenCalledWith('conv-1', { objective: 'ship' });
    expect(dsTick).toHaveBeenCalled();
  });

  it('pause/resume/clear helpers use existing goal routes', async () => {
    client.updateGoal.mockResolvedValue({ objective: 'ship', status: 'paused' });
    await pauseConversationGoal({ context: makeContext('conv-1') });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-1', { status: 'paused' });

    client.updateGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    await resumeConversationGoal({ context: makeContext('conv-1') });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-1', { status: 'active' });

    client.clearGoal.mockResolvedValue(undefined);
    await clearConversationGoal({ context: makeContext('conv-1') });
    expect(client.clearGoal).toHaveBeenCalledWith('conv-1');
  });

  it('feed-native goal handlers use the generic feed conversation context', async () => {
    client.createGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    await saveGoalFeedForm({
      context: {
        identity: { conversationId: 'conv-feed-1' },
        handlers: {
          dataSource: {
            getFormData: () => ({ objective: 'ship' }),
          },
        },
      },
    });
    expect(client.createGoal).toHaveBeenCalledWith('conv-feed-1', { objective: 'ship' });
    expect(refreshGoalFeed).toHaveBeenCalledWith('conv-feed-1');

    client.updateGoal.mockResolvedValue({ objective: 'ship', status: 'paused' });
    await pauseGoalFeed({ context: { identity: { conversationId: 'conv-feed-1' } } });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-feed-1', { status: 'paused' });

    client.updateGoal.mockResolvedValue({ objective: 'ship', status: 'active' });
    await resumeGoalFeed({ context: { identity: { conversationId: 'conv-feed-1' } } });
    expect(client.updateGoal).toHaveBeenCalledWith('conv-feed-1', { status: 'active' });

    client.clearGoal.mockResolvedValue(undefined);
    await clearGoalFeed({ context: { identity: { conversationId: 'conv-feed-1' } } });
    expect(client.clearGoal).toHaveBeenCalledWith('conv-feed-1');
  });
});

describe('submitMessage goal routing', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('routes a /goal line to the goal API and not to the agent', async () => {
    client.clearGoal.mockResolvedValue(undefined);
    await submitMessage({ context: makeContext('conv-1'), message: '/goal clear' });
    expect(client.clearGoal).toHaveBeenCalledWith('conv-1');
    expect(submitToChatStore).not.toHaveBeenCalled();
    expect(ensureConversation).not.toHaveBeenCalled();
    expect(client.query).not.toHaveBeenCalled();
  });

  it('blocks /goal when the workspace capability is disabled', async () => {
    await submitMessage({ context: makeContextWithoutGoals('conv-1'), message: '/goal clear' });
    expect(client.clearGoal).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith('Goals are not enabled in this workspace.', expect.anything());
  });
});
