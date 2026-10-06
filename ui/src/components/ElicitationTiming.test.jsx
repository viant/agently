import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import ElicitationTiming, { elicitationDeadline, elicitationTimingText } from './ElicitationTiming';
import { normalizeElicitationDialogState } from '../services/elicitationBus';
import { jsonSchemaToFields } from 'forge/utils/schema';
import { validateSchemaFormFields } from 'forge/widgets/schemaFormValidation';
import SchemaFormChoices from 'forge/widgets/SchemaFormChoices';

const schema = {type: 'object', required: ['businesses'], properties: {businesses: {type: 'array', title: 'Business types', 'x-ui-widget': 'tags', default: [], 'x-ui-order': 10, minItems: 1, maxItems: 2, items: {type: 'string', enum: ['dispensaries', 'brands', 'delivery']}}}};

describe('elicitation timing and multiple choices', () => {
  it('uses server deadline, preserves metadata across dialog normalization and never invents a timer', () => {
    const deadline = '2026-10-06T12:00:00Z';
    const normalized = normalizeElicitationDialogState({elicitationId: 'ask', conversationId: 'conv', requestedSchema: schema, expiresAt: deadline});
    expect(elicitationDeadline(normalized)).toBe(Date.parse(deadline));
    expect(elicitationDeadline({elicitation: {createdAt: deadline, timeoutMs: 3000}})).toBe(Date.parse(deadline) + 3000);
    expect(elicitationDeadline({timeoutMs: 3000})).toBeNull();
    expect(renderToStaticMarkup(<ElicitationTiming source={{}} />)).toBe('');
  });
  it('changes countdown into late-answer guidance without an automatic decision', () => {
    expect(elicitationTimingText(65000, 0)).toContain('1:05');
    expect(elicitationTimingText(65000, 65000)).toBe('Taking longer? You can still answer or skip.');
    expect(renderToStaticMarkup(<ElicitationTiming source={{deadline: 1}} />)).toContain('You can still answer or skip');
  });
  it('renders array enums with multiple independent choices and no implicit selection', () => {
    const [field] = jsonSchemaToFields(schema);
    expect(field.widget).toBe('checkboxGroup');
    const empty = renderToStaticMarkup(<SchemaFormChoices field={field} value={[]} onChange={() => {}} />);
    expect(empty.match(/type="checkbox"/g)).toHaveLength(3);
    expect(empty).not.toContain('checked');
    const selected = renderToStaticMarkup(<SchemaFormChoices field={field} value={['brands', 'delivery']} onChange={() => {}} />);
    expect(selected.match(/checked=""/g)).toHaveLength(2);
    expect(validateSchemaFormFields([field], {businesses: ['brands', 'delivery']})).toEqual({});
    expect(validateSchemaFormFields([field], {businesses: []})).toEqual({businesses: 'Select at least 1'});
    expect(validateSchemaFormFields([field], {businesses: ['brands', 'unknown']})).toEqual({businesses: 'Invalid value'});
    expect(validateSchemaFormFields([field], {businesses: ['brands', 'delivery', 'dispensaries']})).toEqual({businesses: 'Select at most 2'});
  });
});
