import { describe, it, expect, vi } from 'vitest';
import { resolveLayoutURL, openLayoutURL } from './layoutNavigation';

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
