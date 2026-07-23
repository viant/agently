import { beforeEach, describe, expect, it, vi } from 'vitest';

const { executeTool } = vi.hoisted(() => ({ executeTool: vi.fn() }));
vi.mock('./agentlyClient', () => ({ client: { executeTool } }));

import { emitReportUIEvent } from './reportEventService';

describe('reportEventService', () => {
  beforeEach(() => executeTool.mockReset());

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
});
