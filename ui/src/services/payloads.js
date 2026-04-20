const PAYLOAD_PREVIEW_LIMIT = 32768;

export function resolvePayload(payload = null) {
  if (!payload || typeof payload !== 'object') return null;
  const id = String(payload?.id ?? payload?.Id ?? '').trim();
  const compression = String(payload?.compression ?? payload?.Compression ?? 'none').toLowerCase();
  const inlineBody = payload?.inlineBody ?? payload?.InlineBody;
  if (compression && compression !== 'none') {
    return {
      id,
      compression,
      note: 'payload is compressed in transcript; use payload id to inspect raw body'
    };
  }
  if (typeof inlineBody === 'string' && inlineBody.trim() !== '') {
    try {
      return JSON.parse(inlineBody);
    } catch (_) {
      const preview = inlineBody.length > PAYLOAD_PREVIEW_LIMIT
        ? `${inlineBody.slice(0, PAYLOAD_PREVIEW_LIMIT)}\n...[truncated]`
        : inlineBody;
      return {
        id,
        compression: compression || 'none',
        inlineBody: preview
      };
    }
  }
  return {
    id,
    compression: compression || 'none'
  };
}
