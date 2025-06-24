// url.js – small helper utilities around URL concatenation

/**
 * joinURL joins base and path ensuring exactly one slash in-between.
 * @param {string} base – base URL (may end with '/')
 * @param {string} path – path segment (may start with '/')
 * @return {string}
 */
export function joinURL(base = '', path = '') {
  return `${(base || '').replace(/\/+$/, '')}/${(path || '').replace(/^\/+/, '')}`;
}
