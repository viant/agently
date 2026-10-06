import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/agentlyClient', () => ({ client: {} }));
import { normalizeBackendConnections } from './useAgUiBackendSelection';
describe('configured backend selection', () => {
  it('keeps native identity fixed and admits only distinct standard connections', () => {
    const values = normalizeBackendConnections([
      { id: 'agently', profile: 'standard', label: 'Foreign override' },
      { id: 'remote', profile: 'standard', label: 'Public demo', ephemeral: true },
      { id: 'remote', profile: 'standard', label: 'Duplicate' },
      { id: 'unsupported', profile: 'unknown' }, { id: '', profile: 'standard' },
    ]);
    expect(values.map(value => value.id)).toEqual(['agently', 'remote']);
    expect(values[0]).toMatchObject({ profile: 'agently', label: 'Agently' });
    expect(values[1]).toMatchObject({ label: 'Public demo', ephemeral: true });
  });
  it('keeps the existing native shell when no remote backend is configured', () => {
    expect(normalizeBackendConnections(null)).toEqual([{ id: 'agently', label: 'Agently', profile: 'agently' }]);
  });
});
