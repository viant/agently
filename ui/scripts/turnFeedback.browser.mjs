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
  const mode = page.getByRole('combobox',{name:'Workspace color mode',exact:true});
  const originalMode = await mode.inputValue();
  // Rich content loads lazily; the user's prose may appear before the answer.
  await page.locator('#foundation-chat [data-testid="chat-message-example-answer-row"] .app-rich-prose > p').nth(1).waitFor();
  const geometry = [];
  const measure = () => footer.evaluate(node => {
    const row = node.previousElementSibling;
    const text = row.querySelector('.app-rich-prose p, .app-rich-prose');
    const toolbar = node.querySelector('.app-turn-feedback-toolbar');
    const rating = toolbar.querySelector('[data-feedback-action="rating"]');
    const more = toolbar.querySelector('[data-feedback-action="more"]');
    const textBox = text.getBoundingClientRect();
    const ratingBox = rating.getBoundingClientRect();
    const moreBox = more.getBoundingClientRect();
    const css = getComputedStyle(rating);
    const glyphs = [...rating.querySelectorAll('svg')].map(svg => svg.getBoundingClientRect().width);
    const paragraphs = [...row.querySelectorAll('.app-rich-prose > p')];
    return {
      alignment: rating.querySelector('svg').getBoundingClientRect().left - textBox.left,
      answerGap: toolbar.getBoundingClientRect().top - row.getBoundingClientRect().bottom,
      controlGap: moreBox.left - ratingBox.right,
      ratingSize: [ratingBox.width, ratingBox.height],
      moreSize: [moreBox.width, moreBox.height],
      moreOffset: moreBox.left - textBox.left,
      glyphs,
      paragraphGap: paragraphs[1].getBoundingClientRect().top - paragraphs[0].getBoundingClientRect().bottom,
      lastParagraphMargin: getComputedStyle(paragraphs.at(-1)).marginBottom,
      preservedBreaks: paragraphs[1].querySelectorAll('br').length,
      color: css.color,
      background: css.backgroundColor,
    };
  });
  const verifyGeometry = async state => {
    const values = [originalMode];
    if (!await mode.isDisabled()) {
      values.length = 0;
      for (const value of ['light','dark']) {
        if (!await mode.locator('option[value="' + value + '"]').isDisabled()) values.push(value);
      }
    }
    for (const value of values) {
      if (await mode.inputValue() !== value) await mode.selectOption(value);
      const actual = await measure();
      assert.ok(Math.abs(actual.alignment) <= 1, `${state}/${value}: glyph must align with the answer: ${JSON.stringify(actual)}`);
      assert.equal(actual.answerGap,12);
      assert.equal(actual.paragraphGap,12,'Paragraphs must not stack literal newlines and blank-line breaks');
      assert.equal(actual.lastParagraphMargin,'0px','The final paragraph must not add space before feedback');
      assert.equal(actual.preservedBreaks,1);
      assert.equal(actual.controlGap,4);
      assert.deepEqual(actual.glyphs, state === 'unrated' ? [14,14] : [16]);
      if (state !== 'unrated') {
        assert.notEqual(actual.color,geometry.find(previous => previous.mode === value).color,'Selected thumb must retain the accent color');
        assert.notEqual(actual.background,'rgba(0, 0, 0, 0)','Selected thumb must retain its subtle background');
      }
      if (touch) for (const size of [actual.ratingSize,actual.moreSize]) assert.ok(size.every(value => value >= 44));
      else assert.equal(actual.ratingSize[1],28);
      if (geometry.length) {
        assert.deepEqual(actual.ratingSize,geometry[0].ratingSize,'Rating must not shift on selection');
        assert.equal(actual.moreOffset,geometry[0].moreOffset,'Options must not shift on selection');
      }
      geometry.push({state,mode:value,...actual});
      await capture(footer, `${state}-${value}`);
    }
    if (await mode.inputValue() !== originalMode) await mode.selectOption(originalMode);
  };
  const openComment = async name => {
    if (touch) await options.tap();
    else await options.press('Enter');
    await menu.getByRole('menuitem',{name,exact:true}).click();
    await field.waitFor();
    await page.waitForFunction(() => document.querySelector('.app-turn-feedback-editor textarea') === document.activeElement);
  };

  const rate = footer.getByRole('button',{name:'Rate response',exact:true});
  await verifyGeometry('unrated');
  await capture(page.locator('#foundation-chat .app-bubble-row-assistant'), 'answer');
  await capture(footer, 'controls');
  await rate.focus();
  await rate.press('Enter');
  const good = menu.getByRole('menuitem',{name:'Good response',exact:true});
  await good.focus();
  assert.equal(await good.locator('svg.lucide-thumbs-up').count(),1);
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
  assert.equal(await dialog.getByText('Your rating is saved.',{exact:true}).isVisible(),true);
  assert.equal(await dialog.getByRole('button',{name:'Not now',exact:true}).isVisible(),true);
  const saveComment = dialog.getByRole('button',{name:'Save comment',exact:true});
  assert.equal(await saveComment.isDisabled(),true);
  await field.fill(' \n\t ');
  assert.equal(await saveComment.isDisabled(),true);
  await field.fill('');
  assert.equal(writes.length,1,'An empty new comment must not cause an extra write');
  const box = await dialog.boundingBox();
  assert.ok(box.x >= 16 && box.x + box.width <= page.viewportSize().width - 15 && box.width <= 560);
  assert.equal(await field.getAttribute('rows'),'4');
  const form = await field.evaluate(node => {
    const css = getComputedStyle(node);
    const save = node.form.querySelector('[type="submit"]').getBoundingClientRect();
    return {height:node.getBoundingClientRect().height,resize:css.resize,shadow:css.boxShadow,
      outline:parseFloat(css.outlineWidth),rightGap:node.getBoundingClientRect().right-save.right};
  });
  assert.ok(form.height >= 96);
  assert.equal(form.resize,'vertical');
  assert.equal(form.shadow,'none','Textarea must have one focus indicator');
  assert.ok(form.outline > 0);
  assert.ok(Math.abs(form.rightGap) <= 1);
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
  await verifyGeometry('negative');

  await openComment('Add comment');
  assert.equal(await field.inputValue(),'');
  const comment = 'Helpful example 👍\nMore detail';
  await field.fill(comment);
  await dialog.getByRole('button',{name:'Save comment',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  assert.deepEqual(writes.at(-1),{method:'PUT',rating:-1,comment});
  assert.equal(await footer.getByText('Helpful example 👍',{exact:false}).count(),0);
  assert.equal(await options.evaluate(node => node === document.activeElement),true);
  assert.equal(await footer.getByRole('status').textContent(),'Comment saved');
  const savedGeometry = await measure();
  await page.clock.runFor(3100);
  assert.equal(await footer.getByRole('status').textContent(),'');
  assert.deepEqual(await measure(),savedGeometry,'Save confirmation must not move controls');
  await options.click();
  await menu.getByRole('menuitem',{name:'Good response',exact:true}).click();
  const positive = footer.getByRole('button',{name:'Remove positive feedback',exact:true});
  await positive.waitFor();
  await menu.waitFor({state:'hidden'});
  assert.equal(await dialog.count(),0);
  assert.deepEqual(writes.at(-1),{method:'PUT',rating:1,comment});
  await verifyGeometry('positive');

  const appearances = [];
  for (const value of ['light','dark']) {
    if (await mode.locator('option[value="' + value + '"]').isDisabled()) continue;
    await mode.selectOption(value);
    await openComment('Edit comment');
    assert.equal(await field.inputValue(),comment);
    assert.equal(await saveComment.isDisabled(),true,'An unchanged comment must not be submitted again');
    await field.fill(comment + '\nDraft');
    assert.equal(await saveComment.isEnabled(),true);
    await field.fill(comment);
    assert.equal(await saveComment.isDisabled(),true);
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
    await field.fill('👍'.repeat(4001));
    const limitError = dialog.getByRole('alert');
    assert.equal(await limitError.textContent(),'Remove 1 character to save your comment.');
    assert.equal(await saveComment.isDisabled(),true);
    assert.equal(await field.getAttribute('aria-invalid'),'true');
    assert.ok((await field.getAttribute('aria-describedby')).split(' ').includes(await limitError.getAttribute('id')));
    const errorAppearance = await limitError.evaluate(node => {
      const foreground = getComputedStyle(node).color;
      const background = getComputedStyle(node.closest('.app-turn-feedback-dialog')).backgroundColor;
      const luminance = color => color.match(/[\d.]+/g).slice(0,3).map(value => {
        const channel = Number(value) / 255;
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
      }).reduce((sum,channel,index) => sum + channel * [0.2126,0.7152,0.0722][index],0);
      const light = luminance(foreground), dark = luminance(background);
      return {foreground,background,contrast:(Math.max(light,dark) + 0.05) / (Math.min(light,dark) + 0.05)};
    });
    assert.ok(errorAppearance.contrast >= 4.5,`Comment validation must remain readable: ${JSON.stringify(errorAppearance)}`);
    appearances.at(-1).error = errorAppearance;
    await capture(dialog, 'limit-error-' + value);
    await field.fill('👍'.repeat(4000));
    assert.equal(await limitError.count(),0);
    assert.equal(await field.getAttribute('aria-invalid'),'false');
    assert.equal(await saveComment.isEnabled(),true);
    await dialog.getByRole('button',{name:'Cancel',exact:true}).click();
    await dialog.waitFor({state:'hidden'});
  }
  if (appearances.length === 2) assert.notEqual(appearances[0].background,appearances[1].background);
  await mode.selectOption(originalMode);
  await openComment('Edit comment');
  await field.fill(' \n\t ');
  assert.equal(await saveComment.isEnabled(),true,'An existing comment can still be cleared');
  await saveComment.click();
  await dialog.waitFor({state:'hidden'});
  assert.deepEqual(writes.at(-1),{method:'PUT',rating:1,comment:null});
  const writeCount = writes.length;
  await openComment('Add comment');
  assert.equal(await field.inputValue(),'');
  assert.equal(await saveComment.isDisabled(),true);
  await dialog.getByRole('button',{name:'Not now',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  assert.equal(writes.length,writeCount,'Not now must preserve the saved vote without another write');
  await positive.click();
  await rate.waitFor();
  assert.deepEqual(writes.at(-1),{method:'DELETE',rating:0,comment:null});
  await page.unroute(endpoint);
  console.log(JSON.stringify({turnFeedback:{touch,box,form,geometry,appearances,writes:writes.length}}));
}
