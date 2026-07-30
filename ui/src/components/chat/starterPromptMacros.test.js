import { describe, expect, it } from 'vitest';

import { resolveStarterPromptMacros } from './starterPromptMacros.js';
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
