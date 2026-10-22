import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./stageBus', () => ({ setStage: vi.fn() }));
vi.mock('./agentlyClient', () => ({ client: { streamEvents: vi.fn(), listPendingElicitations: vi.fn().mockResolvedValue([]) } }));
import { client } from './agentlyClient';
import { setStage } from './stageBus';
import * as chatStore from './chatStore';
import { connectStream, handleStreamEvent, installChatStoreMirror, stopPolling } from './chatRuntime';

const conversationID = 'terminal-snapshot-stage';
let context;
let handlers;
let form;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  chatStore.__resetAll();
  installChatStoreMirror(chatStore);
  form = { id: conversationID, running: true };
  context = {
    identity: { windowId: 'chat/terminal-snapshot-stage' },
    resources: { chat: {} },
    Context: name => name === 'conversations' ? { handlers: { dataSource: {
      peekFormData: () => form,
      setFormData: ({ values }) => { form = values; },
    } } } : null,
  };
  client.streamEvents.mockImplementation((_id, callbacks) => { handlers = callbacks; return { close: vi.fn() }; });
  connectStream(context, conversationID);
});

afterEach(() => {
  stopPolling(context);
  installChatStoreMirror(null);
  chatStore.__resetAll();
  vi.clearAllTimers();
  vi.useRealTimers();
});

function event(type, turnId = 'turn-one') {
  handlers.onEvent({ type, conversationId: conversationID, turnId });
}
function snapshot() {
  handlers.onSnapshot({ conversation: { conversationId: conversationID, turns: [{ turnId: 'turn-one', status: 'completed', createdAt: '2026-10-21T17:33:07Z', pages: [] }] } });
}

describe('terminal canonical stage reconciliation', () => {
  it('settles a completed snapshot and rejects later model/tool stages for that exact turn', () => {
    event('turn_started');
    event('model_completed');
    expect(setStage).toHaveBeenLastCalledWith(expect.objectContaining({ text: 'Assistant thinking…' }));
    snapshot();
    expect(setStage).toHaveBeenLastCalledWith({ phase: 'ready', text: 'Ready' });
    expect(form.running).toBe(false);
    expect(context.resources.chat.lastHasRunning).toBe(false);
    expect(context.resources.chat.runningTurnId).toBe('');
    setStage.mockClear();
    for (const type of ['model_started', 'model_completed', 'tool_call_completed', 'narration', 'assistant']) event(type);
    expect(setStage).not.toHaveBeenCalled();
    expect(chatStore.getState(conversationID).turns[0].lifecycle).toBe('completed');
    event('turn_started', 'turn-new');
    expect(setStage).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'executing' }));
    expect(context.resources.chat.lastHasRunning).toBe(true);
  });

  it('keeps a newer turn executing when an older terminal snapshot arrives', () => {
    event('turn_started', 'turn-two');
    setStage.mockClear();
    snapshot();
    expect(setStage).not.toHaveBeenCalled();
    expect(form.running).toBe(true);
    expect(context.resources.chat.runningTurnId).toBe('turn-two');
  });

  it('projects late final assistant content and tool results without reopening progress', () => {
    snapshot();
    setStage.mockClear();
    handlers.onEvent({
      type: 'assistant', conversationId: conversationID, turnId: 'turn-one',
      messageId: 'final-message', pageId: 'final-page', patch: { role: 'assistant' },
      content: 'Completed report body delivered after the terminal snapshot',
    });
    handlers.onEvent({
      type: 'tool_call_completed', conversationId: conversationID, turnId: 'turn-one',
      assistantMessageId: 'final-message', pageId: 'final-page', toolCallId: 'late-result',
      status: 'completed', responsePayload: { rows: [{ count: 42 }] },
    });
    const turn = chatStore.getState(conversationID).turns[0];
    expect(turn.lifecycle).toBe('completed');
    expect(turn.pages.find(page => page.pageId === 'final-page')?.content)
      .toBe('Completed report body delivered after the terminal snapshot');
    expect(turn.pages.flatMap(page => page.toolCalls).find(tool => tool.toolCallId === 'late-result'))
      .toMatchObject({ status: 'completed', responsePayload: { rows: [{ count: 42 }] } });
    expect(setStage).not.toHaveBeenCalled();
    expect(context.resources.chat.lastHasRunning).toBe(false);
    expect(form.running).toBe(false);
  });

  it('fences late execution events using a previously hydrated terminal canonical turn', () => {
    chatStore.onTranscript(conversationID, { conversationId: conversationID, turns: [{ turnId: 'turn-one', status: 'completed', pages: [] }] });
    handleStreamEvent(context.resources.chat, context, conversationID, { type: 'model_completed', conversationId: conversationID, turnId: 'turn-one' });
    expect(setStage).not.toHaveBeenCalled();
    expect(chatStore.getState(conversationID).turns[0].lifecycle).toBe('completed');
  });

  it('does not publish a hidden conversation snapshot into the visible status bar', () => {
    form.id = 'another-visible-conversation';
    snapshot();
    expect(setStage).not.toHaveBeenCalled();
  });
});
