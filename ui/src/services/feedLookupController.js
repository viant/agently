/**
 * Owns the single lookup request that a Tool Feed can display at one time.
 * Opening another lookup or removing the feed settles the previous request as
 * a cancellation so Forge callers never retain an unresolved promise.
 */
export function createFeedLookupController(onChange = () => {}) {
  let activeRequest = null;
  let disposed = false;
  let requestId = 0;

  const publish = () => {
    if (disposed) return;
    onChange(activeRequest ? {
      id: activeRequest.id,
      request: activeRequest.request,
    } : null);
  };

  const settle = (target, value) => {
    if (!target || target.settled) return false;
    target.settled = true;
    target.request?.signal?.removeEventListener?.('abort', target.onAbort);
    if (activeRequest === target) {
      activeRequest = null;
      publish();
    }
    target.resolve(value ?? null);
    return true;
  };

  const cancel = () => settle(activeRequest, null);

  const open = (request = {}) => {
    if (disposed || request?.signal?.aborted) return Promise.resolve(null);
    cancel();
    return new Promise((resolve) => {
      const target = {
        id: ++requestId,
        request: request && typeof request === 'object' ? request : {},
        resolve,
        settled: false,
        onAbort: null,
      };
      target.onAbort = () => settle(target, null);
      activeRequest = target;
      target.request?.signal?.addEventListener?.('abort', target.onAbort, { once: true });
      publish();
    });
  };

  return {
    activate: () => { disposed = false; },
    cancel,
    dispose: () => {
      disposed = true;
      settle(activeRequest, null);
    },
    getSnapshot: () => activeRequest ? {
      id: activeRequest.id,
      request: activeRequest.request,
    } : null,
    open,
    select: (row) => settle(activeRequest, row),
  };
}
