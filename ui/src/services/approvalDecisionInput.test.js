import { describe, expect, it } from 'vitest';
import { approvalDecisionInput } from './approvalDecisionInput';
describe('approval decision payload', () => {
  it('does not send display-only tool arguments as editor values', () => {
    expect(approvalDecisionInput({ metadata: { approval: { data: { names: ['FIXTURE'] } } } }, 'approve', { names: ['FIXTURE'] })).toEqual({ action: 'approve' });
  });
  it('sends only explicitly authorized named editor values', () => {
    const item = { metadata: JSON.stringify({ approval: { editors: [{ name: 'command' }] } }) };
    expect(approvalDecisionInput(item, 'approve', { command: '', unsafe: true })).toEqual({ action: 'approve', editedFields: { command: '' } });
  });
  it('sends review form data through payload, including false and zero values', () => {
    const item = { metadata: { review: { requestedSchema: { type: 'object', properties: { rows: {}, confirm: {}, count: {} } } } } };
    expect(approvalDecisionInput(item, 'approve', { rows: [{ selected: false }], confirm: false, count: 0, privateArgument: 'excluded' })).toEqual({ action: 'approve', payload: { rows: [{ selected: false }], confirm: false, count: 0 } });
  });
  it('does not apply edits when rejecting or canceling', () => {
    expect(approvalDecisionInput({ metadata: { approval: { editors: [{ name: 'x' }] } } }, 'reject', { x: 'changed' })).toEqual({ action: 'reject' });
  });
});
