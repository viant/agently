import { describe, it, expect } from 'vitest';
import { readWorkspaceSession, updateWorkspaceSession, mergeWorkspaceSessionWindows, workspaceHistory } from './workspaceSession.js';
const storage = () => {
  const values = new Map();
  return { getItem: (key) => values.get(key), setItem: (key, value) => values.set(key, value) };
};
const entry = (id, revision = 1, turnId = 'turn-1') => ({ windowId: id, conversationId: 'conversation-1',
  workspaceObject: { objectId: `workspace:${id}`, revision, origin: { turnId }, lifecycle: { state: 'ready' } } });
describe('conversation-owned workspace session', () => {
  it('retains all historical objects and original ownership across show, close, and reload', () => {
    const local = storage();
    updateWorkspaceSession(local, 'conversation-1', (state) => mergeWorkspaceSessionWindows(state, [entry('one'), entry('two')]));
    updateWorkspaceSession(local, 'conversation-1', (state) => ({ ...mergeWorkspaceSessionWindows(state, [entry('one', 2, 'later-turn')]), closedWindowIds: ['one'] }));
    const history = workspaceHistory(readWorkspaceSession(local, 'conversation-1'));
    expect(history).toHaveLength(2);
    expect(history[0].workspaceObject.origin.turnId).toBe('turn-1');
    expect(history[0].workspaceObject.lifecycle.state).toBe('closed');
    expect(readWorkspaceSession(local, 'another').windows).toEqual([]);
  });
  it('ignores stale revisions and foreign conversations', () => {
    const initial = readWorkspaceSession(storage(), 'conversation-1');
    const current = mergeWorkspaceSessionWindows(initial, [entry('one', 4)]);
    const merged = mergeWorkspaceSessionWindows(current, [entry('one', 2), { ...entry('foreign'), conversationId: 'other' }]);
    expect(merged.windows).toHaveLength(1);
    expect(merged.windows[0].workspaceObject.revision).toBe(4);
  });
  it('migrates legacy layout and window state into a single record', () => {
    const local = storage();
    local.setItem('agently.workspaceState:conversation-1', JSON.stringify(entry('one')));
    local.setItem('agently.workspacePresentationMode:conversation-1', 'full');
    const migrated = updateWorkspaceSession(local, 'conversation-1', (state) => ({ ...state, activeSurface: 'workspace' }));
    expect(migrated.workspaceMode).toBe('focus');
    expect(readWorkspaceSession(local, 'conversation-1')).toEqual(migrated);
  });
});
