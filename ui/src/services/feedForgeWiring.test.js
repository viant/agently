import { describe, expect, it } from 'vitest';

import { computeDataMap } from './feedForgeWiring';

describe('computeDataMap', () => {
  it('resolves root-level output selectors directly from feed payload', () => {
    const got = computeDataMap({
      dataSources: {
        commands: { source: 'output.commands' },
      },
      dataFeed: {
        name: 'commands',
        data: {
          output: {
            commands: [
              { input: 'pwd', output: '/tmp' },
              { input: 'ls', output: 'a\nb' },
            ],
          },
        },
      },
    });

    expect(got.commands).toEqual([
      { input: 'pwd', output: '/tmp' },
      { input: 'ls', output: 'a\nb' },
    ]);
  });

  it('supports root-level output sources with child selectors', () => {
    const got = computeDataMap({
      dataSources: {
        snapshot: { source: 'output' },
        changes: { dataSourceRef: 'snapshot', selectors: { data: 'changes' } },
      },
      dataFeed: {
        name: 'snapshot',
        data: {
          output: {
            changes: [
              { path: 'foo.go', action: 'modify' },
              { path: 'bar_test.go', action: 'add' },
            ],
          },
        },
      },
    });

    expect(got.snapshot).toEqual([
      {
        changes: [
          { path: 'foo.go', action: 'modify' },
          { path: 'bar_test.go', action: 'add' },
        ],
      },
    ]);
    expect(got.changes).toEqual([
      { path: 'foo.go', action: 'modify' },
      { path: 'bar_test.go', action: 'add' },
    ]);
  });

  it('falls back to root-level data when the payload is not wrapped in output', () => {
    const got = computeDataMap({
      dataSources: {
        results: { source: 'output.files' },
      },
      dataFeed: {
        name: 'results',
        data: {
          files: [
            { Path: 'pathway.go', Matches: 4 },
            { Path: 'caller.go', Matches: 2 },
          ],
        },
      },
    });

    expect(got.results).toEqual([
      { Path: 'pathway.go', Matches: 4 },
      { Path: 'caller.go', Matches: 2 },
    ]);
  });
});
