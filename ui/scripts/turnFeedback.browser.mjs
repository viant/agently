import assert from 'node:assert/strict';
import {mkdirSync} from 'node:fs';
import path from 'node:path';

// Exercise the product component and SDK against synthetic local receipts.
export async function verifyTurnFeedback(page, {touch = false} = {}) {
  const writes = [];
  const capture = async (locator, name) => {
    if (!process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS) return;
    mkdirSync(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,{recursive:true});
    await locator.screenshot({path:path.join(process.env.AGENTLY_TYPOGRAPHY_SCREENSHOTS,
      'feedback-' + page.viewportSize().width + '-' + name + '.png')});
  };
  const endpoint = '**/v1/conversations/specimen-feedback/turns/example-answer/feedback';
  await page.route(endpoint, async route => {
    const request = route.request();
    const input = request.method() === 'DELETE' ? {rating:0,comment:null} : request.postDataJSON();
    writes.push({method:request.method(), ...input});
    await route.fulfill({contentType:'application/json',body:JSON.stringify({...input,
      updatedAt:input.rating ? '2026-10-24T12:00:00Z' : null})});
  });
  const footer = page.locator('#foundation-chat .app-turn-feedback');
  const options = footer.getByRole('button',{name:'Feedback options',exact:true});
  const menu = page.getByRole('menu',{name:'Response feedback',exact:true});
  const dialog = page.getByRole('dialog',{name:'Feedback',exact:true});
  const field = dialog.getByRole('textbox',{name:'Comment (optional)',exact:true});
  const openComment = async name => {
    if (touch) await options.tap();
    else await options.press('Enter');
    await menu.getByRole('menuitem',{name,exact:true}).click();
    await field.waitFor();
    await page.waitForFunction(() => document.querySelector('.app-turn-feedback-editor textarea') === document.activeElement);
  };

  const rate = footer.getByRole('button',{name:'Rate response',exact:true});
  if (touch) assert.ok(await rate.evaluate(node => node.getBoundingClientRect().height >= 44));
  await capture(footer, 'controls');
  await rate.focus();
  await rate.press('Enter');
  const good = menu.getByRole('menuitem',{name:'Good response',exact:true});
  await good.focus();
  await capture(menu, 'menu');
  await good.press('ArrowDown');
  const bad = menu.getByRole('menuitem',{name:'Bad response',exact:true});
  assert.equal(await bad.evaluate(node => node === document.activeElement), true);
  await bad.press('Enter');
  await field.waitFor();
  await page.waitForFunction(() => document.querySelector('.app-turn-feedback-editor textarea') === document.activeElement);
  assert.deepEqual(writes,[{method:'PUT',rating:-1,comment:null}]);
  assert.equal(await dialog.getByRole('textbox').count(),1);
  assert.equal(await dialog.getByRole('checkbox').count(),0);
  assert.equal(await dialog.getByRole('combobox').count(),0);
  const box = await dialog.boundingBox();
  assert.ok(box.x >= 16 && box.x + box.width <= page.viewportSize().width - 15 && box.width <= 560);
  // Blueprint's focusable container and focus guards surround role=dialog.
  // Check a complete cycle in both directions against the modal overlay.
  for (const key of ['Tab','Shift+Tab']) {
    for (let step = 0; step < 8; step++) {
      await page.keyboard.press(key);
      assert.equal(await dialog.evaluate(node => node.closest('.bp6-overlay').contains(document.activeElement)),true,
        'Modal must contain keyboard focus');
    }
  }
  await field.fill('Discard this draft');
  await field.press('Escape');
  await dialog.waitFor({state:'hidden'});
  const negative = footer.getByRole('button',{name:'Remove negative feedback',exact:true});
  assert.equal(await negative.getAttribute('aria-pressed'),'true');
  assert.equal(await negative.evaluate(node => node === document.activeElement),true);

  await openComment('Add comment');
  assert.equal(await field.inputValue(),'');
  const comment = 'Helpful example 👍\nMore detail';
  await field.fill(comment);
  await dialog.getByRole('button',{name:'Save comment',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  assert.deepEqual(writes.at(-1),{method:'PUT',rating:-1,comment});
  assert.equal(await footer.getByText('Helpful example 👍',{exact:false}).count(),0);
  assert.equal(await options.evaluate(node => node === document.activeElement),true);
  await options.click();
  await menu.getByRole('menuitem',{name:'Good response',exact:true}).click();
  const positive = footer.getByRole('button',{name:'Remove positive feedback',exact:true});
  await positive.waitFor();
  await menu.waitFor({state:'hidden'});
  assert.equal(await dialog.count(),0);
  assert.deepEqual(writes.at(-1),{method:'PUT',rating:1,comment});

  const mode = page.getByRole('combobox',{name:'Workspace color mode',exact:true});
  const originalMode = await mode.inputValue();
  const appearances = [];
  for (const value of ['light','dark']) {
    if (await mode.locator('option[value="' + value + '"]').isDisabled()) continue;
    await mode.selectOption(value);
    await openComment('Edit comment');
    assert.equal(await field.inputValue(),comment);
    appearances.push(await dialog.evaluate(node => ({
      mode:node.closest('[data-forge-color-mode]')?.dataset.forgeColorMode,
      background:getComputedStyle(node).backgroundColor,
      header:getComputedStyle(node.querySelector('.bp6-dialog-header')).backgroundColor,
      headerShadow:getComputedStyle(node.querySelector('.bp6-dialog-header')).boxShadow,
      field:getComputedStyle(node.querySelector('textarea')).backgroundColor,
    })));
    assert.equal(appearances.at(-1).mode,value,'Portaled dialog must retain the workspace mode');
    assert.equal(appearances.at(-1).header,appearances.at(-1).background,'Header must use the dialog surface');
    assert.equal(appearances.at(-1).headerShadow,'none');
    await capture(dialog, value);
    await dialog.getByRole('button',{name:'Cancel',exact:true}).click();
    await dialog.waitFor({state:'hidden'});
  }
  if (appearances.length === 2) assert.notEqual(appearances[0].background,appearances[1].background);
  await mode.selectOption(originalMode);
  await positive.click();
  await rate.waitFor();
  assert.deepEqual(writes.at(-1),{method:'DELETE',rating:0,comment:null});
  await page.unroute(endpoint);
  console.log(JSON.stringify({turnFeedback:{touch,box,appearances,writes:writes.length}}));
}
