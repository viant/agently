import { describe, expect, it } from 'vitest';

import { parseConversationAndElicitation } from './ElicitationForm';
import {
  collectElicitationFormValues,
  elicitationDataBindingKey,
  prepareRequestedSchema
} from '../elicitationHelpers';

describe('ElicitationForm utilities', () => {
  it('parses conversation and elicitation ids from callback URLs', () => {
    expect(parseConversationAndElicitation({
      callbackURL: '/v1/elicitations/conv-1/elic-1/resolve'
    })).toEqual({
      conversationId: 'conv-1',
      elicitationId: 'elic-1'
    });

    expect(parseConversationAndElicitation({
      elicitation: {
        callbackURL: '/v1/api/conversations/conv-2/elicitation/elic-2'
      }
    })).toEqual({
      conversationId: 'conv-2',
      elicitationId: 'elic-2'
    });
  });

  it('normalizes array and object defaults in requested schema', () => {
    expect(prepareRequestedSchema({
      type: 'object',
      properties: {
        tags: { type: 'array', default: 'oops' },
        meta: { type: 'object' }
      }
    })).toEqual({
      type: 'object',
      properties: {
        tags: { type: 'array', default: [] },
        meta: { type: 'object', default: {} }
      }
    });
  });

  it('builds a stable data binding key', () => {
    expect(elicitationDataBindingKey('elic-1')).toBe('window.state.answers.elic_elic-1');
    expect(elicitationDataBindingKey('')).toBe('window.state.answers.elic_local');
  });

  it('collects tracked form values without touching the DOM', () => {
    expect(collectElicitationFormValues({
      dataBindingKey: 'window.state.answers.elic_test',
      formWrapperId: 'missing',
      schema: { properties: { color: { type: 'string' } } },
      trackedValues: { color: 'blue' }
    })).toEqual({ color: 'blue' });
  });
});
