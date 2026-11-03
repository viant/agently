// Isolated synthetic data; no application server or conversation execution.
import assert from 'node:assert/strict';
import {readFileSync, mkdtempSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer, transformWithEsbuild} from 'vite';
import {chromium} from 'playwright';
import viteConfig from '../vite.config.js';

const root = fileURLToPath(new URL('..', import.meta.url));
const core = path.resolve(root, '../../agently-core');
const revision = 'a'.repeat(64);
const metadata = {workspaceId: 'audit-proof', uiStyles: {version: 1, revision, href: '/__audit/theme.css'}, uiThemes: {version: 1, revision, href: '/__audit/themes.json'}};
const row = {id: 'synthetic-turn', conversationId: 'synthetic-conversation', conversationTitle: 'Example conversation', userId: 'example-user', status: 'completed', createdAt: '2026-10-29T10:00:00Z',
  conversationCreatedAt: '2026-10-28T09:00:00Z',
  agentIdUsed: 'example-agent', models: ['alpha/chat', 'zeta/reasoning'], inputTokens: 42, outputTokens: 63, executionSpanMs: 62000,
  input: `Synthetic input, with Unicode: żółć\n${'Full input text. '.repeat(40)}INPUT END`,
  output: `Synthetic final response\n${'Full output text. '.repeat(40)}OUTPUT END`,
  inputPreview: 'Obsolete input preview', outputPreview: 'Obsolete output preview', feedbackRating: -1, feedbackComment: 'Needs correction', modelCalls: 3, toolCalls: 0};
const tableHeaders = ['Turn time', 'Turn ID', 'Conversation ID', 'Conversation', 'Input', 'Output', 'User', 'Agent', 'Status', 'Models',
  'Input tokens', 'Output tokens', 'Turn duration', 'Feedback', 'Feedback comment',
  'Model calls', 'Tool calls', 'Conversation created at'];
const csvColumns = ['turn_created_at', 'turn_id', 'conversation_id', 'conversation_title', 'input', 'output', 'user_id', 'agent_id', 'status', 'models',
  'input_tokens', 'output_tokens', 'execution_span_ms', 'feedback_rating', 'feedback_comment',
  'model_calls', 'tool_calls', 'conversation_created_at'];
const csvValues = [row.createdAt, row.id, row.conversationId, row.conversationTitle, row.input, row.output, row.userId, row.agentIdUsed, row.status,
  row.models.join(', '), row.inputTokens, row.outputTokens, row.executionSpanMs,
  row.feedbackRating, row.feedbackComment, row.modelCalls, row.toolCalls, row.conversationCreatedAt];
const fixtureCSV = `${csvColumns.join(',')}\n${csvValues.map(value => `"${String(value).replaceAll('"', '""')}"`).join(',')}\n`;
const jsonRecord = Object.fromEntries(csvColumns.map((key, index) => [key, key === 'models' ? row.models : csvValues[index]]));
const fixtureJSON = JSON.stringify([jsonRecord]);
let enabled = true;
let granted = true;
let admin = false;
let paginated = false;
const reads = [];
const exports = [];
const html = '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="root"></div><script type="module" src="/__audit/fixture.jsx"></script></body></html>';
const source = `
import React from 'react';
import {createRoot} from 'react-dom/client';
import {SettingProvider, addWindow} from 'forge/core';
import AppSectionContent from '/src/components/AppSectionContent.jsx';
import WorkspaceStyleProvider from '/src/components/WorkspaceStyleProvider.jsx';
import '/src/index.css';
import '/src/typography.js';
import '/src/components/sectionNavigation.css';
const entry = addWindow('Conversation audit',null,'conversation-audit','',true,{}, {region:'app.section',inlineMetadata:{windowKey:'conversation-audit',dataSource:{},view:{content:{id:'audit'}}}});
createRoot(document.getElementById('root')).render(<WorkspaceStyleProvider metadata={${JSON.stringify(metadata)}} accountKey="synthetic" workspaceKey="audit-proof"><SettingProvider services={{}} connectorConfig={{}}><div style={{height:'100dvh',display:'flex',flexDirection:'column',overflow:'hidden'}}><main className="app-chat-pane"><AppSectionContent window={entry} title="Conversation audit"/></main></div></SettingProvider></WorkspaceStyleProvider>);`;
const config = viteConfig({command: 'serve', mode: 'test'});
const server = await createServer({...config, configFile: false, root, logLevel: 'error',
  server: {...config.server, host: '127.0.0.1', port: 0},
  plugins: [...config.plugins, {
    name: 'synthetic-audit-proof',
    resolveId(id) {if (id === '/__audit/fixture.jsx') return '\0audit-proof.jsx';},
    async load(id) {
      if (id === '\0audit-proof.jsx') return (await transformWithEsbuild(source, 'audit-proof.jsx', {loader: 'jsx', jsx: 'automatic'})).code;
    },
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        if (req.url === '/__audit') {res.setHeader('Content-Type', 'text/html'); res.end(await server.transformIndexHtml('/__audit', html)); return;}
        if (req.url === '/__audit/theme.css') {res.setHeader('Content-Type', 'text/css'); res.end(readFileSync(path.join(core, 'protocol/ui/theme/testdata/baseline.css'))); return;}
        if (req.url === '/__audit/themes.json') {res.setHeader('Content-Type', 'application/json'); res.end(readFileSync(path.join(core, 'protocol/ui/theme/testdata/baseline.json'))); return;}
        if (!req.url?.startsWith('/v1/api/conversation-audit/')) return next();
        reads.push(req.url);
        if (req.url.endsWith('/export') || req.url.endsWith('/export/json')) {
          let input = '';
          for await (const chunk of req) input += chunk;
          exports.push(JSON.parse(input));
          const json = req.url.endsWith('/json');
          res.setHeader('Content-Type', json ? 'application/json' : 'text/csv; charset=utf-8');
          res.setHeader('Content-Disposition', `attachment; filename="conversation-audit.${json ? 'json' : 'csv'}"`);
          res.end(json ? fixtureJSON : fixtureCSV); return;
        }
        let body;
        if (req.url.endsWith('/capabilities')) body = {uiEnabled: enabled, canAudit: granted, currentUserId: 'example-user', canAuditAllUsers: admin};
        else if (req.url.includes('/turns?')) {
          const query = new URL(req.url, 'http://localhost').searchParams;
          const cursor = query.get('cursor');
          const pageNumber = cursor === 'cursor-3' ? 3 : cursor === 'cursor-2' ? 2 : 1;
          const size = Number(query.get('limit'));
          body = paginated ? {
            rows: Array.from({length: Math.min(size, 21 - (pageNumber - 1) * size)}, (_, i) => ({...row, id: `page-${pageNumber}-turn-${i}`})),
            ...(pageNumber * size < 21 ? {nextCursor: `cursor-${pageNumber + 1}`} : {}),
          } : {rows: [row]};
          if (query.get('includeTotal') === 'true') body.totalCount = paginated ? 21 : 1;
        }
        else {res.statusCode = 404; body = {error: 'Unexpected audit route'};}
        res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(body));
      });
    },
  }],
});
let browser;
const output = process.env.AGENTLY_AUDIT_PROOF_DIR || mkdtempSync(path.join(tmpdir(), 'agently-audit-proof-'));
try {
  await server.listen();
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome'});
  for (const width of [1440, 390]) {
    paginated = false;
    const page = await browser.newPage({viewport: {width, height: 900}});
    page.setDefaultTimeout(60000);
    const failures = [];
    page.on('pageerror', e => {failures.push(e.message); console.error(e.stack);});
    const before = reads.length;
    // Cold transforms of the shared Forge sources are much slower than a built UI.
    await page.goto(`${base}/__audit`, {timeout: 240000});
    await page.getByRole('button', {name: 'Search turns', exact: true}).waitFor();
    assert.equal(await page.getByRole('textbox', {name: 'User ID', exact: true}).isEditable(), false);
    assert.equal(await page.getByRole('button', {name: 'Users', exact: true}).count(), 0);
    assert.equal(reads.length, before + 1, 'opening the window must only read capabilities');
    await page.locator('.conversation-audit input').nth(2).click();
    const calendar = page.locator('.conversation-audit .rdp');
    await calendar.waitFor();
    assert.equal(await calendar.evaluate(el => getComputedStyle(el).getPropertyValue('--rdp-cell-size').trim()), '30px');
    assert.equal(await page.evaluate(() => {
      const outside = document.createElement('div'); outside.className = 'rdp'; document.body.append(outside);
      const size = getComputedStyle(outside).getPropertyValue('--rdp-cell-size').trim(); outside.remove(); return size;
    }), '', 'audit calendar styles must not affect other windows');
    await page.screenshot({path: path.join(output, `audit-calendar-${width}.png`), fullPage: true, animations: 'disabled'});
    await page.getByRole('heading', {name: 'Conversation audit', exact: true}).click();
    await page.getByRole('button', {name: 'Search turns', exact: true}).click();
    await page.getByText(/1 turns on this page/).waitFor();
    assert.equal(await page.getByRole('button', {name: 'Export CSV'}).isEnabled(), true);
    const table = page.locator('.conversation-audit-table');
    await table.getByText('Conversation created at', {exact: true}).waitFor();
    assert.deepEqual(await table.locator('thead .forge-table-header-label').allTextContents(), tableHeaders);
    const cells = await table.locator('tbody tr').first().locator('td').allTextContents();
    assert.ok(cells[0].trim(), 'turn time must be visible');
    assert.ok(cells.at(-1).trim(), 'conversation creation time must be visible');
    assert.notEqual(cells[0], cells.at(-1), 'conversation creation must not repeat turn time');
    for (const text of [row.id, row.conversationId, row.agentIdUsed, 'alpha/chat, zeta/reasoning', '1 min 2 s']) {
      assert.equal(await table.getByText(text, {exact: true}).count(), 1);
    }
    assert.equal(await page.getByRole('combobox', {name: 'Turn to inspect'}).count(), 0);
    assert.equal(await page.getByRole('button', {name: 'View turn'}).count(), 0);
    assert.equal(await page.getByRole('region', {name: 'Saved turn details'}).count(), 0);
    await table.getByText(row.id, {exact: true}).click();
    assert.equal(reads.length, before + 2, 'clicking a table row must not read turn details or payloads');
    assert.equal(await table.getByText(/Obsolete/).count(), 0);
    const beforeContent = reads.length;
    for (const [label, content] of [['Input', row.input], ['Output', row.output]]) {
      const expand = table.getByRole('button', {name: `Show full ${label} content`, exact: true});
      await expand.scrollIntoViewIfNeeded();
      await expand.click();
      const dialog = page.getByRole('dialog', {name: 'Full Content'});
      await dialog.waitFor();
      assert.equal(await dialog.locator('p').textContent(), content, 'dialog must retain the full text, including line breaks');
      const bounds = await dialog.boundingBox();
      assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= width + 1, 'full content must fit the viewport');
      await page.screenshot({path: path.join(output, `audit-${label.toLowerCase()}-${width}.png`), animations: 'disabled'});
      await page.keyboard.press('Escape');
      await dialog.waitFor({state: 'hidden'});
    }
    assert.equal(reads.length, beforeContent, 'opening full text must not fetch each turn again');
    assert.equal(await page.locator('.conversation-audit-table').getByText('Needs correction', {exact: true}).count(), 1);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'table overflow must stay inside the audit pane');
    const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', {name: 'Export CSV'}).click()]);
    assert.equal(download.suggestedFilename(), 'conversation-audit.csv');
    assert.equal(readFileSync(await download.path(), 'utf8'), fixtureCSV);
    const list = new URL(reads.slice(before).find(url => url.includes('/turns?')), base).searchParams;
    assert.equal(list.get('includeContent'), 'true');
    assert.deepEqual(exports.at(-1), {userId: 'example-user', from: list.get('from'), to: list.get('to')});
    assert.equal(reads.length, before + 3, 'the window only reads capabilities, the turn list and CSV');
    assert.equal(list.get('includeTotal'), 'true');
    assert.equal(list.get('limit'), '10');
    assert.equal(await table.locator('.toolbar-center').getByText('Page 1 of 1', {exact: true}).count(), 1);
    assert.equal(await page.getByRole('combobox', {name: 'Rows per page'}).inputValue(), '10');
    const pageStatus = table.locator('.toolbar-center').getByText('Page 1 of 1', {exact: true});
    assert.equal(await pageStatus.evaluate(el => el.getClientRects().length === 1 && el.clientHeight < 40), true,
      'the page label must remain on one line');
    const sizeBounds = await page.getByRole('combobox', {name: 'Rows per page'}).boundingBox();
    assert.ok(sizeBounds.width >= 50, 'the page size value must remain readable on a narrow screen');
    const [jsonDownload] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', {name: 'Export JSON'}).click()]);
    assert.equal(jsonDownload.suggestedFilename(), 'conversation-audit.json');
    assert.deepEqual(JSON.parse(readFileSync(await jsonDownload.path(), 'utf8')), [jsonRecord]);
    assert.deepEqual(exports.at(-1), {userId: 'example-user', from: list.get('from'), to: list.get('to')});
    await page.screenshot({path: path.join(output, `audit-${width}.png`), fullPage: true});
    paginated = true;
    const pagingStart = reads.length;
    await page.getByRole('button', {name: 'Refresh', exact: true}).click();
    await table.getByText('page-1-turn-0', {exact: true}).waitFor();
    for (const [button, pageNumber] of [['Next Page', 2], ['Next Page', 3], ['Previous Page', 2], ['Next Page', 3], ['First Page', 1]]) {
      await page.getByRole('button', {name: button, exact: true}).click();
      await table.getByText(`page-${pageNumber}-turn-0`, {exact: true}).waitFor();
      await table.locator('.toolbar-center').getByText(`Page ${pageNumber} of 3`, {exact: true}).waitFor();
    }
    assert.deepEqual(reads.slice(pagingStart).map(url => new URL(url, base).searchParams.get('cursor')),
      [null, 'cursor-2', 'cursor-3', 'cursor-2', 'cursor-3', null]);
    assert.deepEqual(reads.slice(pagingStart).map(url => new URL(url, base).searchParams.get('includeTotal')),
      ['true', null, null, null, null, null]);
    assert.equal(await page.getByRole('button', {name: 'Last Page', exact: true}).isDisabled(), true);
    await page.getByRole('combobox', {name: 'Rows per page'}).selectOption('20');
    await table.locator('.toolbar-center').getByText('Page 1 of 2', {exact: true}).waitFor();
    await page.getByRole('button', {name: 'Next Page', exact: true}).click();
    await table.locator('.toolbar-center').getByText('Page 2 of 2', {exact: true}).waitFor();
    const resized = reads.slice(-2).map(url => new URL(url, base).searchParams);
    assert.deepEqual(resized.map(query => query.get('limit')), ['20', '20']);
    assert.deepEqual(resized.map(query => query.get('cursor')), [null, 'cursor-2']);
    await page.getByRole('heading', {name: 'Conversation audit', exact: true}).scrollIntoViewIfNeeded();
    await page.screenshot({path: path.join(output, `audit-pagination-${width}.png`), fullPage: true});
    assert.deepEqual(failures, []);
    await page.close();
    admin = true;
    paginated = false;
    const adminPage = await browser.newPage({viewport: {width, height: 900}});
    adminPage.setDefaultTimeout(60000);
    adminPage.on('pageerror', e => failures.push(e.message));
    await adminPage.goto(`${base}/__audit`, {timeout: 120000});
    const owner = adminPage.getByRole('textbox', {name: 'User ID', exact: true});
    await owner.waitFor();
    assert.equal(await owner.isEditable(), false);
    assert.equal(await owner.inputValue(), 'example-user');
    assert.equal(await adminPage.getByRole('checkbox').count(), 0);
    const users = adminPage.getByRole('button', {name: 'Users', exact: true});
    assert.equal(await users.textContent(), 'My conversations');
    const chooseUsers = async label => {
      await users.click();
      await adminPage.getByText(label, {exact: true}).click();
    };
    const searchAdmin = async () => {
      const [result] = await Promise.all([
        adminPage.waitForResponse(response => response.url().includes('/api/conversation-audit/turns?')),
        adminPage.getByRole('button', {name: 'Search turns', exact: true}).click(),
      ]);
      await adminPage.waitForFunction(() => !document.querySelector('.conversation-audit-actions button:last-of-type').disabled);
      return new URL(result.url()).searchParams;
    };
    let adminQuery = await searchAdmin();
    assert.equal(adminQuery.get('userId'), 'example-user');
    const originalDates = [adminQuery.get('from'), adminQuery.get('to')];
    await users.press('Enter');
    await adminPage.getByText('User ID contains', {exact: true}).waitFor();
    await adminPage.keyboard.press('Escape');
    await adminPage.getByText('User ID contains', {exact: true}).waitFor({state: 'hidden'});
    await chooseUsers('User ID contains');
    assert.equal(await owner.isEditable(), true);
    await owner.fill('Team_%*');
    assert.equal(await adminPage.getByRole('button', {name: 'Export CSV'}).isDisabled(), true);
    assert.equal(await adminPage.getByRole('button', {name: 'Export JSON'}).isDisabled(), true);
    adminQuery = await searchAdmin();
    assert.equal(adminQuery.get('userIdContains'), 'Team_%*');
    assert.equal(adminQuery.has('userId'), false);
    for (const format of ['CSV', 'JSON']) {
      await Promise.all([adminPage.waitForEvent('download'), adminPage.getByRole('button', {name: `Export ${format}`}).click()]);
      assert.equal(exports.at(-1).userIdContains, 'Team_%*');
      assert.equal(Object.hasOwn(exports.at(-1), 'userId'), false);
    }
    assert.deepEqual([adminQuery.get('from'), adminQuery.get('to')], originalDates);
    await adminPage.screenshot({path: path.join(output, `audit-admin-${width}.png`), fullPage: true});
    await chooseUsers('Exact user ID');
    assert.equal(await owner.inputValue(), 'Team_%*');
    adminQuery = await searchAdmin();
    assert.equal(adminQuery.get('userId'), 'Team_%*');
    assert.equal(adminQuery.has('userIdContains'), false);
    await owner.fill('  ');
    const blankBefore = reads.length;
    await adminPage.getByRole('button', {name: 'Search turns', exact: true}).click();
    await adminPage.getByRole('alert').filter({hasText: 'Enter a user ID or select All users.'}).waitFor();
    assert.equal(reads.length, blankBefore);
    await owner.fill('Team_%*');
    await chooseUsers('All users');
    assert.equal(await owner.isDisabled(), true);
    assert.equal(await owner.inputValue(), '');
    assert.equal(await owner.getAttribute('placeholder'), 'All users');
    assert.equal(await adminPage.getByRole('button', {name: 'Export CSV'}).isDisabled(), true);
    adminQuery = await searchAdmin();
    assert.equal(adminQuery.has('userId'), false);
    assert.equal(adminQuery.has('userIdContains'), false);
    assert.deepEqual([adminQuery.get('from'), adminQuery.get('to')], originalDates);
    for (const format of ['CSV', 'JSON']) {
      await Promise.all([adminPage.waitForEvent('download'), adminPage.getByRole('button', {name: `Export ${format}`}).click()]);
      assert.equal(Object.hasOwn(exports.at(-1), 'userId'), false);
      assert.equal(Object.hasOwn(exports.at(-1), 'userIdContains'), false);
    }
    assert.equal(await adminPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    await adminPage.screenshot({path: path.join(output, `audit-admin-all-${width}.png`), fullPage: true});
    await chooseUsers('My conversations');
    assert.equal(await owner.isEditable(), false);
    assert.equal(await owner.inputValue(), 'example-user');
    adminQuery = await searchAdmin();
    assert.equal(adminQuery.get('userId'), 'example-user');
    assert.equal(adminQuery.has('userIdContains'), false);
    const beforeSwitch = reads.length;
    await chooseUsers('User ID contains');
    assert.equal(await owner.inputValue(), 'Team_%*');
    assert.equal(reads.length, beforeSwitch, 'changing scope must not fetch conversation data');
    assert.deepEqual(failures, []);
    await adminPage.close();
    admin = false;
  }
  granted = false;
  const denied = await browser.newPage();
  const deniedBefore = reads.length;
  await denied.goto(`${base}/__audit`, {timeout: 120000});
  await denied.getByText('You do not have conversation audit access.').waitFor();
  assert.equal(await denied.getByRole('button', {name: 'Search turns'}).count(), 0);
  assert.equal(reads.length, deniedBefore + 1);
  await denied.close();
  granted = true;
  enabled = false;
  const page = await browser.newPage();
  const before = reads.length;
  await page.goto(`${base}/__audit`, {timeout: 120000});
  await page.getByText(/experimental audit window is disabled/).waitFor();
  assert.equal(reads.length, before + 1);
  await page.close();
  console.log(`Audit browser checks passed (1440px, 390px, ordinary/admin/denied, UI disabled). Screenshots: ${output}`);
} finally {
  await browser?.close();
  await server.close();
}
