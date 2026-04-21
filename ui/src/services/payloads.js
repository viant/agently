const PAYLOAD_PREVIEW_LIMIT = 32768;

function normalizeInlineBytes(value = '') {
  if (typeof value !== 'string') return new Uint8Array();
  const bytes = new Uint8Array(value.length);
  for (let i = 0; i < value.length; i += 1) {
    bytes[i] = value.charCodeAt(i) & 0xff;
  }
  return bytes;
}

function limitPreview(text = '') {
  return text.length > PAYLOAD_PREVIEW_LIMIT
    ? `${text.slice(0, PAYLOAD_PREVIEW_LIMIT)}\n...[truncated]`
    : text;
}

function decodeMaybeGzip(inlineBody = '', compression = '') {
  const mode = String(compression || '').trim().toLowerCase();
  if (mode !== 'gzip' || typeof DecompressionStream === 'undefined') {
    return null;
  }
  try {
    const input = normalizeInlineBytes(inlineBody);
    const stream = new Response(new Blob([input]).stream().pipeThrough(new DecompressionStream('gzip')));
    return stream.text();
  } catch (_) {
    return null;
  }
}

export function resolvePayload(payload = null) {
  if (!payload || typeof payload !== 'object') return null;
  const id = String(payload?.id ?? payload?.Id ?? '').trim();
  const compression = String(payload?.compression ?? payload?.Compression ?? 'none').toLowerCase();
  const inlineBody = payload?.inlineBody ?? payload?.InlineBody;
  if (compression && compression !== 'none') {
    if (typeof inlineBody === 'string' && inlineBody.length) {
      const decodedPromise = decodeMaybeGzip(inlineBody, compression);
      if (decodedPromise && typeof decodedPromise.then === 'function') {
        return {
          id,
          compression,
          inlineBody,
          decodedInlineBodyPromise: decodedPromise.then((text) => {
            try {
              return JSON.parse(text);
            } catch (_) {
              return {
                id,
                compression: 'none',
                inlineBody: limitPreview(text),
              };
            }
          }).catch(() => ({
            id,
            compression,
            note: 'payload is compressed in transcript; use payload id to inspect raw body'
          }))
        };
      }
    }
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
      return {
        id,
        compression: compression || 'none',
        inlineBody: limitPreview(inlineBody)
      };
    }
  }
  return {
    id,
    compression: compression || 'none'
  };
}
