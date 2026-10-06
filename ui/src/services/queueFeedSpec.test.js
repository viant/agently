import {describe,it,expect} from 'vitest';
import {normalizeQueueEditorBinding} from './queueFeedSpec';
const legacy = {containers:[{id:'queueEditor',title:'Edit selected request',schemaBasedForm:{id:'queueEditForm',dataSourceRef:'queueTurns',schema:{type:'object',properties:{preview:{type:'string',title:'Queued request'}}}},toolbar:{items:[{on:[{handler:'chat.saveQueuedTurnForm'}]},{on:[{handler:'custom.extra'}]}]}}]};
describe('legacy stock Queue editor compatibility',()=>{
 it('binds the full selected prompt instead of its truncated preview without mutating layout or actions',()=>{
  const prompt='Full selected queued prompt '.repeat(30);
  const row={content:prompt,preview:prompt.slice(0,220)};
  const normalized=normalizeQueueEditorBinding('queue',legacy);
  const editor=normalized.containers[0];
  expect(Object.keys(editor.schemaBasedForm.schema.properties)).toEqual(['content']);
  const field=Object.keys(editor.schemaBasedForm.schema.properties)[0];
  expect(row[field]).toBe(prompt);
  expect(row[field].length).toBeGreaterThan(220);
  expect(editor.schemaBasedForm.schema.properties.content.title).toBe('Queued request');
  expect(editor.toolbar).toEqual(legacy.containers[0].toolbar);
  expect(legacy.containers[0].schemaBasedForm.schema.properties.preview).toBeDefined();
 });
 it('preserves explicitly authored content bindings and unrelated feeds/forms',()=>{
  const explicit=structuredClone(legacy);explicit.containers[0].schemaBasedForm.schema.properties.content={type:'string',title:'Custom content'};
  expect(normalizeQueueEditorBinding('queue',explicit)).toEqual(explicit);
  expect(normalizeQueueEditorBinding('other',legacy)).toBe(legacy);
  const custom=structuredClone(legacy);custom.containers[0].schemaBasedForm.id='customEditor';
  expect(normalizeQueueEditorBinding('queue',custom)).toEqual(custom);
  const customSave=structuredClone(legacy);customSave.containers[0].toolbar.items[0].on[0].handler='custom.save';
  expect(normalizeQueueEditorBinding('queue',customSave)).toBe(customSave);
 });
});
