import { describe, it, expect } from 'vitest';
import { buildStandardMcpDocument, readStandardAppResource, resolveSandboxUrl, normalizeAppCsp, MCP_APP_MIME } from './standardDocument.js';
const resource = { uri: 'ui://generic/app', mimeType: MCP_APP_MIME, text: '<!doctype html><script>window.ready=true</script>' };
describe('official app resources and CSP', () => {
  it('uses restrictive official defaults and permits inline bundled app code without eval or blob', () => {
    const doc = buildStandardMcpDocument(resource);
    expect(doc).toContain("script-src 'self' 'unsafe-inline'"); expect(doc).not.toContain('unsafe-eval'); expect(doc).not.toContain('blob:');
    expect(doc).toContain("connect-src 'none'"); expect(doc).toContain("frame-src 'none'"); expect(doc).toContain('window.ready=true');
  });
  it('preserves resource-owned domain declarations, rejects fragments and exact resource/MIME substitution', () => {
    const doc = buildStandardMcpDocument({ ...resource, _meta: { ui: { csp: { connectDomains: ['https://api.example'], resourceDomains: ['https://cdn.example'], baseUriDomains: ['https://base.example'] } } } });
    expect(doc).toContain('connect-src https://api.example'); expect(doc).toContain('base-uri https://base.example');
    for (const value of ['*', 'data:', "https://cdn.example; connect-src *", 'https://a.example/path', 'https://name:password@a.example']) expect(() => normalizeAppCsp({ resourceDomains: [value] })).toThrow();
    expect(() => readStandardAppResource({ contents: [resource] }, 'ui://other/app')).toThrow();
    expect(() => buildStandardMcpDocument({ ...resource, mimeType: 'text/html' })).toThrow();
    expect(() => readStandardAppResource({ contents: [resource, resource] }, resource.uri)).toThrow();
  });
  it('supports base64 HTML and requires a dedicated sandbox origin', () => {
    expect(readStandardAppResource({ contents: [{ ...resource, text: undefined, blob: btoa('<p>fixture</p>') }] }, resource.uri).html).toBe('<p>fixture</p>');
    expect(resolveSandboxUrl('https://sandbox.example/mcp-app-sandbox.html', 'https://host.example')).toContain('#hostOrigin=');
    expect(() => resolveSandboxUrl('https://host.example/sandbox', 'https://host.example')).toThrow();
    expect(() => resolveSandboxUrl('http://sandbox.example/sandbox', 'https://host.example')).toThrow();
  });
});
