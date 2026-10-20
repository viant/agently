import {describe, it, expect, vi} from 'vitest';
import {WorkspaceResources} from './workspaceResources';

describe('workspace metadata archives', () => {
  const entry = {server: 'studio', providerIdentity: 'owner', namespace: 'demo'};
  const window = {resource: {providerIdentity: 'owner', uri: 'window://demo/customer'}};
  function fixture() {
    const instances = [];
    const createFS = vi.fn(options => {
      const adapter = {options, invalidate: vi.fn(), preload: vi.fn().mockResolvedValue([]), loadWindow: vi.fn(value => Promise.resolve({...value, loaded: true}))};
      instances.push(adapter); return adapter;
    });
    const read = vi.fn().mockResolvedValue({contents: []});
    return {host: new WorkspaceResources({createFS, read}), instances, createFS, read};
  }
  it('reuses each scoped adapter and clears it on account/configuration change', () => {
    const {host, instances, createFS} = fixture();
    host.configure('alice', [entry]); host.configure('alice', [entry]);
    expect(createFS).toHaveBeenCalledTimes(1);
    expect(instances[0].preload).toHaveBeenCalledTimes(1);
    host.configure('bob', [entry]);
    expect(instances[0].invalidate).toHaveBeenCalledOnce();
    expect(instances[1].options.scopeKey).toBe('bob');
    host.clear(); expect(instances[1].invalidate).toHaveBeenCalledOnce();
  });
  it('routes only matching provider and namespace and leaves other windows untouched', async () => {
    const {host, instances} = fixture(); host.configure('alice', [entry]);
    expect((await host.loadWindow(window)).loaded).toBe(true);
    const other = {resource: {providerIdentity: 'other', uri: 'window://demo/customer'}};
    expect(await host.loadWindow(other)).toBe(other);
    expect(instances[0].loadWindow).toHaveBeenCalledTimes(1);
    expect(window.loaded).toBeUndefined();
  });
  it('uses the host resource endpoint and projects datasource transport without mutating source', async () => {
    const {host, instances, read} = fixture(); host.configure('alice', [entry]);
    await instances[0].options.readResource({uri: 'resource-archive://demo/dialogs.tar.gz'});
    expect(read).toHaveBeenCalledWith('/api/mcp/resources/read?server=studio&uri=resource-archive%3A%2F%2Fdemo%2Fdialogs.tar.gz', {retries: 0});
    const source = {id: 'records', backend: {kind: 'mcp_tool'}, parameters: [{name: 'ID'}]};
    const projected = instances[0].options.prepareDataSource(source, {id: 'records'});
    expect(projected.service.uri).toBe('/v1/api/datasources/records/fetch');
    expect(projected.backend).toBeUndefined(); expect(source.backend).toEqual({kind: 'mcp_tool'});
  });
});
