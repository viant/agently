import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  clearPendingFeedDraft,
  markFeedDataSourcesDirty,
  resetPendingFeedDrafts,
  restorePendingFeedDraft,
  savePendingFeedDraft,
} from './feedDraftState';

describe('pending feed draft state', () => {
  afterEach(resetPendingFeedDrafts);

  it('restores dirty form and collection state when the authoritative feed did not change', () => {
    const form = { setFormData: vi.fn() };
    const collection = { setCollection: vi.fn(), setSelection: vi.fn() };
    const contexts = {
      draft: { handlers: { dataSource: form }, signals: { formStatus: { value: {} } } },
      rows: { handlers: { dataSource: collection }, signals: { formStatus: { value: {} } } },
    };
    savePendingFeedDraft('plan', 'conv-1', {
      sourceSignature: 'v1',
      formDataSourceRef: 'draft',
      formData: { instruction: 'Add Display' },
      collections: { rows: { rows: [{ id: 1 }], selectedRows: [{ id: 1 }] } },
    });
    expect(restorePendingFeedDraft('plan', 'conv-1', 'v1', { Context: (ref) => contexts[ref] })).toBe('restored');
    expect(form.setFormData).toHaveBeenCalledWith({ values: { instruction: 'Add Display' } });
    expect(collection.setCollection).toHaveBeenCalledWith([{ id: 1 }]);
    expect(collection.setSelection).toHaveBeenCalledWith({ selected: [{ id: 1 }] });
    expect(contexts.draft.signals.formStatus.value).toEqual({ dirty: true });
    expect(contexts.rows.signals.formStatus.value).toEqual({ dirty: true });
  });

  it('clears the pending overlay after authoritative feed data changes', () => {
    savePendingFeedDraft('plan', 'conv-1', { sourceSignature: 'v1' });
    expect(restorePendingFeedDraft('plan', 'conv-1', 'v2', null)).toBe('cleared');
    expect(restorePendingFeedDraft('plan', 'conv-1', 'v2', null)).toBe('none');
    expect(clearPendingFeedDraft('plan', 'conv-1')).toBe(false);
  });

  it('restores persistent preview dirty markers after feed rewiring', () => {
    const statuses = {
      channels: { value: { dirty: false }, peek() { return this.value; } },
      frequency: { value: { dirty: false }, peek() { return this.value; } },
    };
    const context = { Context: (ref) => ({ signals: { formStatus: statuses[ref] } }) };
    expect(markFeedDataSourcesDirty(context, ['frequency', 'channels', 'frequency'])).toBe(2);
    expect(statuses.frequency.value.dirty).toBe(true);
    expect(statuses.channels.value.dirty).toBe(true);
  });
});
