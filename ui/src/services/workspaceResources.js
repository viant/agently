import {createMCPResourceFS} from 'forge/resources';
import {request} from './httpClient';

export class WorkspaceResources {
  constructor({createFS = createMCPResourceFS, read = request} = {}) {
    this.createFS = createFS;
    this.read = read;
    this.adapters = new Map();
    this.configuration = '';
  }
  clear() {
    for (const adapter of this.adapters.values()) adapter.invalidate();
    this.adapters.clear();
    this.configuration = '';
  }
  configure(scopeKey, entries = []) {
    const signature = JSON.stringify([scopeKey, entries]);
    if (signature === this.configuration) return;
    this.clear();
    if (!scopeKey) return;
    for (const entry of entries) {
      if (!entry.server || !entry.providerIdentity || !entry.namespace) continue;
      const key = JSON.stringify([entry.providerIdentity, entry.namespace]);
      if (this.adapters.has(key)) throw new Error('Duplicate resource archive namespace');
      const adapter = this.createFS({
        namespace: entry.namespace, scopeKey,
        archiveFormat: entry.format || 'tar.gz',
        preloadKinds: entry.kinds,
        readResource: ({uri}) => this.read(`/api/mcp/resources/read?${new URLSearchParams({server: entry.server, uri})}`, {retries: 0}),
        prepareDataSource: (source, {id}) => {
          const {backend, ...definition} = source;
          return {...definition, service: {endpoint: 'agentlyAPI', uri: `/v1/api/datasources/${encodeURIComponent(id)}/fetch`, method: 'POST'}};
        },
      });
      this.adapters.set(key, adapter);
    }
    this.configuration = signature;
    void this.preload();
  }
  preload() { return Promise.all([...this.adapters.values()].map(adapter => adapter.preload())); }
  loadWindow(definition) {
    const owner = definition?.resource?.providerIdentity;
    const uri = definition?.resource?.uri || '';
    const namespace = /^window:\/\/([^/]+)\//.exec(uri)?.[1];
    return this.adapters.get(JSON.stringify([owner, namespace]))?.loadWindow(definition) || Promise.resolve(definition);
  }
}

export const workspaceResources = new WorkspaceResources();
if (typeof window !== 'undefined') {
  window.addEventListener('agently:unauthorized', () => workspaceResources.clear());
  window.addEventListener('agently:logout', () => workspaceResources.clear());
  window.addEventListener('agently:authorized', () => workspaceResources.clear());
}
