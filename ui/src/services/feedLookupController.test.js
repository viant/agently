import { describe, expect, it, vi } from 'vitest';

import { createFeedLookupController } from './feedLookupController';

describe('createFeedLookupController', () => {
  it('resolves a selected raw row and publishes open and closed states', async () => {
    const onChange = vi.fn();
    const controller = createFeedLookupController(onChange);
    const row = { id: 42, label: 'Selected row' };

    const result = controller.open({ lookup: { dataSourceRef: 'records' } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({
      request: { lookup: { dataSourceRef: 'records' } },
    }));

    controller.select(row);

    await expect(result).resolves.toBe(row);
    expect(onChange).toHaveBeenLastCalledWith(null);
  });

  it('settles cancel, replacement, abort, and disposal as null', async () => {
    const controller = createFeedLookupController();
    const first = controller.open({ lookup: { dataSourceRef: 'first' } });
    const second = controller.open({ lookup: { dataSourceRef: 'second' } });
    await expect(first).resolves.toBeNull();

    controller.cancel();
    await expect(second).resolves.toBeNull();

    const abortController = new AbortController();
    const aborted = controller.open({ signal: abortController.signal });
    abortController.abort();
    await expect(aborted).resolves.toBeNull();

    const pendingAtUnmount = controller.open({ lookup: { dataSourceRef: 'third' } });
    controller.dispose();
    await expect(pendingAtUnmount).resolves.toBeNull();
    await expect(controller.open({})).resolves.toBeNull();
  });

  it('can be reactivated after a React strict-mode cleanup cycle', async () => {
    const controller = createFeedLookupController();
    controller.dispose();
    controller.activate();

    const result = controller.open({ lookup: { dataSourceRef: 'records' } });
    controller.select({ id: 7 });

    await expect(result).resolves.toEqual({ id: 7 });
  });
});
