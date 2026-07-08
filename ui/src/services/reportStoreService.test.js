import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./reportingToolClient', () => ({
  executeReportingTool: vi.fn(async (name, args) => ({
    tool: name,
    args,
  })),
}));

import { executeReportingTool } from './reportingToolClient';
import { getReport, listReports, saveReport, updateReport } from './reportStoreService';

describe('reportStoreService', () => {
  beforeEach(() => {
    executeReportingTool.mockClear();
  });

  it('routes save_report through the reporting tool client', async () => {
    const request = {
      reportId: 'forecastingQ3',
      title: 'Forecasting Q3',
      reportDocument: { kind: 'reportDocument', id: 'forecastingQ3' },
    };

    const result = await saveReport(request);

    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:save_report',
      request,
      'report save request failed',
    );
    expect(result).toMatchObject({ ok: true, tool: 'reporting:save_report' });
  });

  it('routes get_report through the reporting tool client', async () => {
    executeReportingTool.mockResolvedValueOnce({
      artifactId: 'report-1',
      reportId: 'forecastingQ3',
    });

    const result = await getReport({ artifactId: 'report-1' });

    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:get_report',
      { artifactId: 'report-1' },
      'report get request failed',
    );
    expect(result).toMatchObject({ artifactId: 'report-1', reportId: 'forecastingQ3' });
  });

  it('routes list_reports through the reporting tool client', async () => {
    executeReportingTool.mockResolvedValueOnce({
      reports: [{ artifactId: 'report-1' }],
      totalCount: 1,
    });

    const result = await listReports({ limit: 10 });

    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:list_reports',
      { limit: 10 },
      'report list request failed',
    );
    expect(result).toMatchObject({ totalCount: 1 });
  });

  it('routes update_report through the reporting tool client', async () => {
    executeReportingTool.mockResolvedValueOnce({
      artifactId: 'report-1',
      title: 'Forecasting Q3 Updated',
    });

    const result = await updateReport({
      artifactId: 'report-1',
      title: 'Forecasting Q3 Updated',
    });

    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:update_report',
      { artifactId: 'report-1', title: 'Forecasting Q3 Updated' },
      'report update request failed',
    );
    expect(result).toMatchObject({ ok: true, artifactId: 'report-1' });
  });
});
