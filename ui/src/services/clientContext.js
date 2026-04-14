export function detectWebFormFactor() {
  if (typeof window === 'undefined') return 'desktop';
  const width = Number(window.innerWidth || 0);
  if (width > 0 && width < 768) return 'phone';
  if (width > 0 && width < 1100) return 'tablet';
  return 'desktop';
}

export function buildWebTargetContext() {
  return {
    platform: 'web',
    formFactor: detectWebFormFactor(),
    surface: 'browser',
    capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
  };
}

export function buildWebClientContext() {
  return {
    kind: 'web',
    ...buildWebTargetContext(),
  };
}

export function buildWebQueryContext() {
  return {
    client: buildWebClientContext(),
  };
}
