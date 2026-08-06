import { chromium } from 'playwright';

const baseUrl = process.env.AGENTLY_PROBE_BASE_URL || 'http://127.0.0.1:5174';
const secretsURL = process.env.AGENTLY_PROBE_OOB_SECRET_REF || '';
const configURL = process.env.AGENTLY_PROBE_AUTH_CONFIG_REF || '';
if (!secretsURL || !configURL) {
  throw new Error('Set AGENTLY_PROBE_OOB_SECRET_REF and AGENTLY_PROBE_AUTH_CONFIG_REF before running this probe.');
}
const oobBody = {
  secretsURL,
  configURL
};
const authResp = await fetch(`${baseUrl}/v1/api/auth/oob`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
  body: JSON.stringify(oobBody),
});
if (!authResp.ok) {
  throw new Error(`OOB auth failed (${authResp.status}): ${await authResp.text()}`);
}
const setCookie = authResp.headers.get('set-cookie') || '';
const match = setCookie.match(/agently_session=([^;]+)/);
if (!match) {
  throw new Error(`Missing agently_session cookie: ${setCookie}`);
}
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.context().addCookies([{ name: 'agently_session', value: match[1], domain: '127.0.0.1', path: '/' }]);
const consoleEntries = [];
page.on('console', (msg) => consoleEntries.push({ type: msg.type(), text: msg.text() }));
page.on('pageerror', (err) => consoleEntries.push({ type: 'pageerror', text: String(err?.stack || err) }));
await page.goto(`${baseUrl}/mcp-ui/forge-window?windowKey=metricReportBuilder`, { waitUntil: 'networkidle', timeout: 60000 });
const text = await page.locator('body').innerText().catch(() => '');
console.log(JSON.stringify({ text, consoleEntries }, null, 2));
await browser.close();
