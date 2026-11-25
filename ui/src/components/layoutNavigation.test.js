import { describe, it, expect, vi } from 'vitest';
import { resolveLayoutURL, openLayoutURL, resolveLayoutLabels, workspaceMenuKey } from './layoutNavigation';

describe('workspace-owned theme navigation labels', () => {
 const first = {id:'example',providerIdentity:'first',title:'Example',menus:[{id:'group',title:'Original group',children:[{id:'records',title:'Original records',action:{type:'window',windowKey:'recordList'}}]}]};
 const second = {...first,providerIdentity:'second'};
 const groupKey = workspaceMenuKey(first,['group']);
 const recordKey = workspaceMenuKey(first,['group','records']);
 const layout = {left:{navigation:{labelsByTheme:{'design-system-2':{[groupKey]:'Records',[recordKey]:'All records'}}}},applications:[first,second]};

 it('applies exact theme, provider and full menu-path identities without changing source or actions', () => {
  const original=JSON.stringify(layout);
  const projected=resolveLayoutLabels(layout,'design-system-2');
  expect(projected.applications[0].menus[0].title).toBe('Records');
  expect(projected.applications[0].menus[0].children[0].title).toBe('All records');
  expect(projected.applications[0].menus[0].children[0].action).toBe(first.menus[0].children[0].action);
  expect(projected.applications[1]).toBe(second);
  expect(JSON.stringify(layout)).toBe(original);
 });
 it.each(['baseline','legacy','',undefined])('retains the original layout for %s', theme => {
  expect(resolveLayoutLabels(layout,theme)).toBe(layout);
 });
 it('retains explicit personal names and falls back to theme defaults after reset', () => {
  const preferences={menuLabels:{[groupKey]:'My records'}};
  expect(resolveLayoutLabels(layout,'design-system-2',preferences).applications[0].menus[0].title).toBe('My records');
  expect(resolveLayoutLabels(layout,'baseline',preferences).applications[0].menus[0].title).toBe('My records');
  expect(resolveLayoutLabels(layout,'design-system-2',{}).applications[0].menus[0].title).toBe('Records');
 });
});

describe('layout URL actions', () => {
 it.each(['https://studio.example/?section=models','http://localhost:8080/','http://127.0.0.1:8080/','http://[::1]:8080/'])('opens %s safely', (url) => {
  const open = vi.fn(); vi.stubGlobal('window', {open});
  expect(openLayoutURL({type:'url',url})).toBe(true);
  expect(open).toHaveBeenCalledWith(new URL(url).href,'_blank','noopener,noreferrer');
  vi.unstubAllGlobals();
 });
 it.each(['javascript:alert(1)','data:text/html,hello','http://studio.example/','https://user:pass@studio.example/','//studio.example/',' https://studio.example/','https://studio.example\\@evil.example/'])('rejects %s', (url) => {
  expect(resolveLayoutURL({type:'url',url})).toBe('');
 });
});
