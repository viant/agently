// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, describe, expect, it, vi} from 'vitest';
import NamedLookupInput from './NamedLookupInput.jsx';
import {getMetadataSignal} from 'forge/core';

globalThis.IS_REACT_ACT_ENVIRONMENT=true;
afterEach(()=>{vi.restoreAllMocks();vi.unstubAllGlobals();document.body.innerHTML='';});
const context=(owner,authority='alice')=>({metadata:{resource:{uri:owner,kind:'working',providerIdentity:'owned-provider',authorityBinding:authority}},handlers:{window:{}}});
const response=entries=>new Response(JSON.stringify({entries}),{headers:{'Content-Type':'application/json'}});
const entry=name=>({name,dataSource:'datasource://owned/records'});
async function mount(ctx,loaded){const node=document.createElement('div');document.body.append(node);const root=createRoot(node);const render=async ctx=>act(async()=>root.render(<NamedLookupInput value='' context={ctx} contextKind='template' contextID='form' autoResolveAuthored={false} onRegistryLoaded={loaded}/>));await render(ctx);return {root,render};}

describe('mounted lookup owner forwarding',()=>{
 it('forwards the current canonical owner without adding auth or authority claims',async()=>{
  const fetch=vi.fn(async()=>response([entry('account')]));vi.stubGlobal('fetch',fetch);
  const loaded=vi.fn();const fixture=await mount(context('template://owned/form'),loaded);
  try {expect(fetch).toHaveBeenCalledTimes(1);const [url,options]=fetch.mock.calls[0];const query=new URL(url,'http://fixture.invalid').searchParams;
   expect(query.get('context')).toBe('template:form');expect(query.get('owner')).toBe('template://owned/form');expect(query.get('revision')).toBe('working');expect(query.get('providerIdentity')).toBe('owned-provider');expect(query.has('authorityBinding')).toBe(false);expect(options).toEqual({credentials:'include'});expect(loaded).toHaveBeenLastCalledWith([entry('account')]);
  }finally{await act(async()=>fixture.root.unmount());}
 });
 it('discards a late response after switching owning primitive',async()=>{
  const pending=[];vi.stubGlobal('fetch',vi.fn(()=>new Promise(resolve=>pending.push(resolve))));const loaded=vi.fn();const fixture=await mount(context('template://alice/form'),loaded);
  try {await fixture.render(context('template://bob/form','bob'));expect(pending).toHaveLength(2);
   await act(async()=>pending[1](response([entry('bob')])));expect(loaded).toHaveBeenLastCalledWith([entry('bob')]);
   await act(async()=>pending[0](response([entry('alice')])));expect(loaded).toHaveBeenCalledTimes(1);expect(loaded).toHaveBeenLastCalledWith([entry('bob')]);
  }finally{await act(async()=>fixture.root.unmount());}
 });
 it('revalidates same-URI actor scope and clears registry on private denial',async()=>{
  const fetch=vi.fn().mockResolvedValueOnce(response([entry('alice')])).mockResolvedValueOnce(new Response('',{status:403,statusText:'Forbidden'}));vi.stubGlobal('fetch',fetch);vi.spyOn(console,'warn').mockImplementation(()=>{});
  const loaded=vi.fn();const fixture=await mount(context('template://owned/form'),loaded);
  try {expect(loaded).toHaveBeenLastCalledWith([entry('alice')]);await fixture.render(context('template://owned/form','bob'));expect(fetch).toHaveBeenCalledTimes(2);expect(loaded).toHaveBeenLastCalledWith([]);
  }finally{await act(async()=>fixture.root.unmount());}
 });
 it('uses current metadata signal and never falls back to removed private metadata',async()=>{
  const signal=getMetadataSignal('lookup-owner-source-fixture');signal.value={resource:{uri:'template://current/form',kind:'working',providerIdentity:'owned-provider'}};
  const fetch=vi.fn(async()=>response([]));vi.stubGlobal('fetch',fetch);
  const stale=context('template://stale/form');stale.identity={windowId:'lookup-owner-source-fixture'};
  const fixture=await mount(stale,vi.fn());
  try {expect(new URL(fetch.mock.calls[0][0],'http://fixture.invalid').searchParams.get('owner')).toBe('template://current/form');
   await act(async()=>{signal.value={resource:{uri:'template://new/form',kind:'working',providerIdentity:'owned-provider'}};});
   expect(fetch).toHaveBeenCalledTimes(2);expect(new URL(fetch.mock.calls[1][0],'http://fixture.invalid').searchParams.get('owner')).toBe('template://new/form');
   await act(async()=>{signal.value=null;});
   expect(fetch).toHaveBeenCalledTimes(3);expect(new URL(fetch.mock.calls[2][0],'http://fixture.invalid').searchParams.has('owner')).toBe(false);
  }finally{await act(async()=>fixture.root.unmount());signal.value=null;}
 });
 it('keeps legacy unowned context without guessing an owner',async()=>{
  const fetch=vi.fn(async()=>response([]));vi.stubGlobal('fetch',fetch);const fixture=await mount({windowState:{windowKey:'private-guess'},metadata:{},handlers:{window:{}}},vi.fn());
  try {const query=new URL(fetch.mock.calls[0][0],'http://fixture.invalid').searchParams;expect(query.get('context')).toBe('template:form');expect(query.has('owner')).toBe(false);}
  finally{await act(async()=>fixture.root.unmount());}
 });
});
