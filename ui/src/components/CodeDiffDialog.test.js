import { describe, expect, it } from 'vitest';

import {
  hasPreviousDiffVersion,
  normalizeCodeDiffMode,
  resolveCodeDiffContent,
  resolveCodeDiffTabs,
} from './CodeDiffDialog.jsx';

describe('CodeDiffDialog helpers', () => {
  it('exposes current, prev, and diff tabs when a previous version exists', () => {
    expect(resolveCodeDiffTabs({
      hasPrev: true,
      prevUri: '/repo/file.go',
      current: 'current body',
      prev: 'previous body',
      diff: '--- before\n+++ after',
    })).toEqual(['current', 'prev', 'diff']);
  });

  it('hides the prev tab when there is no previous version', () => {
    expect(resolveCodeDiffTabs({
      hasPrev: false,
      current: 'current body',
      diff: '--- before\n+++ after',
    })).toEqual(['current', 'diff']);
  });

  it('normalizes prev mode back to current when no previous version exists', () => {
    const state = {
      hasPrev: false,
      current: 'current body',
      prev: '',
    };
    expect(hasPreviousDiffVersion(state)).toBe(false);
    expect(normalizeCodeDiffMode('prev', state)).toBe('current');
  });

  it('returns the correct content for each selected tab', () => {
    const state = {
      hasPrev: true,
      prevUri: '/repo/file.go',
      current: 'current body',
      prev: 'previous body',
      diff: '--- before\n+++ after',
    };
    expect(resolveCodeDiffContent(state, 'current')).toBe('current body');
    expect(resolveCodeDiffContent(state, 'prev')).toBe('previous body');
    expect(resolveCodeDiffContent(state, 'diff')).toBe('--- before\n+++ after');
  });
});
