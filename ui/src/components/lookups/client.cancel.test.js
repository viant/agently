import {afterEach,describe,expect,it,vi} from 'vitest';
import {fetchDatasource} from './client.js';

afterEach(()=>{vi.unstubAllGlobals();vi.useRealTimers();});
describe('authenticated datasource cancellation',()=>{
  it('keeps cancellation attached until the response body finishes',async()=>{
    const controller=new AbortController();
    let requestSignal;
    let bodyReady;
    const ready=new Promise(resolve=>{bodyReady=resolve;});
    vi.stubGlobal('fetch',vi.fn(async(_url,request)=>{
      requestSignal=request.signal;
      expect(request.credentials).toBe('include');
      expect(JSON.parse(request.body)).toEqual({inputs:{limit:1}});
      return {ok:true,json:()=>{
        bodyReady();
        return new Promise((resolve,reject)=>request.signal.addEventListener('abort',()=>reject(new DOMException('cancelled','AbortError')),{once:true}));
      }};
    }));
    const result=fetchDatasource('report',{limit:1},{signal:controller.signal});
    await ready; controller.abort();
    await expect(result).rejects.toMatchObject({name:'AbortError'});
    expect(requestSignal.aborted).toBe(true);
  });

  it('does not impose a short timeout on report requests',async()=>{
    vi.useFakeTimers();
    const controller=new AbortController();
    vi.stubGlobal('fetch',vi.fn((_url,request)=>new Promise((resolve,reject)=>
      request.signal.addEventListener('abort',()=>reject(new DOMException('cancelled','AbortError')),{once:true}))));
    const result=fetchDatasource('report',{}, {signal:controller.signal});
    const rejected=expect(result).rejects.toMatchObject({name:'AbortError'});
    await vi.advanceTimersByTimeAsync(65000);
    expect(controller.signal.aborted).toBe(false);
    controller.abort(); await rejected;
  });
});
