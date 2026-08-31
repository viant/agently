import { getReportExportArtifact } from './reportExportService';
import { executeReportingTool } from './reportingToolClient';

function safeId(value = '') {
  return String(value || '').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '') || 'feed';
}

function rowsFor(value) {
  if (Array.isArray(value)) return value;
  if (value && typeof value === 'object') return [value];
  return [];
}

export function buildFeedReportRequest({ feedId = '', conversationId = '', title = '', dataMap = {}, target = {} } = {}) {
  const refs = Object.keys(dataMap || {}).sort();
  return {
    viewRef: `feed://${String(feedId || '').trim()}`,
    reportId: `feed_${safeId(feedId)}`,
    title: title || 'Tool feed',
    format: 'pdf',
    conversationId,
    dataSourceRefs: refs,
    dataSourceOverrides: Object.fromEntries(refs.map((ref) => [ref, { collection: rowsFor(dataMap[ref]) }])),
    target: {
      platform: 'web',
      formFactor: 'desktop',
      surface: 'browser',
      ...target,
    },
  };
}

export async function exportFeedReportPDF({ feedId = '', conversationId = '', title = '', dataMap = {}, target = {} } = {}) {
  const request = buildFeedReportRequest({ feedId, conversationId, title, dataMap, target });
  const result = await executeReportingTool(
    'reporting:compile_and_export_forge_ui',
    request,
    'Unable to export Forge UI as PDF.',
  );
  const job = result?.job || {};
  const artifactId = job.artifactId || result?.artifact?.artifactId;
  if (String(job.status || '').toLowerCase() !== 'succeeded' || !artifactId) {
    throw new Error(job.error || 'PDF export failed.');
  }
  const artifact = await getReportExportArtifact({ artifactId, conversationId });
  if (!(artifact?.bytes instanceof Uint8Array) || artifact.bytes.length === 0) {
    throw new Error('PDF export returned no data.');
  }
  const url = URL.createObjectURL(new Blob([artifact.bytes], { type: 'application/pdf' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = `${safeId(title || feedId || 'tool-feed')}.pdf`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  return { ...job, artifact };
}
