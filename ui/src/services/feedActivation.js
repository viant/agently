function hasActivation(feed) {
  return typeof feed?.activationKnown === 'boolean' || typeof feed?.active === 'boolean' || feed?.active === null;
}

export function feedActivationEventType(feed = {}) {
  if (feed.activationKnown === false || feed.active === null
    || (feed.activationKnown === true && typeof feed.active !== 'boolean')) return 'tool_feed_unknown';
  return feed.active === false ? 'tool_feed_inactive' : 'tool_feed_active';
}

// Legacy transcript payloads carry content, not a new lifecycle decision.
export function mergeFeedSnapshotActivation(incoming = [], previous = []) {
  const key = feed => String(feed?.feedId || '').trim();
  const known = new Map((Array.isArray(previous) ? previous : []).map(feed => [key(feed), feed]));
  const rows = Array.isArray(incoming) ? incoming : [];
  const present = new Set(rows.map(key));
  const result = rows.map(feed => {
    if (!feed) return feed;
    const prior = known.get(key(feed));
    if (hasActivation(feed) || !hasActivation(prior)) return feed;
    return { ...feed, active: prior.active, activationKnown: prior.activationKnown };
  });
  // Omitted content is not evidence that a lifecycle decision disappeared.
  // Keep only its marker; do not synthesize content or a new activation.
  for (const [id, prior] of known) {
    if (id && !present.has(id) && hasActivation(prior)) {
      result.push({ feedId: id, active: prior.active, activationKnown: prior.activationKnown });
    }
  }
  return result;
}
