import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({buttons: [], resolve: vi.fn().mockResolvedValue({}), tick: vi.fn().mockResolvedValue({})}));
vi.mock('@blueprintjs/core', () => ({
  Button: (props) => { mocks.buttons.push(props); return <button>{props.children}</button>; },
  Dialog: (props) => <section>{props.children}</section>, Spinner: () => null,
  Classes: {DIALOG_BODY: '', DIALOG_FOOTER: '', DIALOG_FOOTER_ACTIONS: ''}
}));
vi.mock('forge/widgets/SchemaBasedForm.jsx', () => ({default: () => null}));
vi.mock('../services/agentlyClient', () => ({client: {resolveElicitation: mocks.resolve}}));
vi.mock('../services/chatRuntime', () => ({refreshAfterElicitationResolution: mocks.tick}));
import ElicitationForm from './chat/ElicitationForm';
import { ElicitationDialog } from './ElicitationOverlay';

const requestedSchema = {type: 'object', properties: {types: {type: 'array', items: {type: 'string', enum: ['brands', 'dispensaries']}}}};
const pending = {conversationId: 'root', elicitationId: 'ask', callbackURL: '/v1/api/conversations/child/elicitation/ask', requestedSchema, deadline: '2000-01-01T00:00:00Z'};
beforeEach(() => { mocks.buttons.length = 0; mocks.resolve.mockClear(); });
describe('ordinary elicitation actions', () => {
  for (const [name, element] of [['form', <ElicitationForm message={{elicitation: pending}} />], ['overlay', <ElicitationDialog pending={pending} />]]) {
    it(`${name} skips explicitly with empty payload and authoritative target, even after timeout`, async () => {
      const html = renderToStaticMarkup(element);
      expect(html).toContain('You can still answer or skip');
      const skip = mocks.buttons.find((button) => button.children === 'Skip');
      expect(skip).toBeDefined();
      expect(mocks.buttons.find((button) => button.intent === 'primary').disabled).toBe(false);
      await skip.onClick();
      expect(mocks.resolve).toHaveBeenCalledWith('child', 'ask', {action: 'decline', payload: {}});
    });
  }
  it('keeps the approval denial label and omits question countdown', () => {
    const schema = {type: 'object', properties: {_type: {const: 'tool_approval'}, _rejectLabel: {const: 'Deny'}}};
    const html = renderToStaticMarkup(<ElicitationForm message={{elicitation: {...pending, requestedSchema: schema}}} />);
    expect(html).not.toContain('You can still answer');
    expect(mocks.buttons.some((button) => button.children === 'Deny')).toBe(true);
    expect(mocks.buttons.some((button) => button.children === 'Skip')).toBe(false);
  });
});
