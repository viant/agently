import { describe, expect, it } from 'vitest';

import { parseConversationAndElicitation } from './ElicitationForm';
import {
  buildApprovalEditorState,
  collectElicitationFormValues,
  elicitationDataBindingKey,
  extractToolApprovalMeta,
  prepareRequestedSchema,
  resolveElicitationSubmitAction,
  serializeApprovalEditedFields
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

  it('hides internal metadata fields from rendered schema', () => {
    expect(prepareRequestedSchema({
      type: 'object',
      required: ['name', '_title'],
      properties: {
        name: { type: 'string' },
        _title: { type: 'string', const: 'OS Env Access' }
      }
    })).toEqual({
      type: 'object',
      required: ['name'],
      properties: {
        name: { type: 'string' }
      }
    });
  });

  it('extracts tool approval metadata from schema constants', () => {
    expect(extractToolApprovalMeta({
      type: 'object',
      properties: {
        _type: { type: 'string', const: 'tool_approval' },
        _title: { type: 'string', const: 'OS Env Access' },
        _toolName: { type: 'string', const: 'system/os/getEnv' },
        _acceptLabel: { type: 'string', const: 'Allow' },
        _rejectLabel: { type: 'string', const: 'Deny' },
        _cancelLabel: { type: 'string', const: 'Cancel' }
      }
    })).toEqual({
      type: 'tool_approval',
      title: 'OS Env Access',
      toolName: 'system/os/getEnv',
      acceptLabel: 'Allow',
      rejectLabel: 'Deny',
      cancelLabel: 'Cancel'
    });
  });

  it('uses submit for schema forms and accept for tool approvals', () => {
    expect(resolveElicitationSubmitAction({
      type: 'object',
      properties: {
        path: { type: 'string' }
      }
    })).toBe('submit');

    expect(resolveElicitationSubmitAction({
      type: 'object',
      properties: {
        _type: { type: 'string', const: 'tool_approval' }
      }
    })).toBe('accept');
  });

  it('extracts rich approval metadata with editors from _approvalMeta', () => {
    const meta = {
      type: 'tool_approval',
      title: 'OS Env Access',
      toolName: 'system/os/getEnv',
      acceptLabel: 'Allow',
      rejectLabel: 'Deny',
      cancelLabel: 'Cancel',
      editors: [
        {
          name: 'names',
          kind: 'checkbox_list',
          path: 'names',
          label: 'Environment variables',
          description: 'Choose which environment variables this tool may access.',
          options: [
            { id: 'HOME', label: 'HOME', selected: true },
            { id: 'SHELL', label: 'SHELL', selected: true },
            { id: 'PATH', label: 'PATH', selected: true }
          ]
        }
      ]
    };
    expect(extractToolApprovalMeta({
      type: 'object',
      properties: {
        _approvalMeta: { type: 'string', const: JSON.stringify(meta) }
      }
    })).toEqual({
      ...meta,
      forge: undefined,
      message: '',
      editors: [
        {
          ...meta.editors[0],
          options: meta.editors[0].options.map((option) => ({ ...option, description: '', item: undefined }))
        }
      ]
    });
    expect(buildApprovalEditorState(meta)).toEqual({ names: ['HOME', 'SHELL', 'PATH'] });
    expect(serializeApprovalEditedFields(meta, { names: ['HOME', 'PATH'] })).toEqual({ names: ['HOME', 'PATH'] });
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
