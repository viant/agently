import { beforeEach, describe, expect, it, vi } from 'vitest';

const { executeTool } = vi.hoisted(() => ({ executeTool: vi.fn() }));
vi.mock('./agentlyClient', () => ({ client: { executeTool } }));

import { emitReportUIEvent } from './reportEventService';

describe('reportEventService', () => {
  beforeEach(() => executeTool.mockReset());

  it('does not publish conversation telemetry for a standalone menu report', async () => {
    await expect(emitReportUIEvent({kind: 'report.context_updated', windowId: 'menu-spo'})).resolves.toEqual({recorded: false});
    expect(executeTool).not.toHaveBeenCalled();
  });

  it('omits other report telemetry when the window has no conversation', async () => {
    await expect(emitReportUIEvent({kind: 'report.export_complete'})).resolves.toEqual({recorded: false});
    expect(executeTool).not.toHaveBeenCalled();
  });

  it('records a scoped report UI event', async () => {
    executeTool.mockResolvedValue({ recorded: true });
    await emitReportUIEvent({
      kind: 'report.export_complete',
      windowId: 'window-1',
      conversationId: 'conversation-1',
      detail: { reportName: 'Inventory Brief', artifactId: 'artifact-1' },
    });
    expect(executeTool).toHaveBeenCalledWith('ui/events:record', {
      kind: 'report.export_complete',
      windowId: 'window-1',
      detail: { reportName: 'Inventory Brief', artifactId: 'artifact-1' },
    }, { conversationId: 'conversation-1' });
  });

  it('records an unkeyed report context update at conversation scope', async () => {
    executeTool.mockResolvedValue({ recorded: true });

    await emitReportUIEvent({
      kind: 'report.context_updated',
      conversationId: 'conversation-1',
      windowId: 'stale-local-window',
      detail: { reportId: 'report-1' },
    });

    expect(executeTool).toHaveBeenCalledWith('ui/events:record', {
      kind: 'report.context_updated',
      detail: { reportId: 'report-1' },
    }, { conversationId: 'conversation-1' });
  });
});
