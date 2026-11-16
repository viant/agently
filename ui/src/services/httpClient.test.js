import { beforeEach, describe, expect, it, vi } from 'vitest';

describe('redirectToLogin', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
  });

  it('signals unauthorized state without forcing an immediate oauth redirect', async () => {
    const dispatchEvent = vi.fn();
    const listeners = [];
    globalThis.window = {
      dispatchEvent,
      setTimeout: globalThis.setTimeout.bind(globalThis),
    };
    globalThis.document = {
      getElementById: () => null,
      createElement: () => ({
        style: {},
        appendChild: vi.fn(),
        remove: vi.fn(),
        textContent: '',
        id: ''
      }),
      body: {
        appendChild: (node) => listeners.push(node)
      }
    };
    globalThis.requestAnimationFrame = (cb) => { cb(); return 1; };
    globalThis.CustomEvent = class extends Event {
      constructor(name, init = {}) {
        super(name);
        this.detail = init.detail;
      }
    };

    const module = await import('./httpClient.js');
    module.redirectToLogin('/v1/agent/query');

    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    expect(dispatchEvent.mock.calls[0][0]?.type).toBe('agently:unauthorized');
    expect(listeners.length).toBe(1);
  });
});

describe('explicit notification ownership',()=>{
 it('retains typed diagnostics without creating a toast when notify is false',async()=>{
  vi.resetModules();
  const createElement=vi.fn();
  globalThis.document={getElementById:()=>null,createElement,body:{appendChild:vi.fn()}};
  const previousFetch=globalThis.fetch;
  globalThis.fetch=vi.fn(async()=>({ok:false,status:503,headers:{get:()=> 'application/json'},json:async()=>({error:'internal diagnostic'})}));
  try{
   const {request}=await import('./httpClient.js');
   await expect(request('/v1/example',{method:'PATCH',notify:false,retries:0})).rejects.toMatchObject({status:503,payload:{error:'internal diagnostic'}});
   expect(createElement).not.toHaveBeenCalled();
  }finally{globalThis.fetch=previousFetch;}
 });
});
