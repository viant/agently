import { classifyMessage } from '../src/services/messageNormalizer';

describe('classifyMessage – table-driven', () => {
  const cases = [
    {
      name: 'execution message',
      input: { executions: [{ tool: 't', steps: [] }] },
      want: 'execution',
    },
    {
      name: 'mcpelicitation open',
      input: { role: 'mcpelicitation', status: 'open' },
      want: 'mcpelicitation',
    },
    {
      name: 'assistant form',
      input: { role: 'assistant', elicitation: { requestedSchema: {} } },
      want: 'form',
    },
    {
      name: 'plain bubble',
      input: { role: 'assistant', content: 'hello' },
      want: 'bubble',
    },
  ];

  cases.forEach(tc => {
    test(tc.name, () => {
      expect(classifyMessage(tc.input)).toEqual(tc.want);
    });
  });
});
