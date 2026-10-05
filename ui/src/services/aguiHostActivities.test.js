import { beforeEach, describe, expect, it } from 'vitest';
import { getAgUiHostActivities, replaceAgUiHostActivities, resetAgUiHostActivities } from './aguiHostActivities';
const activity = (id, instance) => ({ id, role: 'activity', activityType: 'mcp-apps', content: {
  serverId: `agui-app:${instance}`, serverHash: 'hash', resourceUri: 'ui://same/app',
  _agentlyApp: { version: '1', appInstanceId: instance, publicServerId: `agui-app:${instance}`, serverHash: 'hash', resourceUri: 'ui://same/app', nativeTurnId: 'turn' },
  result: { _meta: { private: true } },
} });
beforeEach(resetAgUiHostActivities);
describe('renderer-only AG-UI host activities', () => {
  it('keeps same-URI instances distinct and repeated snapshots stable', () => {
    const input = [activity('a', 'one'), activity('b', 'two')];
    replaceAgUiHostActivities('thread', input, 'agently');
    const first = getAgUiHostActivities('thread');
    expect(first).toHaveLength(2);
    expect(first[0].mountKey).not.toBe(first[1].mountKey);
    replaceAgUiHostActivities('thread', structuredClone(input), 'agently');
    expect(getAgUiHostActivities('thread')).toBe(first);
    input[0].content.result._meta.private = false;
    expect(first[0].content.result._meta.private).toBe(true);
  });
  it('rejects foreign profile authority and mismatched bindings', () => {
    replaceAgUiHostActivities('foreign', [activity('a', 'one')]);
    expect(getAgUiHostActivities('foreign')).toHaveLength(0);
    const forged = activity('a', 'one');
    forged.content.serverId = 'agui-app:other';
    replaceAgUiHostActivities('thread', [forged], 'agently');
    expect(getAgUiHostActivities('thread')).toHaveLength(0);
  });
  it('removes absent activities and clears all host data at the account boundary', () => {
    replaceAgUiHostActivities('thread', [activity('a', 'one')], 'agently');
    replaceAgUiHostActivities('thread', [], 'agently');
    expect(getAgUiHostActivities('thread')).toHaveLength(0);
    replaceAgUiHostActivities('thread', [activity('a', 'one')], 'agently');
    resetAgUiHostActivities();
    expect(getAgUiHostActivities('thread')).toHaveLength(0);
  });
});
