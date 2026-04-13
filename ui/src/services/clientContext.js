export function detectWebFormFactor() {
  if (typeof window === 'undefined') return 'desktop';
  const width = Number(window.innerWidth || 0);
  if (width > 0 && width < 768) return 'phone';
  if (width > 0 && width < 1100) return 'tablet';
  return 'desktop';
}

export function buildWebClientContext() {
  const formFactor = detectWebFormFactor();
  return {
    kind: 'web',
    platform: 'web',
    formFactor,
    surface: 'browser',
    capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
  };
}

export function buildWebQueryContext() {
  return {
    client: buildWebClientContext(),
  };
}
