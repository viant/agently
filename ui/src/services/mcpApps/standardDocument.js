function origins(values) {
  return (Array.isArray(values) ? values : []).map(value => {
    try {
      const parsed = new URL(value);
      return ['https:', 'http:'].includes(parsed.protocol) && !parsed.username && !parsed.password
        && (parsed.pathname === '/' || parsed.pathname === '') && !parsed.search && !parsed.hash ? parsed.origin : '';
    } catch { return ''; }
  }).filter(Boolean).join(' ');
}
export function buildStandardMcpDocument(resource) {
  if (resource?.mimeType?.split(';')[0].trim() !== 'text/html' || typeof resource.text !== 'string') throw new Error('App resource must contain HTML');
  const csp = resource._meta?.ui?.csp || {};
  const resources = origins(csp.resourceDomains);
  const policy = ["default-src 'none'", `script-src 'unsafe-inline' 'unsafe-eval' blob: ${resources}`,
    `style-src 'unsafe-inline' ${resources}`, `img-src data: blob: ${resources}`, `font-src data: ${resources}`,
    `connect-src ${origins(csp.connectDomains) || "'none'"}`, `frame-src ${origins(csp.frameDomains) || "'none'"}`,
    "object-src 'none'", "base-uri 'none'", "form-action 'none'"].join('; ');
  const escaped = policy.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
  return `<!doctype html><meta http-equiv="Content-Security-Policy" content="${escaped}">${resource.text}`;
}
