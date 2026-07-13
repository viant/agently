import { chromium } from 'playwright';

const baseUrl = 'http://127.0.0.1:5174';
const oobBody = {
  secretsURL: '/Users/awitas/.secret/awitas_dsp_ui.enc|blowfish://default',
  configURL: '/Users/awitas/.secret/idp_viant.enc|blowfish://default'
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
