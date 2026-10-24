import {conversation as outputConversation} from '../src/components/outputHandoffFixture.js';
import {verifyDocumentChat} from './documentChat.browser.mjs';
import assert from 'node:assert/strict';
import {realpathSync, readFileSync, existsSync, mkdirSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {createServer} from 'vite';
import {chromium} from 'playwright';

const root = fileURLToPath(new URL('..', import.meta.url));
const dependencies = realpathSync(fileURLToPath(new URL('../node_modules', import.meta.url)));
const cacheDir = fileURLToPath(new URL('../.vite/typography-proof', import.meta.url));
const core = path.resolve(root, '../../agently-core');
const forge = path.resolve(root, '../../forge');
const {verifyChartFoundation,verifyAuthoredCharts} = await import(pathToFileURL(path.join(forge,'src/demos/theme/chartFoundation.browser.mjs')));
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
  resolve: {alias: {forge: path.join(forge,'src'), 'agently-core-ui-sdk':path.join(core,'sdk/ts/src'), react: path.join(dependencies,'react'), 'react-dom':path.join(dependencies,'react-dom'), '@preact/signals-react':path.join(dependencies,'@preact/signals-react')}},
  plugins: [{name:'typography-proof-fixture',
    resolveId(source) { if (source === '/__typography/fixture.js') return '\0typography-fixture'; },
    load(id) { if (id === '\0typography-fixture') return `export default ${JSON.stringify({metadata,pilot})}`; },
    configureServer(server) {
    server.middlewares.use((req,res,next) => {
      if (['/api/auth/me','/v1/api/auth/me'].includes(req.url)) {res.setHeader('Content-Type','application/json');res.end(JSON.stringify({provider:'fixture',subject:'specimen-reader',sessionBinding:'proof-session',displayName:'Example Reviewer'}));return;}
      if (req.url?.startsWith(`/v1/conversations/${outputConversation.conversationId}/transcript`)) {res.setHeader('Content-Type','application/json');res.end(JSON.stringify({conversation:outputConversation}));return;}
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
    const navigation = page.getByRole('region', {name: 'Section navigation specimen'});
    const rail = navigation.getByRole('navigation', {name: 'App sections'});
    await activate(rail.getByRole('button', {name: 'Workspaces', exact: true}));
    assert.equal(await rail.getByRole('button', {name: 'Workspaces', exact: true}).getAttribute('aria-current'), 'page');
    await navigation.getByRole('heading', {name: 'Workspaces', exact: true}).waitFor();
    const records = navigation.getByRole('button', {name: 'Example records', exact: true});
    await records.click();
    assert.equal(await records.getAttribute('aria-current'), 'page');
    assert.equal(await records.getAttribute('data-forge-type'), 'body');
    assert.equal(await records.evaluate(node => getComputedStyle(node).outlineWidth), '0px');
    assert.equal(await navigation.getByRole('button', {name: 'Overview', exact: true}).getAttribute('aria-current'), null);
    assert.equal(await navigation.getByRole('button', {name: 'Unavailable destination', exact: true}).isEnabled(), false);
    await records.press('Shift+Tab');
    const overview = navigation.getByRole('button', {name: 'Overview', exact: true});
    assert.equal(await overview.evaluate(node => node === document.activeElement && node.matches(':focus-visible')), true);
    assert.equal(await overview.evaluate(node => getComputedStyle(node).outlineWidth), await overview.evaluate(node => getComputedStyle(node).getPropertyValue('--agently-theme-focus-width').trim() || '2px'));
    const menu = navigation.locator('.app-section-menu');
    const menuStyle = await menu.getAttribute('style');
    await menu.evaluate(node => {
      for (const [key, value] of Object.entries({size:'17px',weight:'500','line-height':'24px','letter-spacing':'0.03em',family:'monospace'})) node.style.setProperty(`--forge-type-body-${key}`, value);
    });
    const menuType = await records.evaluate(node => {const css=getComputedStyle(node);return [css.fontSize,css.fontWeight,css.lineHeight,css.letterSpacing,css.fontFamily];});
    assert.deepEqual(menuType.slice(0,4), ['17px','500','24px','0.51px']);
    assert.match(menuType[4], /monospace/);
    await menu.evaluate((node, style) => {if(style === null) node.removeAttribute('style'); else node.setAttribute('style',style);}, menuStyle);
    assert.equal(await rail.getByRole('button', {name: 'Unavailable section', exact: true}).isDisabled(), true);
    await activate(rail.getByRole('button', {name: 'Chat', exact: true}));
    await navigation.getByRole('heading', {name: 'Conversations', exact: true}).waitFor();
    assert.equal(await navigation.locator('.app-section-menu').getAttribute('data-forge-type'), 'body');
    if (viewport.width < 500) assert.ok(await rail.getByRole('button', {name: 'Chat', exact: true}).evaluate(node => node.getBoundingClientRect().height >= 44));
    const viewer = navigation.locator('.app-header-specimen');
    await viewer.getByRole('tab', {name: 'Example output', exact: true}).waitFor();
    const viewerGeometry = await viewer.evaluate(node => {
      const surface = node.querySelector('.app-summary-surface-shell');
      const header = node.querySelector('header');
      const panel = node.querySelector('.app-summary-workspace');
      const chat = node.querySelector('.app-summary-conversation');
      const glyph = header.querySelector(innerWidth < 761 ? '.app-shell-menu-toggle svg' : '.app-shell-history svg');
      return {direction: getComputedStyle(surface).flexDirection, headerHeight: header.getBoundingClientRect().height,
        sideBySide: panel.getBoundingClientRect().x > chat.getBoundingClientRect().x, chatVisible: getComputedStyle(chat).display !== 'none',
        glyph: glyph.getBoundingClientRect().width, role: header.querySelector('h1').dataset.forgeType,
        titleSize: getComputedStyle(header.querySelector('h1')).fontSize};
    });
    assert.equal(viewerGeometry.direction, 'row');
    assert.equal(viewerGeometry.role, 'label');
    assert.equal(viewerGeometry.glyph, 18);
    if (viewport.width > 500) {assert.equal(viewerGeometry.headerHeight, 54); assert.equal(viewerGeometry.sideBySide, true); assert.equal(viewerGeometry.chatVisible, true);}
    else {assert.equal(viewerGeometry.headerHeight, 59); assert.equal(viewerGeometry.chatVisible, false);}
    for (const name of ['Go back', 'Go forward']) assert.equal(await viewer.getByRole('button', {name, exact: true}).isVisible(), true);
    const closeGlyph = viewer.getByRole('button', {name: 'Close Example output', exact: true}).locator('svg');
    assert.equal(await closeGlyph.isVisible(), true, 'the tab-close glyph remains visible on phones');
    const hide = viewer.getByRole('button', {name: 'Hide workspace', exact: true});
    if (viewport.width > 500) {
      await activate(viewer.getByRole('button', {name: 'Expand workspace', exact: true}));
      const restore = viewer.getByRole('button', {name: 'Restore split view', exact: true});
      const pointer = await restore.evaluate(node => {
        const css = getComputedStyle(node), box = node.getBoundingClientRect();
        const next = node.parentElement.querySelector('[aria-expanded]'), other = next.getBoundingClientRect();
        return {focusVisible: node.matches(':focus-visible'), outline: css.outlineWidth, border: css.borderWidth,
          size: [box.width, box.height], nextSize: [other.width, other.height], gap: other.left - box.right,
          inset: node.parentElement.parentElement.getBoundingClientRect().right - other.right};
      });
      assert.equal(pointer.focusVisible, false);
      assert.equal(pointer.outline, '0px', 'Pointer focus must not inherit Blueprint’s global outline');
      assert.equal(pointer.border, '0px');
      assert.deepEqual(pointer.size, [36, 36]);
      assert.deepEqual(pointer.nextSize, pointer.size);
      assert.equal(pointer.gap, 2);
      assert.equal(pointer.inset, 12);
      await restore.press('Tab');
      const keyboard = await hide.evaluate(node => ({focused: node === document.activeElement,
        visible: node.matches(':focus-visible'), outline: getComputedStyle(node).outlineWidth,
        expectedOutline: getComputedStyle(node).getPropertyValue('--agently-theme-focus-width').trim() || '2px'}));
      assert.equal(keyboard.focused, true);
      assert.equal(keyboard.visible, true);
      assert.equal(keyboard.outline, keyboard.expectedOutline);
      await activate(restore);
      console.log(JSON.stringify({viewport, shellPointerFocus: pointer, shellKeyboardFocus: keyboard}));
    }

    if (viewport.width < 500) assert.ok(await hide.evaluate(node => node.getBoundingClientRect().width >= 44 && node.getBoundingClientRect().height >= 44));
    await activate(hide);
    assert.equal(await viewer.getByRole('tab', {name: 'Example output', exact: true}).count(), 0);
    assert.equal(await viewer.locator('[role="tabpanel"]').getAttribute('inert'), '');
    await activate(viewer.getByRole('button', {name: 'Show workspace', exact: true}));
    await viewer.getByRole('tab', {name: 'Example output', exact: true}).waitFor();
    console.log(JSON.stringify({viewport, shell: viewerGeometry}));
    console.log(JSON.stringify({viewport,charts:await verifyChartFoundation(page,{expectedFont:pilot?.expectedFont})}));
    if(pilot?.charts?.length) console.log(JSON.stringify({viewport,pilotCharts:await verifyAuthoredCharts(page,pilot.charts,pilot.chartReferenceStyles)}));
    const size = selector => page.locator(selector).first().evaluate(node => getComputedStyle(node).fontSize);
    await verifyDocumentChat(page, {touch: viewport.width < 500});
    const lifecycle=page.getByRole('region',{name:'Request lifecycle specimen'});
    await lifecycle.getByRole('button',{name:'Stop request',exact:true}).click();
    assert.match(await lifecycle.textContent(),/Request canceled/);
    await lifecycle.getByRole('button',{name:'Review and retry',exact:true}).click();
    await lifecycle.getByRole('button',{name:'Open example review',exact:true}).click();
    const review=page.locator('.app-request-review-dialog');await review.waitFor();
    const locked=review.locator('[data-forge-review-table=example-locked-approval]');
    assert.equal(await locked.getByRole('checkbox').first().isDisabled(),true);
    assert.equal(await review.getByRole('button',{name:'Download CSV',exact:true}).count(),0);
    for(const role of ['section','body','metadata','label']) {
      const target=review.locator(`[data-forge-type="${role}"]`).first();
      assert.equal(await target.evaluate((node,role)=>{const s=getComputedStyle(node),p=`--forge-type-${role}-`;return s.fontSize===s.getPropertyValue(p+'size').trim() && s.fontWeight===s.getPropertyValue(p+'weight').trim();},role),true);
    }
    await review.getByRole('button',{name:'Close example',exact:true}).click();
    const selected = await page.getByRole('combobox',{name:'Workspace theme',exact:true}).inputValue();
    const roleTokens = catalog.themes.find(theme => theme.id === selected).modes.light;
    const selectedTheme=catalog.themes.find(theme=>theme.id===selected);
    const profileTheme=catalog.themes.find(theme=>theme.id===selectedTheme.presentations?.document)||selectedTheme;
    const profileTokens=profileTheme.modes.light||profileTheme.modes[profileTheme.fallbackMode];
    for(const [key,tokens] of [['profile-legacy',roleTokens],['profile-document',profileTokens]]) {
      const text=page.locator(`#foundation-presentation-profile [data-forge-window-key="${key}"] p`);
      const type=await text.evaluate(node=>{const s=getComputedStyle(node);return {size:s.fontSize,weight:s.fontWeight,leading:s.lineHeight};});
      if(tokens['typography.body.size']!=null) assert.equal(type.size,`${tokens['typography.body.size']}px`);
      if(tokens['typography.body.weight']!=null) assert.equal(type.weight,String(tokens['typography.body.weight']));
      if(tokens['typography.body.lineHeight']!=null) assert.equal(type.leading,`${tokens['typography.body.lineHeight']}px`);
    }

    for (const label of ['Workspace theme', 'Workspace color mode']) {
      const select = page.getByRole('combobox', {name:label,exact:true});
      const geometry = await select.evaluate(node => {
        const icon = node.parentElement.querySelector('svg');
        const control = node.getBoundingClientRect();
        const glyph = icon?.getBoundingClientRect();
        const css = getComputedStyle(node);
        return {native:node.tagName,shared:node.parentElement.classList.contains('forge-select-control'),appearance:css.appearance,padding:parseFloat(css.paddingInlineEnd),inset:glyph ? control.right-glyph.right : null,size:glyph?.width,decorative:icon?.getAttribute('aria-hidden'),pointer:icon ? getComputedStyle(icon).pointerEvents : null,height:control.height};
      });
      assert.equal(geometry.native, 'SELECT');
      assert.equal(geometry.shared, true, 'Settings dropdowns must use the shared select');
      assert.equal(geometry.appearance, 'none');
      assert.equal(geometry.padding, 32);
      assert.ok(Math.abs(geometry.inset-10)<0.1, 'Chevron must be inset from the right edge');
      assert.equal(geometry.size, 16);
      assert.equal(geometry.decorative, 'true');
      assert.equal(geometry.pointer, 'none');
      if (viewport.width<500) assert.ok(geometry.height>=44);
    }
    if (process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS) {
      mkdirSync(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,{recursive:true});
      await page.locator('.app-ui-settings-card').first().screenshot({path:path.join(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,`settings-${viewport.width}.png`)});
    }
    const mode = page.getByRole('combobox',{name:'Workspace color mode',exact:true});
    await mode.selectOption('light');
    assert.equal(await mode.inputValue(), 'light');
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
    await page.getByRole('combobox',{name:'Workspace theme',exact:true}).selectOption('forge-default');
    assert.equal(await mode.isDisabled(),true,'No named theme keeps color mode unavailable');
    assert.equal(await mode.evaluate(node=>node.parentElement.classList.contains('forge-select-control')),true);
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
