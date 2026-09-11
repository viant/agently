const warnedLegacyTargets = new Set();
const EXPLICIT_TARGETS = new Set(['inline', 'rail', 'overlay']);

export function normalizeToolFeedTarget(value = '') {
  const normalized = String(value || '').trim().toLowerCase();
  if (!normalized || normalized === 'auto') return 'auto';
  if (normalized === 'workspace' || normalized === 'detached') {
    const canonical = normalized === 'workspace' ? 'rail' : 'overlay';
    if (import.meta.env?.DEV && !warnedLegacyTargets.has(normalized)) {
      warnedLegacyTargets.add(normalized);
      console.warn(`Legacy tool-feed target "${normalized}"; use "${canonical}".`);
    }
    return canonical;
  }
  return EXPLICIT_TARGETS.has(normalized) ? normalized : 'auto';
}

export function toolFeedTargetsPlacement(feed = null, placement = 'rail', includeAuto = true) {
  if (feed?.presentation?.workspaceObjectId) return false;
  const target = normalizeToolFeedTarget(feed?.presentation?.target);
  const normalizedPlacement = normalizeToolFeedTarget(placement);
  if (target === 'auto') return includeAuto === true;
  return target === normalizedPlacement;
}
