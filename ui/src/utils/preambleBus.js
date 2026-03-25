/**
 * Tiny pub/sub for preamble text updates.
 * IterationBlock publishes the current page's preamble when the user paginates.
 * The preamble-bubble component subscribes and re-renders.
 */
const listeners = new Map();

export function subscribePreamble(iterationRef, callback) {
  if (!listeners.has(iterationRef)) {
    listeners.set(iterationRef, new Set());
  }
  listeners.get(iterationRef).add(callback);
  return () => {
    const set = listeners.get(iterationRef);
    if (set) {
      set.delete(callback);
      if (set.size === 0) listeners.delete(iterationRef);
    }
  };
}

export function publishPreamble(iterationRef, preambleText) {
  const set = listeners.get(iterationRef);
  if (!set) return;
  for (const cb of set) cb(preambleText);
}
