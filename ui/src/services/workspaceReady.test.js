import { describe, it, expect, vi } from 'vitest';
import { signal } from '@preact/signals-react';
import { waitForWorkspaceReady } from '../../../../forge/src/core/ui/workspaceReady.js';
const entry = (state) => ({ windowId: 'resource-1', workspaceObject: { objectId: 'workspace:resource-1', lifecycle: { state } } });
describe('workspace renderer acknowledgment', () => {
  it('does not resolve before readiness, then returns the canonical object', async () => {
    const windows = signal([entry('opening')]);
    const resolved = vi.fn();
    const pending = waitForWorkspaceReady(windows, 'resource-1').then(resolved);
    await Promise.resolve();
    expect(resolved).not.toHaveBeenCalled();
    windows.value = [entry('ready')];
    await pending;
    expect(resolved).toHaveBeenCalledWith(entry('ready').workspaceObject);
  });
  it('rejects permission/metadata failures without claiming success', async () => {
    const windows = signal([entry('opening')]);
    const pending = waitForWorkspaceReady(windows, 'resource-1');
    windows.value = [entry('failed')];
    await expect(pending).rejects.toThrow('could not be opened');
  });
  it('rejects a closed or timed out open', async () => {
    const windows = signal([entry('opening')]);
    const pending = waitForWorkspaceReady(windows, 'resource-1');
    windows.value = [];
    await expect(pending).rejects.toThrow('closed before');
    await expect(waitForWorkspaceReady(signal([entry('opening')]), 'resource-1', 1)).rejects.toThrow('timed out');
  });
});
