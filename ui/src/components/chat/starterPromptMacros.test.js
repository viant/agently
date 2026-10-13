import { describe, expect, it } from 'vitest';

import { resolveStarterPromptMacros, resolveComposerPromptMacros } from './starterPromptMacros.js';
import { flattenStored, parseTokens } from '../lookups/tokens.js';

describe('resolveStarterPromptMacros', () => {
  const now = new Date(2026, 6, 30, 18, 15, 0);

  it('resolves current-date and relative-date macros using local calendar dates', () => {
    expect(resolveStarterPromptMacros(
      'Build from ${7 days ago} through ${now}; today is ${today}.',
      now,
    )).toBe('Build from 2026-07-23 through 2026-07-30; today is 2026-07-30.');
  });

  it('accepts compact, singular, and case-insensitive relative forms', () => {
    expect(resolveStarterPromptMacros(
      '${1 day ago}|${7days ago}|${NOW}',
      now,
    )).toBe('2026-07-29|2026-07-23|2026-07-30');
  });

  it('leaves unknown macros unchanged', () => {
    expect(resolveStarterPromptMacros('Keep ${order_id}.', now)).toBe('Keep ${order_id}.');
  });

  it('resolves raw Forge composer prefills with dates and a money default', () => {
    const resolved = resolveComposerPromptMacros(
      'Plan from ${beginning of next month} to ${end of next month} for ${money 10 000}.',
      new Date(2026, 9, 13),
    );
    expect(parseTokens(resolved).map(token => token.name)).toEqual(['date_from', 'date_to', 'money']);
    expect(flattenStored(resolved, [])).toBe('Plan from 2026-11-01 to 2026-11-30 for $10 000.');
    expect(resolveComposerPromptMacros(resolved, new Date(2026, 10, 13))).toBe(resolved);
  });

  it('preserves existing token labels and user-edited dates during composer expansion', () => {
    const existing = '@{date_from:2026-11-04 "From: 2026-11-04"} @{order:7 "Label ${now}"}';
    expect(resolveComposerPromptMacros(existing, now)).toBe(existing);
  });

  it.each([
    [new Date(2026, 9, 13), '2026-11-01', '2026-11-30'],
    [new Date(2026, 11, 31), '2027-01-01', '2027-01-31'],
    [new Date(2027, 0, 31), '2027-02-01', '2027-02-28'],
    [new Date(2028, 0, 31), '2028-02-01', '2028-02-29'],
  ])('resolves next-month calendar boundaries for %s', (date, start, end) => {
    expect(resolveStarterPromptMacros('${beginning of next month}|${end of next month}', date)).toBe(`${start}|${end}`);
  });

  it('accepts start and beginning aliases with case-insensitive spacing', () => {
    expect(resolveStarterPromptMacros('${ START of NEXT month }|${begining of next month}', now)).toBe('2026-08-01|2026-08-01');
  });

  it('emits next-month date chips and an editable money slot for media plans', () => {
    const resolved = resolveStarterPromptMacros(
      'Plan from ${beginning of next month} to ${end of next month} for ${money}.',
      new Date(2026, 9, 13), { dateTokens: true },
    );
    expect(parseTokens(resolved).map(({ name, id }) => ({ name, id }))).toEqual([
      { name: 'date_from', id: '2026-11-01' },
      { name: 'date_to', id: '2026-11-30' },
      { name: 'money', id: '?' },
    ]);
    expect(flattenStored(resolved.replace('@{money:? "$[___]"}', '@{money:5000 "$5000"}'), [])).toBe('Plan from 2026-11-01 to 2026-11-30 for $5 000.');
  });

  it('can emit editable date chips for starter-prompt ranges', () => {
    const resolved = resolveStarterPromptMacros(
      'from ${7 days ago} through ${now}',
      now,
      { dateTokens: true },
    );
    expect(resolved).toBe(
      'from @{date_from:2026-07-23 "From: 2026-07-23"} through @{date_to:2026-07-30 "To: 2026-07-30"}',
    );
    expect(parseTokens(resolved)).toHaveLength(2);
    expect(flattenStored(resolved, [])).toBe('from 2026-07-23 through 2026-07-30');
  });
});
