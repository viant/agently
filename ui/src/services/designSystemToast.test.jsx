// @vitest-environment jsdom
import React,{act} from 'react';
import {expect,it,vi} from 'vitest';
import {showDesignSystemToast} from './designSystemToast.jsx';

it('uses the native themed Toast portal, announces failures and keeps keyboard focus',async()=>{
  vi.useFakeTimers();
  globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  const owner=document.documentElement;
  owner.classList.add('agently-application');
  owner.dataset.agentlyTheme='design-system-2';
  const field=document.createElement('input');document.body.append(field);field.focus();
  let dismiss;
  // The native create() promise resolves after React mounts the imperative owner.
  let ready;
  await act(async()=>{ready=showDesignSystemToast(owner,'Options unavailable',{intent:'danger',ttlMs:4000});await Promise.resolve();});
  await act(async()=>{dismiss=await ready;});
  const toast=document.querySelector('.bp6-toast');
  expect(toast).toBeTruthy();
  expect(toast.closest('.bp6-portal').parentElement).toBe(document.body);
  expect(toast.classList.contains('bp6-intent-danger')).toBe(true);
  expect(toast.querySelector('[role=alert]').textContent).toBe('Options unavailable');
  expect(toast.querySelector('svg')).toBeTruthy();
  expect(toast.querySelector('button[aria-label=Close]')).toBeNull();
  expect(document.activeElement).toBe(field);
  await act(async()=>vi.advanceTimersByTimeAsync(4000));
  expect(toast.classList.contains('bp6-toast-exit')).toBe(true);
  await act(async()=>vi.advanceTimersByTimeAsync(400));
  expect(document.querySelector('.bp6-toast')).toBeNull();
  await act(async()=>dismiss());
  vi.useRealTimers();
  field.remove();owner.classList.remove('agently-application');delete owner.dataset.agentlyTheme;
});
