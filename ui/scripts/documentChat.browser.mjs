import assert from 'node:assert/strict';
import {mkdirSync} from 'node:fs';
import path from 'node:path';

export async function verifyExecutionDetails(page, {touch = false} = {}) {
  const specimen=page.locator('#foundation-execution-details');
  const open=specimen.getByRole('button',{name:'Open model details',exact:true});
  const dialog=page.getByRole('dialog',{name:'Step details',exact:true});
  const tab=name=>dialog.getByRole('tab',{name,exact:true});
  const oneDialog=async()=>{await dialog.waitFor();await page.clock.runFor(400);assert.equal(await page.locator('[role="dialog"]:visible').count(),1);};
  const capture=async name=>{
    if(!process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS)return;
    mkdirSync(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,{recursive:true});
    await dialog.screenshot({path:path.join(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,`execution-${page.viewportSize().width}-${name}.png`)});
  };
  await open.click();await oneDialog();
  assert.equal(await dialog.getByRole('tab').count(),6);
  assert.equal(await dialog.getByRole('heading',{name:'Step details',exact:true}).getAttribute('data-forge-type'),'section');
  const box=await dialog.boundingBox(), viewport=page.viewportSize();
  assert.equal(box.width,viewport.width>600?960:viewport.width-16);
  assert.ok(box.x>=0 && box.y>=0 && box.x+box.width<=viewport.width && box.y+box.height<=viewport.height,JSON.stringify(box));
  assert.equal(await dialog.evaluate(node=>node.scrollWidth<=node.clientWidth),true);
  await capture('overview');
  await tab('Overview').press('ArrowRight');
  await dialog.locator('.cm-content').waitFor();await oneDialog();
  assert.equal(await tab('Request').getAttribute('aria-selected'),'true');
  assert.equal(await tab('Request').evaluate(node=>node===document.activeElement),true);
  for(const [name,text] of [['Response','Ready for review.'],['Stream','Example output fragment.'],['Provider request','example-model'],['Provider response','complete']]) {
    await tab(name).click();await oneDialog();
    await page.waitForFunction(value=>document.querySelector('.app-execution-code .cm-content')?.textContent.includes(value),text);
    assert.equal(await dialog.getByRole('button',{name:'Copy',exact:true}).isEnabled(),true);
    if(name==='Stream') {
      const scroll=await dialog.locator('.cm-scroller').evaluate(node=>({height:node.clientHeight,content:node.scrollHeight}));
      assert.ok(scroll.height>0 && scroll.content>scroll.height,JSON.stringify(scroll));
      await capture('stream');
    }
  }
  const code=dialog.locator('.cm-scroller');
  await dialog.locator('.app-execution-code').evaluate(node=>node.style.setProperty('--forge-type-code-size','17px'));
  assert.equal(await code.evaluate(node=>getComputedStyle(node).fontSize),'17px');
  await dialog.locator('.app-execution-code').evaluate(node=>node.style.removeProperty('--forge-type-code-size'));
  const mode=page.getByRole('combobox',{name:'Workspace color mode',exact:true}), originalMode=await mode.inputValue();
  await dialog.getByRole('button',{name:'Close execution details',exact:true}).click();await dialog.waitFor({state:'hidden'});
  const appearances=[];
  for(const value of ['light','dark']) {
    if(await mode.locator(`option[value="${value}"]`).isDisabled())continue;
    await mode.selectOption(value);await open.click();await oneDialog();
    appearances.push(await dialog.evaluate(node=>({mode:node.closest('[data-forge-color-mode]')?.dataset.forgeColorMode,background:getComputedStyle(node).backgroundColor,panel:getComputedStyle(node.querySelector('.app-detail-panel')).backgroundColor})));
    assert.equal(appearances.at(-1).mode,value);
    assert.equal(appearances.at(-1).background,appearances.at(-1).panel);
    await capture(value);
    await tab('Overview').press('Escape');await dialog.waitFor({state:'hidden'});
    assert.equal(await open.evaluate(node=>node===document.activeElement),true);
  }
  if(appearances.length===2)assert.notEqual(appearances[0].background,appearances[1].background);
  await mode.selectOption(originalMode);
  await specimen.getByRole('button',{name:'Open tool details',exact:true}).click();
  assert.equal(await dialog.getByRole('tab').count(),3);
  await tab('Response').click();await dialog.locator('.cm-content').waitFor();await oneDialog();
  assert.equal(await dialog.locator('.cm-content').textContent(),'false');
  await dialog.getByRole('button',{name:'Close execution details',exact:true}).click();await dialog.waitFor({state:'hidden'});
  console.log(JSON.stringify({executionDetails:{touch,box,appearances,singleInspector:true}}));
}

export async function verifyDocumentChat(page, {touch = false} = {}) {
  const specimen = page.locator('#foundation-chat');
  const field = specimen.getByRole('textbox', {name: 'Message', exact: true});
  await field.waitFor();
  await specimen.getByRole('article', {name: 'Assistant response'}).locator('.app-rich-prose').waitFor();
  assert.equal(await field.getAttribute('data-forge-type'), 'body');
  assert.equal(await field.getAttribute('aria-describedby') !== null, true);
  const filters = specimen.locator('.forge-task-filters button');
  assert.ok((await filters.evaluateAll(nodes => nodes.map(node => node.dataset.forgeType))).every(role => role === 'small'));
  const smallRoles = specimen.locator('.forge-task-filters button, .forge-task-row [data-forge-type=small]');
  await smallRoles.evaluateAll(nodes => nodes.forEach(node => {for(const [key,value] of Object.entries({size:'15px',weight:'600','line-height':'22px','letter-spacing':'.02em',family:'monospace'})) node.style.setProperty(`--forge-type-small-${key}`,value);}));
  assert.ok((await smallRoles.evaluateAll(nodes => nodes.map(node=>{const s=getComputedStyle(node);return [s.fontSize,s.fontWeight,s.lineHeight,s.letterSpacing,s.fontFamily];}))).every(values=>JSON.stringify(values)===JSON.stringify(['15px','600','22px','0.3px','monospace'])));
  await smallRoles.evaluateAll(nodes=>nodes.forEach(node=>{for(const key of ['size','weight','line-height','letter-spacing','family'])node.style.removeProperty(`--forge-type-small-${key}`);}));
  const send = specimen.getByRole('button', {name: 'Send', exact: true});
  assert.equal(await send.isDisabled(), true);
  await field.fill('   '); assert.equal(await send.isDisabled(), true);
  await field.fill('Review this example.');
  await field.press('Shift+Enter');
  assert.equal(await field.inputValue(), 'Review this example.\n');
  const controls = specimen.getByRole('button', {name: 'Tools', exact: true});
  await controls.click();
  const model = page.getByRole('dialog', {name: 'Tools', exact: true}).getByRole('combobox', {name: 'Model', exact: true});
  await model.waitFor();
  const bounds = await model.boundingBox();
  assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= page.viewportSize().width);
  await model.press('Escape');
  await page.getByRole('dialog', {name:'Tools', exact:true}).waitFor({state:'hidden'});
  await page.waitForFunction(() => document.querySelector('#foundation-chat button[aria-label=Tools]') === document.activeElement);
  assert.equal(await controls.evaluate(node => node === document.activeElement), true);
  await field.click();
  const pointer = await specimen.locator('.forge-document-composer-shell').evaluate(node => ({outline: getComputedStyle(node).outlineWidth, style: getComputedStyle(node).outlineStyle, className: node.parentElement.className}));
  assert.equal(pointer.style, 'none', JSON.stringify(pointer));
  await field.press('Enter');
  assert.equal(await field.inputValue(), '');
  await page.waitForFunction(() => document.querySelector('#foundation-chat [data-testid="chat-composer-input"]') === document.activeElement);
  const roles = await specimen.locator('.app-bubble-row').evaluateAll(nodes => nodes.map(node => ({role: node.dataset.forgeType, border: getComputedStyle(node.querySelector('.app-bubble')).borderWidth, size: getComputedStyle(node).fontSize, lineHeight: getComputedStyle(node).lineHeight})));
  assert.ok(roles.every(row => row.role === 'body' && row.border === '0px'));
  if (touch) assert.equal(await field.evaluate(node => getComputedStyle(node).fontSize), '16px');
  await page.evaluate(() => {
    const host = document.querySelector('#foundation-chat').closest('.agently-workspace');
    for (const [key, value] of Object.entries({size: '17px', weight: '500', 'line-height': '24px', 'letter-spacing': '.03em', family: 'monospace'})) host.style.setProperty(`--forge-type-body-${key}`, value);
  });
  const changed = await specimen.locator('.app-bubble-row').evaluateAll(nodes => nodes.map(node => {
    const css = getComputedStyle(node.querySelector('.app-bubble-content .app-rich-prose') || node); return [css.fontSize, css.fontWeight, css.lineHeight, css.letterSpacing, css.fontFamily];
  }));
  for (const role of changed) assert.deepEqual(role, ['17px', '500', '24px', '0.51px', 'monospace']);
  await page.evaluate(() => {const host = document.querySelector('#foundation-chat').closest('.agently-workspace'); for (const key of ['size', 'weight', 'line-height', 'letter-spacing', 'family']) host.style.removeProperty(`--forge-type-body-${key}`);});
  const home = page.locator('#foundation-chat-home .forge-document-chat');
  const allocation = await home.evaluate(node=>{const h=node.querySelector('h1'),form=node.querySelector('form'),tasks=node.querySelector('.forge-task-browser');return {gap:form.getBoundingClientRect().top-h.getBoundingClientRect().bottom,composerFirst:!!(form.compareDocumentPosition(tasks)&Node.DOCUMENT_POSITION_FOLLOWING)};});
  assert.ok(Math.abs(allocation.gap-25)<=1, 'Native Home composer must stay below its heading when content is short');
  assert.equal(allocation.composerFirst,true);
  const homeField = home.getByRole('textbox', {name:'Message', exact:true});
  const selectTask = () => touch ? home.locator('.forge-task-row').tap() : home.locator('.forge-task-row').click();
  const waitForDraft = async value => {
    try {
      await page.waitForFunction(expected => document.querySelector('#foundation-chat-home textarea')?.value === expected, value);
    } catch (error) {
      const actual = await page.locator('#foundation-chat-home textarea').inputValue();
      throw new Error(`Composer draft mismatch: expected ${JSON.stringify(value)}, received ${JSON.stringify(actual)}`, {cause: error});
    }
  };
  await homeField.fill('Independent Home draft.');
  await selectTask();
  const back = home.getByRole('button', {name:'New Chat', exact:true});
  await back.waitFor();
  await waitForDraft('Review this example.');
  assert.equal(await homeField.inputValue(), 'Review this example.');
  await homeField.fill('Edited starter request.');
  const returnGeometry = await back.evaluate(node => {
    const box = node.getBoundingClientRect(), heading = node.closest('.forge-document-chat').querySelector('h1').getBoundingClientRect();
    return {width:box.width, height:box.height, left:box.x-heading.x, gap:heading.y-box.bottom, frameWidth:heading.width};
  });
  assert.equal(await back.getAttribute('data-forge-variant'), 'link');
  assert.equal(await back.getAttribute('data-forge-type'), 'small');
  await back.evaluate(node => {for (const [key,value] of Object.entries({size:'15px',weight:'600','line-height':'22px','letter-spacing':'.02em',family:'monospace'})) node.style.setProperty(`--forge-type-small-${key}`,value);});
  assert.deepEqual(await back.evaluate(node => {const css=getComputedStyle(node);return [css.fontSize,css.fontWeight,css.lineHeight,css.letterSpacing,css.fontFamily];}), ['15px','600','22px','0.3px','monospace']);
  await back.evaluate(node => {for (const key of ['size','weight','line-height','letter-spacing','family']) node.style.removeProperty(`--forge-type-small-${key}`);});
  assert.ok(returnGeometry.width < returnGeometry.frameWidth - 20, 'The return action must remain compact');
  assert.ok(returnGeometry.height >= 44, 'The return action retains a touch target');
  assert.ok(Math.abs(returnGeometry.left) < 1 && Math.abs(returnGeometry.gap-16) < 1, 'Return aligns with the heading and keeps its spacing');
  await homeField.press('Shift+Tab');
  assert.equal(await back.evaluate(node => node === document.activeElement && node.matches(':focus-visible')), true);
  const returnFocus = await back.evaluate(node => ({focused: node === document.activeElement, visible: node.matches(':focus-visible'), outline: getComputedStyle(node).outlineStyle, width: getComputedStyle(node).getPropertyValue('--forge-focus-width'), color: getComputedStyle(node).getPropertyValue('--chat-focus')}));
  assert.notEqual(returnFocus.outline, 'none', JSON.stringify(returnFocus));
  await back.press('Enter');
  await home.locator('.forge-task-row').waitFor();
  await waitForDraft('Independent Home draft.');
  assert.equal(await homeField.inputValue(), 'Independent Home draft.');
  await home.getByRole('button', {name:'Browse all 1 tasks', exact:true}).click();
  await home.getByRole('searchbox', {name:'Search tasks', exact:true}).fill('Review');
  await selectTask();
  await home.getByRole('button', {name:'All tasks', exact:true}).waitFor();
  await waitForDraft('Edited starter request.');
  assert.equal(await homeField.inputValue(), 'Edited starter request.');
  await home.getByRole('button', {name:'All tasks', exact:true}).press('Enter');
  await home.getByRole('searchbox', {name:'Search tasks', exact:true}).waitFor();
  assert.equal(await home.getByRole('searchbox', {name:'Search tasks', exact:true}).inputValue(), 'Review');
  await home.getByRole('button', {name:'New Chat', exact:true}).click();
  await waitForDraft('Independent Home draft.');
  assert.equal(await homeField.inputValue(), 'Independent Home draft.');
  assert.equal(await home.locator('.chat-feed article').count(), 0, 'Task selection and return never send a message');
  const output = page.locator('#foundation-output-handoff');
  const summarySpacing = await output.evaluate(node => {
    const summary = node.querySelector('[data-message-id="findings"] .app-rich-prose');
    const text = [...summary.querySelectorAll('p')];
    const bounds = child => {const range = document.createRange(); range.selectNodeContents(child); return range.getBoundingClientRect();};
    const content = node.querySelector('[data-message-id="answer"] .app-bubble-content');
    const card = content.querySelector('.app-workspace-attachment');
    return {whiteSpace:getComputedStyle(summary).whiteSpace, lineHeight:parseFloat(getComputedStyle(summary).lineHeight),
      textGap:bounds(text.at(-1)).top-bounds(text[0]).bottom,
      cardGap:card.getBoundingClientRect().top-summary.getBoundingClientRect().bottom,
      firstChild:content.firstElementChild.className, cardMargin:getComputedStyle(card).marginTop};
  });
  assert.equal(summarySpacing.whiteSpace,'normal','Generated Markdown line breaks must not also preserve HTML source newlines');
  assert.ok(summarySpacing.textGap <= summarySpacing.lineHeight,JSON.stringify(summarySpacing));
  assert.ok(summarySpacing.cardGap <= summarySpacing.lineHeight*2,JSON.stringify(summarySpacing));
  assert.equal(summarySpacing.firstChild,'app-workspace-attachments','A report-only response must not add an empty text line');
  assert.equal(summarySpacing.cardMargin,'0px');
  assert.equal(await specimen.getByRole('article',{name:'Your message'}).first().locator('.app-rich-prose').evaluate(node=>getComputedStyle(node).whiteSpace),'pre-wrap','User-authored line breaks stay intact');
  const openOutput = output.getByRole('button', {name:'Show Delivery overview', exact:true});
  await openOutput.click();
  await output.locator('.app-summary-workspace .app-rich-inline-report').waitFor();
  assert.equal(await output.getByRole('tab', {name:'Delivery overview', exact:true}).isVisible(), true);
  if (!touch) {
    const style = await output.getAttribute('style');
    try {
      await output.evaluate(node => {node.style.width = '759px';});
      await output.locator('.app-summary-conversation').waitFor({state:'hidden'});
      await output.evaluate(node => {node.style.width = '842px';});
      await output.locator('.app-summary-conversation').waitFor({state:'visible'});
      const panes = await output.evaluate(node => ({chat:node.querySelector('.app-summary-conversation').getBoundingClientRect().toJSON(),workspace:node.querySelector('.app-summary-workspace').getBoundingClientRect().toJSON()}));
      assert.equal(panes.chat.width,380);
      assert.equal(panes.workspace.left,panes.chat.right,'Changing available space must restore chat without a viewport resize');
    } finally {
      await output.evaluate((node, style) => {if (style === null) node.removeAttribute('style'); else node.setAttribute('style', style);}, style);
    }
  }
  await output.getByRole('button', {name:'Hide workspace', exact:true}).click();
  assert.equal(await output.locator('.app-summary-workspace').isVisible(), false);
  await output.getByRole('button', {name:'Show workspace', exact:true}).click();
  await output.getByRole('button', {name:'Close Delivery overview', exact:true}).click();
  assert.equal(await output.getByRole('tab', {name:'Delivery overview', exact:true}).count(), 0);
  await output.getByRole('button', {name:'Saved outputs (1)', exact:true}).click();
  await output.getByRole('button', {name:'Open Delivery overview', exact:true}).click();
  await output.locator('.app-summary-workspace .app-rich-inline-report').waitFor();
  assert.equal(await output.locator('.app-summary-conversation .app-rich-inline-report').count(), 0);
  await output.getByRole('button', {name:'Close Delivery overview', exact:true}).click();
  const reportRead = '**/v1/conversations/specimen-report-handoff/transcript?*';
  await page.route(reportRead, route=>route.fulfill({status:403,contentType:'application/json',body:JSON.stringify({error:'Sample access denied'})}));
  await output.getByRole('button', {name:'Reopen Delivery overview', exact:true}).click();
  await output.getByRole('heading', {name:'You don’t have access to this output',exact:true}).waitFor();
  assert.equal(await output.locator('.app-summary-workspace .app-rich-inline-report').count(),0);
  await output.getByRole('button',{name:'Return to conversation',exact:true}).click();
  assert.equal(await output.locator('.app-summary-workspace').isVisible(),false);
  await output.getByRole('button',{name:'Show workspace',exact:true}).click();
  await page.unroute(reportRead);
  await output.getByRole('button',{name:'Try again',exact:true}).click();
  await output.locator('.app-summary-workspace .app-rich-inline-report').waitFor();
  const attachmentRoles = await output.locator('.app-workspace-attachment [data-forge-type]').evaluateAll(nodes=>nodes.map(node=>node.dataset.forgeType));
  assert.deepEqual(attachmentRoles,['label','metadata']);
  const attachmentStyle = await output.locator('.app-workspace-attachment').evaluate(node=>{
    const css=getComputedStyle(node);return {shadow:css.boxShadow,color:css.color,accent:css.getPropertyValue('--app-accent').trim()};
  });
  assert.equal(attachmentStyle.shadow,'none');
  const title = output.locator('.app-workspace-attachment strong');
  await title.evaluate(node=>node.style.setProperty('--forge-type-label-size','17px'));
  assert.equal(await title.evaluate(node=>getComputedStyle(node).fontSize),'17px');
  await title.evaluate(node=>node.style.removeProperty('--forge-type-label-size'));

  const objectId=await output.locator('[data-workspace-object-id]').getAttribute('data-workspace-object-id');
  await output.getByRole('button',{name:'Prepare example report',exact:true}).click();
  assert.equal(await output.getByTestId('workspace-attachment-card').count(),0);
  assert.equal(await output.getByRole('button',{name:'Saved outputs (1)',exact:true}).count(),0);
  assert.equal(await output.locator('.app-rich-inline-report,.app-forge-fence-loading').count(),0);
  assert.equal(await output.getByText('The selected record is ready for review.',{exact:false}).count(),0);
  const progress=output.getByTestId('turn-progress-status');
  assert.match(await progress.textContent(),/Preparing report/);
  await page.clock.fastForward(31000);
  assert.match(await progress.textContent(),/Preparing report/);
  assert.match(await progress.textContent(),/No new updates/);
  await output.getByRole('button',{name:'Complete example report',exact:true}).click();
  assert.equal(await output.getByTestId('turn-progress-status').count(),0);
  await output.getByText('The selected record is ready for review.',{exact:false}).waitFor();
  assert.equal(await openOutput.isEnabled(),true);
  assert.equal(await openOutput.getAttribute('data-workspace-object-id'),objectId);
  await openOutput.click();
  await output.locator('.app-summary-workspace .app-rich-inline-report').waitFor();
  assert.equal(await output.locator('.app-summary-conversation .app-rich-inline-report').count(),0);

  console.log(JSON.stringify({documentChat: {roles, changed, touch, nativeCallbacks: true}}));
}
