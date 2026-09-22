import assert from 'node:assert/strict';
import {realpathSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
import {chromium} from 'playwright';

const root = fileURLToPath(new URL('..', import.meta.url));
const dependencies = realpathSync(fileURLToPath(new URL('../node_modules', import.meta.url)));
const server = await createServer({
  configFile: false,
  root,
  optimizeDeps: {entries: ['typography-proof.html']},
  server: {host: '127.0.0.1', port: 0, fs: {allow: [root, dependencies]}},
});
let browser;

async function auditViewport(baseURL, viewport) {
  const page = await browser.newPage({viewport});
  await page.goto(`${baseURL}/typography-proof.html`);
  await page.waitForFunction(() => document.fonts.check('14px "IBM Plex Sans Variable"'));
  const result = await page.evaluate(() => {
    const visible = (element) => {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden';
    };
    return Array.from(document.querySelectorAll('h1, p, button, input, textarea, select, option, code'))
      .filter(visible)
      .map((element) => ({tag: element.tagName.toLowerCase(), family: getComputedStyle(element).fontFamily}));
  });
  for (const item of result) {
    if (item.tag === 'code') assert.match(item.family, /mono/i, `${item.tag} must retain an approved monospace family`);
    else assert.match(item.family, /IBM Plex Sans/i, `${item.tag} must use the registered product-primary family`);
  }
  await page.close();
}

try {
  await server.listen();
  const address = server.httpServer.address();
  const baseURL = `http://127.0.0.1:${address.port}`;
  browser = await chromium.launch({headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome'});
  await auditViewport(baseURL, {width: 1440, height: 900});
  await auditViewport(baseURL, {width: 390, height: 844});
  console.log('typography browser proof passed: desktop, narrow, controls, portal, and approved monospace');
} finally {
  await browser?.close();
  await server.close();
}
