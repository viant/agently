export const MCP_APP_MIME = 'text/html;profile=mcp-app';
export const MAX_APP_BYTES = 4 * 1024 * 1024;
const domainKeys = ['connectDomains', 'resourceDomains', 'frameDomains', 'baseUriDomains'];

// A resource declares origins, never arbitrary CSP fragments or keyword sources.
export function normalizeAppCsp(value = {}) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid app CSP');
  const result = {};
  for (const key of domainKeys) {
    if (value[key] === undefined) continue;
    if (!Array.isArray(value[key]) || value[key].length > 64) throw new Error('Invalid app CSP domains');
    result[key] = [...new Set(value[key].map(origin => {
      const parsed = new URL(origin);
      if (typeof origin !== 'string' || !(key === 'connectDomains' ? ['https:', 'http:', 'wss:', 'ws:'] : ['https:', 'http:']).includes(parsed.protocol) || parsed.username || parsed.password
        || parsed.pathname !== '/' || parsed.search || parsed.hash) throw new Error('App CSP requires HTTP(S) origins');
      return parsed.origin;
    }))];
  }
  return result;
}
export function readStandardAppResource(result, uri) {
  if (typeof uri !== 'string' || !uri.startsWith('ui://')) throw new Error('App resource requires an exact ui:// URI');
  const matches = result?.contents?.filter(item => item.uri === uri) || [];
  if (matches.length !== 1) throw new Error('App resource identity is missing or ambiguous');
  const resource = matches[0];
  if (resource.mimeType !== MCP_APP_MIME) throw new Error(`App resource must use ${MCP_APP_MIME}`);
  let html = resource.text;
  if (typeof html !== 'string' && typeof resource.blob === 'string') {
    if (resource.blob.length > MAX_APP_BYTES * 1.4) throw new Error('App resource exceeds budget');
    html = new TextDecoder('utf-8', { fatal: true }).decode(Uint8Array.from(atob(resource.blob), value => value.charCodeAt(0)));
  }
  if (typeof html !== 'string' || !html.trim() || new TextEncoder().encode(html).length > MAX_APP_BYTES) throw new Error('App resource HTML is missing or exceeds budget');
  return { html, csp: normalizeAppCsp(resource._meta?.ui?.csp ?? {}) };
}
export function buildStandardMcpDocument(resource) {
  const { html, csp } = readStandardAppResource({ contents: [resource] }, resource?.uri);
  const resources = (csp.resourceDomains || []).join(' ');
  const policy = ["default-src 'none'", `script-src 'self' 'unsafe-inline' ${resources}`,
    `style-src 'self' 'unsafe-inline' ${resources}`, `img-src 'self' data: ${resources}`, `media-src 'self' data: ${resources}`,
    `font-src 'self' ${resources}`, `connect-src ${(csp.connectDomains || []).join(' ') || "'none'"}`,
    `frame-src ${(csp.frameDomains || []).join(' ') || "'none'"}`, `base-uri ${(csp.baseUriDomains || []).join(' ') || "'self'"}`,
    "object-src 'none'", "form-action 'none'"].join('; ');
  const escaped = policy.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
  return `<!doctype html><meta http-equiv="Content-Security-Policy" content="${escaped}">${html}`;
}

export function resolveSandboxUrl(value, hostOrigin = globalThis.location?.origin) {
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.origin === hostOrigin || url.username || url.password || url.search || url.hash
    || (url.protocol === 'http:' && !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname))) throw new Error('MCP Apps requires a sandbox proxy on a separate HTTPS origin (HTTP loopback is allowed for development)');
  url.hash = new URLSearchParams({ hostOrigin }).toString();
  return url.href;
}
