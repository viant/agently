import {expect,it,vi} from 'vitest';
import {createPublishedWindowBindingRecovery} from './publishedWindowBindingRecovery.js';

it('coalesces parallel datasource readmissions into one mounted-window metadata refresh',async()=>{
  let release;
  const refresh=vi.fn(()=>new Promise(resolve=>{release=resolve;}));
  const recover=createPublishedWindowBindingRecovery({refresh,currentTransport:()=>({sessionId:'session-a'}),
    currentWindow:()=>({windowKey:'window://studio/connectors',providerIdentity:'studio'})});
  const first=recover({windowId:'window-a',sessionId:'session-a'}),second=recover({windowId:'window-a',sessionId:'session-a'});
  await Promise.resolve();
  expect(refresh).toHaveBeenCalledOnce();
  expect(refresh).toHaveBeenCalledWith({windowId:'window-a',reloadMetadata:true});
  release({ok:true});
  await Promise.all([first,second]);
});

it('rejects a changed UI session or aborted caller without replaying data',async()=>{
  let session='session-a',release;
  const refresh=vi.fn(()=>new Promise(resolve=>{release=resolve;}));
  const recover=createPublishedWindowBindingRecovery({refresh,currentTransport:()=>({sessionId:session}),
    currentWindow:()=>({windowKey:'window://studio/connectors',providerIdentity:'studio'})});
  const first=recover({windowId:'window-a',sessionId:'session-a'});
  await Promise.resolve();
  session='session-b';
  await expect(recover({windowId:'window-a',sessionId:'session-a'})).rejects.toThrow(/transport changed/);
  release({ok:true});
  await expect(first).rejects.toThrow(/transport changed/);
  const controller=new AbortController();controller.abort();
  await expect(recover({windowId:'window-b',sessionId:'session-b',signal:controller.signal})).rejects.toMatchObject({name:'AbortError'});
  expect(refresh).toHaveBeenCalledTimes(1);
});

it('rejects a closed or replaced window before releasing datasource replay',async()=>{
  let mounted={windowKey:'window://studio/connectors',providerIdentity:'studio'},release;
  const refresh=vi.fn(()=>new Promise(resolve=>{release=resolve;}));
  const recover=createPublishedWindowBindingRecovery({refresh,currentTransport:()=>({sessionId:'session-a'}),currentWindow:()=>mounted});
  const pending=recover({windowId:'window-a',sessionId:'session-a'});
  await Promise.resolve();
  mounted=null;release({ok:true});
  await expect(pending).rejects.toThrow(/window changed/);
  await expect(recover({windowId:'window-a',sessionId:'session-a'})).rejects.toThrow(/no longer mounted/);
});
