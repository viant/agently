import { describe, expect, it, vi } from 'vitest';

import { computeDataMap, filterFeedRows, projectFeedFields } from './feedForgeWiring';

it('keeps calendar dates and editable values while making range labels readable', () => {
  const NativeDateTimeFormat = Intl.DateTimeFormat;
  const formatter = vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(function(_locale, options) {return new NativeDateTimeFormat('en-US', options);});
  const previousTZ = process.env.TZ;
  process.env.TZ = 'America/Los_Angeles';
  try {
    const got = projectFeedFields({start:'2026-10-21',end:{year:2026,monthIndex:11,day:31}}, {display:{transform:'dateRangeLabel'},editable:{transform:'dateRange'}});
    expect(got.display.replace(/\s/g,' ')).toBe('Oct 21 – Dec 31, 2026');
    expect(got.editable).toEqual({start:'2026-10-21',end:'2026-12-31'});
    const label = (start, end) => projectFeedFields({start,end},{display:{transform:'dateRangeLabel'}}).display;
    expect(label('2026-10-21','2026-10-21')).toBe('Oct 21, 2026');
    expect(label('','2026-12-31')).toBe('Dec 31, 2026');
    expect(label('2026-12-31','2026-10-21')).toBe('2026-12-31 – 2026-10-21');
    expect(label('2026-02-31','2026-03-02')).toBe('2026-02-31 – 2026-03-02');
    expect(label('invalid','')).toBe('invalid');
    expect(label('','')).toBe('');
  } finally {
    formatter.mockRestore();
    if (previousTZ === undefined) delete process.env.TZ; else process.env.TZ = previousTZ;
  }
});

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

  it('projects an editable draft and formats date-part objects', () => {
    const got = computeDataMap({
      dataSources: {
        plan: { source: 'output.plan' },
        editDraft: {
          dataSourceRef: 'plan',
          fields: {
            planId: 'planId',
            totalBudget: 'totalBudget',
            startDate: { path: 'campaignDates.startDate', transform: 'dateParts' },
            flight: {
              transform: 'dateRange',
              startPath: 'campaignDates.startDate',
              endPath: 'campaignDates.endDate',
            },
          },
        },
      },
      dataFeed: {
        name: 'plan',
        data: {
          output: {
            plan: {
              planId: 'plan-1',
              totalBudget: 250000,
              campaignDates: {
                startDate: { year: 2026, monthIndex: 8, day: 15 },
                endDate: { year: 2026, monthIndex: 9, day: 31 },
              },
            },
          },
        },
      },
    });

    expect(got.editDraft).toEqual([{
      planId: 'plan-1',
      totalBudget: 250000,
      startDate: '2026-09-15',
      flight: { start: '2026-09-15', end: '2026-10-31' },
    }]);
  });

  it('flattens configured nested collections without domain-specific code', () => {
    const got = computeDataMap({
      dataSources: {
        root: { source: 'output' },
        inventory: {
          dataSourceRef: 'root',
          selectors: { data: 'groups' },
          flatten: {
            sources: [{
              path: 'items',
              parentFields: { group: 'name' },
              fields: { id: 'id', label: 'title' },
              values: { kind: 'inventory' },
            }],
          },
        },
        coverage: { dataSourceRef: 'inventory', aggregate: { countAs: 'count' } },
      },
      dataFeed: {
        name: 'root',
        data: { output: { groups: [{ name: 'A', items: [{ id: 1, title: 'One' }] }] } },
      },
    });

    expect(got.inventory).toEqual([{ id: 1, label: 'One', group: 'A', kind: 'inventory' }]);
    expect(got.coverage).toEqual([{ count: 1 }]);
  });

  it('projects scalar collections through the explicit root selector', () => {
    const got = computeDataMap({
      dataSources: {
        root: { source: 'output' },
        codes: {
          dataSourceRef: 'root',
          selectors: { data: 'codes' },
          flatten: { sources: [{ path: '$', fields: { code: '$' } }] },
        },
      },
      dataFeed: { name: 'root', data: { output: { codes: ['501', '803'] } } },
    });
    expect(got.codes).toEqual([{ code: '501' }, { code: '803' }]);
  });

  it('excludes aggregate rows through generic datasource metadata', () => {
    expect(filterFeedRows([
      { id: 1, name: 'Item A' },
      { id: 2, name: 'total' },
    ], { exclude: { field: 'name', equalsIgnoreCase: 'TOTAL' } })).toEqual([
      { id: 1, name: 'Item A' },
    ]);

    const got = computeDataMap({
      dataSources: {
        root: { source: 'output' },
        items: {
          dataSourceRef: 'root',
          selectors: { data: 'items' },
          exclude: { field: 'name', equalsIgnoreCase: 'TOTAL' },
        },
      },
      dataFeed: {
        name: 'root',
        data: { output: { items: [{ name: 'Keep' }, { name: 'TOTAL' }] } },
      },
    });
    expect(got.items).toEqual([{ name: 'Keep' }]);
  });
});
