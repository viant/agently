import { hasActiveSteps } from '../src/services/chatService';

describe('hasActiveSteps – table-driven', () => {
  const cases = [
    {
      name: 'empty list',
      in: [],
      want: false,
    },
    {
      name: 'all completed',
      in: [ { reason: 'tool_call', statusText: 'completed' }, { reason: 'thinking', statusText: 'done' } ],
      want: false,
    },
    {
      name: 'pending step',
      in: [ { reason: 'thinking', statusText: 'pending' } ],
      want: true,
    },
    {
      name: 'running step',
      in: [ { reason: 'tool_call', statusText: 'running' } ],
      want: true,
    },
    {
      name: 'elicitation with payload accepted',
      in: [ { reason: 'elicitation', statusText: 'accepted', elicitationPayloadId: 'pid' } ],
      want: false,
    },
  ];

  cases.forEach(tc => {
    test(tc.name, () => {
      expect(hasActiveSteps(tc.in)).toEqual(tc.want);
    });
  });
});

