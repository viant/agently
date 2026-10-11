import { beforeEach, expect, it, vi } from 'vitest';
const control = vi.hoisted(() => ({ creates: [], subscribers: [], snapshots: vi.fn() }));
vi.mock('./agentlyClient', () => ({ client: {} }));
vi.mock('agently-core-ui-sdk', () => ({
  AgUiRunError: class extends Error {},
  AgUiRemoteConversationTransport: class {
    create(id) { return new Promise((resolve, reject) => control.creates.push({ id, resolve, reject })); }
    subscribe(id, threadId, handlers) { const close = vi.fn(); control.subscribers.push({ id, threadId, handlers, close }); return { close }; }
    getSnapshot(id, threadId) { return control.snapshots(id, threadId); }
    reset() {}
  },
}));
import { createRemoteViewService } from './aguiRemoteView';
beforeEach(() => { control.creates = []; control.subscribers = []; control.snapshots.mockReset(); });

it('ignores a previous account creation failure after a new account thread is ready', async () => {
  const service = createRemoteViewService({});
  const old = service.ensure('provider');
  const rejection = expect(old).rejects.toThrow('old account unavailable');
  service.reset();
  const next = service.ensure('provider');
  control.creates[1].resolve({ threadId: 'new-account-thread' });
  await next;
  control.creates[0].reject(new Error('old account unavailable'));
  await rejection;
  expect(service.getSnapshot('provider')).toMatchObject({ phase: 'idle', error: null, thread: { threadId: 'new-account-thread' } });
});

it('ignores already queued snapshots and descriptors after reset and after restoration', async () => {
  const service = createRemoteViewService({});
  const initial = service.ensure('provider');
  control.creates[0].resolve({ threadId: 'old-thread' }); await initial;
  const old = control.subscribers[0];
  const stale = { phase: 'completed', messages: [{ id: 'old-secret', role: 'assistant', content: 'old account output' }] };
  service.reset();
  expect(old.close).toHaveBeenCalledOnce();
  expect(() => old.handlers.onSession(stale)).not.toThrow();
  old.handlers.onDescriptor({ kind: 'protocol-snapshot' });
  expect(control.snapshots).not.toHaveBeenCalled();
  expect(service.getSnapshot('provider').rows).toEqual([]);
  const restored = service.ensure('provider');
  control.creates[1].resolve({ threadId: 'restored-thread' }); await restored;
  old.handlers.onSession(stale);
  expect(service.getSnapshot('provider').rows).toEqual([]);
  control.subscribers[1].handlers.onSession({ phase: 'completed', messages: [{ id: 'current', role: 'assistant', content: 'current account output' }] });
  expect(service.getSnapshot('provider').rows.map(row => row.content)).toEqual(['current account output']);
});
