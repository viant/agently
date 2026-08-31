const EXPLICIT_TARGETS = new Set(['inline', 'workspace', 'detached']);

export function normalizeToolFeedTarget(value = '') {
  const normalized = String(value || '').trim().toLowerCase();
  if (!normalized || normalized === 'auto') return 'auto';
  return EXPLICIT_TARGETS.has(normalized) ? normalized : 'auto';
}

export function toolFeedTargetsPlacement(feed = null, placement = 'workspace', includeAuto = true) {
  const target = normalizeToolFeedTarget(feed?.presentation?.target);
  const normalizedPlacement = normalizeToolFeedTarget(placement);
  if (target === 'auto') return includeAuto === true;
  return target === normalizedPlacement;
}
