import { describe, expect, it, vi } from 'vitest';

import {
  cancelQueuedTurn,
  moveQueuedTurn,
  editQueuedTurn,
  steerQueuedTurn,
} from '../../../../forge/src/components/chatQueueActions.js';

describe('chatQueueActions', () => {
  it('cancels and moves queued turns through chatService helpers', () => {
    const chatService = {
      cancelQueuedTurnByID: vi.fn(),
      moveQueuedTurn: vi.fn(),
    };
    const context = {};

    cancelQueuedTurn({
      chatService,
      context,
      conversationID: 'conv-1',
      turn: { id: 'turn-1' },
    });
    moveQueuedTurn({
      chatService,
      context,
      conversationID: 'conv-1',
      turn: { id: 'turn-1' },
      direction: 'up',
    });

    expect(chatService.cancelQueuedTurnByID).toHaveBeenCalledWith({
      context,
      conversationID: 'conv-1',
      turnID: 'turn-1',
    });
    expect(chatService.moveQueuedTurn).toHaveBeenCalledWith({
      context,
      conversationID: 'conv-1',
      turnID: 'turn-1',
      direction: 'up',
    });
  });

  it('edits queued turns only when prompt returns changed content', () => {
    const chatService = {
      editQueuedTurn: vi.fn(),
    };
    const context = {};
    const promptFn = vi.fn(() => 'Updated queued request');

    editQueuedTurn({
      chatService,
      context,
      conversationID: 'conv-1',
      turn: { id: 'turn-1', preview: 'Original queued request' },
      promptFn,
    });

    expect(promptFn).toHaveBeenCalledWith('Edit queued request', 'Original queued request');
    expect(chatService.editQueuedTurn).toHaveBeenCalledWith({
      context,
      conversationID: 'conv-1',
      turnID: 'turn-1',
      content: 'Updated queued request',
    });
  });

  it('prefers force-steer when available and otherwise falls back to steerTurn', () => {
    const forceService = {
      forceSteerQueuedTurn: vi.fn(),
    };
    steerQueuedTurn({
      chatService: forceService,
      context: {},
      conversationID: 'conv-1',
      turn: { id: 'turn-1', preview: 'Queued follow-up' },
      runningTurnId: 'running-1',
    });
    expect(forceService.forceSteerQueuedTurn).toHaveBeenCalledWith({
      context: {},
      conversationID: 'conv-1',
      turnID: 'turn-1',
    });

    const fallbackService = {
      steerTurn: vi.fn(),
    };
    const context = {};
    steerQueuedTurn({
      chatService: fallbackService,
      context,
      conversationID: 'conv-1',
      turn: { id: 'turn-1', preview: 'Queued follow-up' },
      runningTurnId: 'running-1',
    });
    expect(fallbackService.steerTurn).toHaveBeenCalledWith({
      context,
      conversationID: 'conv-1',
      turnID: 'running-1',
      content: 'Queued follow-up',
    });
  });
});
