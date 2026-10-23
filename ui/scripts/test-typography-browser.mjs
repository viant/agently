import assert from 'node:assert/strict';
import {realpathSync, readFileSync, existsSync, mkdirSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
import {chromium} from 'playwright';

const root = fileURLToPath(new URL('..', import.meta.url));
const dependencies = realpathSync(fileURLToPath(new URL('../node_modules', import.meta.url)));
const cacheDir = fileURLToPath(new URL('../.vite/typography-proof', import.meta.url));
const core = path.resolve(root, '../../agently-core');
const forge = path.resolve(root, '../../forge');
const proofDirectory = process.env.AGENTLY_TYPOGRAPHY_WORKSPACE_PROOF;
const readProof = name => readFileSync(path.join(proofDirectory, name));
const revision = 'a'.repeat(64);
const metadata = proofDirectory ? JSON.parse(readProof('metadata.json')) : {workspaceId:'typography-proof', uiStyles:{version:1,revision,themeRevision:revision,href:`/v1/workspace/ui/styles/${revision}.css`}, uiThemes:{version:1,revision,href:`/v1/workspace/ui/themes/${revision}.json`}};
const catalog = JSON.parse(proofDirectory ? readProof('catalog.json') : readFileSync(path.join(core,'protocol/ui/theme/testdata/baseline.json')));
const css = proofDirectory ? readProof('workspace.css') : readFileSync(path.join(core,'protocol/ui/theme/testdata/baseline.css'));
const pilot = process.env.AGENTLY_TYPOGRAPHY_PILOT_JSON ? JSON.parse(readFileSync(process.env.AGENTLY_TYPOGRAPHY_PILOT_JSON)) : null;
const server = await createServer({
  configFile: false,
  root,
  cacheDir,
  resolve: {alias: {forge: path.join(forge,'src'), react: path.join(dependencies,'react'), 'react-dom':path.join(dependencies,'react-dom'), '@preact/signals-react':path.join(dependencies,'@preact/signals-react')}},
  plugins: [{name:'typography-proof-fixture',
    resolveId(source) { if (source === '/__typography/fixture.js') return '\0typography-fixture'; },
    load(id) { if (id === '\0typography-fixture') return `export default ${JSON.stringify({metadata,pilot})}`; },
    configureServer(server) {
    server.middlewares.use((req,res,next) => {
      if (req.url === '/__typography/fixture.js') {res.setHeader('Content-Type','text/javascript');res.end(`export default ${JSON.stringify({metadata,pilot})}`);return;}
      if (req.url === metadata.uiStyles.href) {res.setHeader('Content-Type','text/css');res.end(css);return;}
      if (req.url === metadata.uiThemes.href) {res.setHeader('Content-Type','application/json');res.end(JSON.stringify(catalog));return;}
      if (proofDirectory && /^\/v1\/workspace\/ui\/fonts\/[a-f0-9]{64}\.woff2$/.test(req.url || '')) {
        const file=path.join(proofDirectory,req.url);
        if (existsSync(file)) {res.setHeader('Content-Type','font/woff2');res.end(readFileSync(file));return;}
      }
      next();
    });
  }}],
  optimizeDeps: {entries: ['typography-proof.html']},
  server: {host: '127.0.0.1', port: 0, fs: {allow: [root, dependencies, forge, core]}},
});
let browser;

async function auditViewport(baseURL, viewport) {
  const page = await browser.newPage({viewport, ...(viewport.width<500 ? {hasTouch:true,isMobile:true} : {})});
  await page.goto(`${baseURL}/typography-proof.html`);
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
    else assert.match(item.family, /Georgia/i, `${item.tag} must use the registered workspace-primary family`);
  }
  await page.close();
}

try {
  await server.listen();
  const address = server.httpServer.address();
  const baseURL = `http://127.0.0.1:${address.port}`;
  browser = await chromium.launch({headless: true, channel: process.env.PLAYWRIGHT_CHANNEL || 'chromium'});
  await auditViewport(baseURL, {width: 1440, height: 900});
  await auditViewport(baseURL, {width: 390, height: 844});
  for (const viewport of [{width:1440,height:900},{width:390,height:844}]) {
    const page = await browser.newPage({viewport, ...(viewport.width<500 ? {hasTouch:true,isMobile:true} : {})});
    const failures = [];
    page.on('pageerror', error => failures.push(error.message));
    await page.goto(`${baseURL}/typography-components-proof.html`);
    const activate = locator => viewport.width < 500 ? locator.tap() : locator.click();
    await page.locator('.app-ui-settings-title[data-forge-type="section"]').waitFor();
    if (pilot?.themeId) await page.getByRole('combobox',{name:'Workspace theme',exact:true}).selectOption(pilot.themeId);
    await page.evaluate(() => document.fonts.ready);
    const size = selector => page.locator(selector).first().evaluate(node => getComputedStyle(node).fontSize);
    const selected = await page.getByRole('combobox',{name:'Workspace theme',exact:true}).inputValue();
    const roleTokens = catalog.themes.find(theme => theme.id === selected).modes.light;
    const resolvedColor = value => page.evaluate(color => {const node = document.createElement('span'); node.style.color = color; document.body.appendChild(node); const result = getComputedStyle(node).color; node.remove(); return result;}, value);
    for (const status of ['info', 'success', 'warning', 'danger']) {
      const callout = page.locator(`.bp6-callout[data-forge-status="${status}"]`);
      const expected = roleTokens[`status.${status}.background`] || await callout.evaluate(node => getComputedStyle(node.closest('.agently-workspace')).getPropertyValue(`--forge-status-${node.dataset.forgeStatus}-background`).trim());
      assert.equal(await callout.evaluate(node => getComputedStyle(node).backgroundColor), await resolvedColor(expected));
    }
    const refresh = page.getByRole('button', {name: 'Reload workspace appearance', exact: true});
    assert.equal(await refresh.getAttribute('data-forge-variant'), 'ghost');
    assert.equal(await refresh.evaluate(node => getComputedStyle(node).fontSize), `${roleTokens['typography.label.size']}px`);
    await activate(refresh);
    assert.equal(await page.evaluate(() => window.__foundationRefresh), 1);
    assert.equal(await size('.app-ui-settings-title'), `${roleTokens['typography.section.size']}px`);
    assert.equal(await size('.forge-type-specimen [data-forge-type="section"]'), `${roleTokens['typography.section.size']}px`);
    for (const check of pilot?.elements || []) {
      assert.equal(await size(check.selector), `${roleTokens[`typography.${check.role}.size`]}px`);
      const actualType = await page.locator(check.selector).evaluate(node => {const css=getComputedStyle(node); return {weight:css.fontWeight,leading:css.lineHeight,tracking:css.letterSpacing};});
      assert.equal(actualType.weight, `${roleTokens[`typography.${check.role}.weight`]}`);
      assert.ok(Math.abs(parseFloat(actualType.leading)-roleTokens[`typography.${check.role}.lineHeight`])<0.01, 'Pilot role leading must inherit the catalog');
      assert.ok(Math.abs((parseFloat(actualType.tracking)||0)-roleTokens[`typography.${check.role}.letterSpacing`]*roleTokens[`typography.${check.role}.size`])<0.01, 'Pilot role tracking must inherit the catalog');
      if (pilot.expectedFont) assert.ok((await page.locator(check.selector).evaluate(node => getComputedStyle(node).fontFamily)).includes(pilot.expectedFont));
    }
    for (const check of pilot?.nonOverlap || []) {
      const first = await page.locator(check.first).boundingBox();
      const second = await page.locator(check.second).boundingBox();
      assert.ok(first.y + first.height <= second.y, 'Pilot content must not overlap');
    }
    if (pilot?.expectedFont) assert.equal(await page.evaluate(family => [...document.fonts].some(face => face.family.includes(family) && face.status === 'loaded'), pilot.expectedFont), true);
    for (const check of pilot?.actions || []) {
      const action = page.locator('[data-foundation-pilot]').getByRole('button', {name: check.name, exact: true});
      assert.equal(await action.getAttribute('data-forge-variant'), check.variant);
      assert.equal(await action.locator('svg.forge-icon').count(), 1);
      await activate(action);
      assert.equal(await page.evaluate(() => window.__foundationActions?.at(-1)), check.handler);
      assert.equal(await action.textContent(), check.name, 'Action label must render once');
      assert.equal(await action.evaluate(node => getComputedStyle(node, '::after').content), 'none', 'Migrated actions must not depend on generated labels');
    }
    if (pilot?.iconStyle) {
      for (const example of pilot.iconExamples) {
        const svg = page.locator(`.forge-foundation-icon-example svg.lucide-${example.name.slice(7)}`);
        assert.equal(await svg.getAttribute('viewBox'), '0 0 24 24');
        const actual = await svg.evaluate(node => {const css = getComputedStyle(node); return {size: parseFloat(css.width), strokeWidth: parseFloat(css.strokeWidth)};});
        assert.deepEqual(actual, pilot.iconStyle);
        assert.ok((await svg.getAttribute('class')).includes(example.name.slice(7)));
      }
    }
    if (viewport.width < 500) assert.equal(await page.evaluate(() => matchMedia('(pointer: coarse)').matches), true, 'Touch checks must retain a coarse pointer');
    for (const [index, table] of (pilot?.tables || []).entries()) {
      const scope = page.locator('[data-foundation-table-pilot]').nth(index);
      const rendered = scope.locator('.forge-document-table');
      const header = rendered.locator('th').first();
      const headerStyle = await header.evaluate(node => {const css = getComputedStyle(node); return {weight: css.fontWeight, transform: css.textTransform, leftBorder: css.borderLeftWidth, size: css.fontSize};});
      for (const check of table.checks.referenceStyles || []) {
        const actual = await rendered.locator(check.selector).first().evaluate((node, properties) => Object.fromEntries(properties.map(property => [property, getComputedStyle(node)[property]])), Object.keys(check.styles));
        const expected={...check.styles};
        if (viewport.width<500 && 'minHeight' in expected) expected.minHeight='44px';
        assert.deepEqual(actual, expected, `Reviewed reference styles for ${check.selector}`);
      }
      assert.equal(headerStyle.weight, table.checks.headerWeight);
      assert.equal(headerStyle.transform, 'none');
      assert.equal(headerStyle.leftBorder, '0px');
      assert.equal(headerStyle.size, `${table.checks.headerSize || roleTokens['typography.metadata.size']}px`);
      const bodyOverride=await page.addStyleTag({content:`.agently-workspace[data-forge-theme="${selected}"][data-forge-color-mode="light"] .forge-document-table-wrap {--forge-type-body-size:14px;--forge-type-body-weight:500;--forge-type-body-line-height:22px;--forge-type-body-letter-spacing:0.02em;--forge-type-body-family:monospace;}`});
      for (const actual of await rendered.locator('[data-column-align="right"] input, td[data-read-only] span').evaluateAll(nodes=>nodes.map(node=>{const css=getComputedStyle(node);return [css.fontSize,css.fontWeight,css.lineHeight,css.letterSpacing,css.fontFamily];}))) {
        assert.deepEqual(actual.slice(0,4),['14px','500','22px','0.28px']);
        assert.match(actual[4],/monospace/);
      }
      await bodyOverride.evaluate(node=>node.remove());
      assert.equal(await rendered.locator('.forge-table-row-details').count(), 0);
      const wrap = scope.locator('.forge-document-table-wrap');
      const details = scope.locator(`td[data-column-key="${table.checks.detailsField}"] span`).first();
      const multiline = rendered.locator('td:first-child textarea').first();
      assert.equal(await multiline.evaluate(node => node.style.height), 'auto', 'Native content sizing follows theme/font loading');
      assert.ok(await multiline.evaluate(node => node.scrollHeight <= node.clientHeight + 1), 'Long names remain fully visible');
      assert.equal(await details.textContent(), String(table.rows[0][table.checks.detailsField]));
      assert.equal(await rendered.getByRole('columnheader', {name: table.container.columns.find(column=>column.key===table.checks.detailsField).label,exact:true}).count(),1);
      assert.equal(await rendered.locator('thead svg').count(),0);
      if (process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS) {
        mkdirSync(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,{recursive:true});
        await scope.screenshot({path:path.join(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,`table-${viewport.width}.png`)});
        await page.locator('#foundation-actions').screenshot({path:path.join(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,`icons-${viewport.width}.png`)});
      }
      const quantity=rendered.locator('[data-column-align="right"] input').first();
      const originalQuantity=await quantity.inputValue();
      if (viewport.width<500) {
        await quantity.tap();
        assert.equal(await quantity.evaluate(node=>node===document.activeElement),true);
        assert.ok((await quantity.boundingBox()).height>=44);
        await quantity.press('Escape');
      }

      if (viewport.width>1000) {
        await quantity.hover();
        assert.notEqual(await quantity.evaluate(node=>getComputedStyle(node).borderTopColor),'rgba(0, 0, 0, 0)');
        assert.equal(await multiline.evaluate(node=>getComputedStyle(node).borderTopColor),'rgba(0, 0, 0, 0)');
      }
      assert.equal(await details.locator('input,select,textarea').count(),0);
      const nameColumn = table.container.columns.find(column => column.key === table.checks.nameKey);
      const name = scope.getByRole('textbox', {name: `${nameColumn.label} row 1`, exact: true});
      await name.fill('Edited example name');
      const numberColumn = table.container.columns.find(column => column.key === table.checks.numberKey);
      await scope.getByRole('spinbutton', {name: `${numberColumn.label} row 1`, exact: true}).fill('9876');
      assert.equal(await page.evaluate(key => window.__foundationTableRows[0][key], table.checks.nameKey), 'Edited example name');
      assert.equal(await page.evaluate(key => window.__foundationTableRows[0][key], table.checks.numberKey), 9876);
      const numeric=scope.getByRole('spinbutton', {name: `${numberColumn.label} row 1`, exact:true});
      await numeric.press('Escape');
      assert.equal(await numeric.inputValue(),originalQuantity);
      assert.equal(await name.inputValue(),'Edited example name','Cancel preserves another field edit');
      await numeric.fill('9876');
      await numeric.press('Enter');
      assert.equal(await numeric.evaluate(node=>node===document.activeElement),false);

      assert.equal(await page.evaluate(key => window.__foundationTableRows[0][key], table.checks.detailsField), table.rows[0][table.checks.detailsField]);
      await activate(scope.getByRole('button', {name: `${table.container.removeRowLabel || 'Remove row'} 2`, exact: true}));
      assert.equal(await rendered.locator('tbody > tr').count(), table.rows.length - 1);
      assert.equal((await page.evaluate(() => window.__foundationTableRows)).length, table.rows.length - 1);
    }
    if (process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS) {
      mkdirSync(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,{recursive:true});
      await page.screenshot({path:path.join(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,`foundation-${viewport.width}.png`),fullPage:true});
    }
    const before = await size('.app-ui-settings-title');
    await page.evaluate(content => { document.querySelector('style[data-workspace-style]').textContent += content; }, `.agently-application[data-agently-theme="${selected}"][data-agently-color-mode="light"] {--agently-theme-type-section-size:21px;--agently-theme-type-section-weight:400;--agently-theme-type-section-line-height:30px;--agently-theme-type-section-letter-spacing:0.02em;} .agently-workspace[data-forge-theme="${selected}"][data-forge-color-mode="light"] {--forge-type-section-size:21px;--forge-type-section-weight:400;--forge-type-section-line-height:30px;--forge-type-section-letter-spacing:0.02em;}`);
    assert.notEqual(before, '21px');
    await page.waitForFunction(() => getComputedStyle(document.querySelector('.app-ui-settings-title')).fontSize === '21px');
    for (const selector of ['.app-ui-settings-title', '.forge-type-specimen [data-forge-type="section"]']) {
      const actual=await page.locator(selector).evaluate(node=>{const css=getComputedStyle(node);return [css.fontSize,css.fontWeight,css.lineHeight,css.letterSpacing];});
      assert.deepEqual(actual,['21px','400','30px','0.42px']);
    }
    await page.evaluate(content => { document.querySelector('style[data-workspace-style]').textContent += content; }, `.agently-application[data-agently-theme="${selected}"][data-agently-color-mode="light"] {--agently-theme-type-metadata-size:13px;--agently-theme-type-metadata-weight:600;--agently-theme-type-metadata-line-height:18px;--agently-theme-type-metadata-letter-spacing:0.01em;--agently-theme-type-metadata-family:monospace;} .agently-workspace[data-forge-theme="${selected}"][data-forge-color-mode="light"] {--forge-type-metadata-size:13px;--forge-type-metadata-weight:600;--forge-type-metadata-line-height:18px;--forge-type-metadata-letter-spacing:0.01em;--forge-type-metadata-family:monospace;}`);
    for (const selector of ['.app-workspace-appearance-controls [data-forge-type="metadata"]', '.forge-type-specimen [data-forge-type="metadata"]', ...(pilot?.elements || []).filter(check => check.role === 'metadata').map(check => check.selector)]) {
      const consumers=await page.locator(selector).evaluateAll(nodes=>nodes.map(node=>{const css=getComputedStyle(node);return [css.fontSize,css.fontWeight,css.lineHeight,css.letterSpacing,css.fontFamily];}));
      assert.ok(consumers.length>0);
      for (const actual of consumers) {
        assert.deepEqual(actual.slice(0,4),['13px','600','18px','0.13px']);
        assert.match(actual[4],/monospace/);
      }
    }
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),true);
    assert.deepEqual(failures,[]);
    await page.close();
  }
  console.log('shared component and workspace role propagation passed');
  console.log('typography browser proof passed: desktop, narrow, controls, portal, and approved monospace');
} finally {
  await browser?.close();
  await server.close();
}
