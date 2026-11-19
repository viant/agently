// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest';
import { attachHistoryScroll } from './historyScroll.js';

function fixture(conversationId = '', feed = {}) {
  const listeners = new Map();
  const frames = new Map();
  let sequence = 0;
  let resize;
  const viewport = {
    scrollTop: 0, scrollHeight: 1200, clientHeight: 400,
    getBoundingClientRect: () => ({top: 0, bottom: 400}),
    addEventListener: (name, callback) => listeners.set(name, callback),
    removeEventListener: name => listeners.delete(name),
  };
  viewport.closest = () => viewport;
  const disconnect = vi.fn();
  const environment = {
    requestAnimationFrame: callback => { frames.set(++sequence, callback); return sequence; },
    cancelAnimationFrame: id => frames.delete(id),
    ResizeObserver: class {
      constructor(callback) { resize = callback; }
      observe = vi.fn();
      disconnect = disconnect;
    },
  };
  const dispose = attachHistoryScroll(feed, viewport, environment, conversationId);
  const flush = () => {
    const pending = [...frames.values()]; frames.clear();
    for (const callback of pending) callback();
    viewport.scrollTop = Math.min(viewport.scrollTop, viewport.scrollHeight - viewport.clientHeight);
    listeners.get('scroll')?.({});
  };
  return { viewport, listeners, frames, dispose, flush, resize: () => resize(), disconnect };
}

describe('history viewport scrolling', () => {
  it('holds reading position when a disclosure is activated and resumes on a new submission', () => {
    const feed={dataset:{requestActive:'false'}},f=fixture('',feed);f.flush();
    expect(f.viewport.scrollTop).toBe(800);
    f.viewport.scrollHeight=1800;f.resize();
    f.listeners.get('click')({target:{closest:()=>({})}});f.flush();
    expect(f.viewport.scrollTop).toBe(800);
    f.viewport.scrollHeight=2400;f.resize();f.flush();
    expect(f.viewport.scrollTop).toBe(800);
    f.listeners.get('forge:chat-submitted')();feed.dataset.requestActive='true';f.flush();
    expect(f.viewport.scrollTop).toBe(2000);f.dispose();
  });
  it.each([
    ['receipt title', '#receipt-title', 1400],
    ['ordinary text', '#body', 1400],
    ['interactive section title', '#disclosure-title', 800],
    ['disclosure caret', 'button span', 800],
  ])('distinguishes %s from an interactive disclosure', (_label, selector, expectedTop) => {
    const host=document.createElement('div');
    host.innerHTML='<div data-forge-part="section-header"><h3 id="receipt-title">Source checked</h3></div><p id="body">Source observation</p><div data-forge-part="section-header" class="bp6-interactive"><h3 id="disclosure-title">Details</h3><button aria-expanded="false"><span>Expand</span></button></div>';
    const f=fixture();f.flush();
    f.listeners.get('click')({target:host.querySelector(selector)});
    f.viewport.scrollHeight=1800;f.resize();f.flush();
    expect(f.viewport.scrollTop).toBe(expectedTop);f.dispose();
  });
  it.each([false, true])('reveals the start of the current answer and holds it, delayed mount=%s', delayed => {
    const feed=document.createElement('div');feed.dataset.requestActive='true';
    feed.innerHTML='<div class="app-bubble-row app-bubble-row-assistant">Old answer</div><div class="app-bubble-row app-bubble-row-user">New question</div><div class="app-bubble-row app-bubble-row-assistant" id="answer">New answer</div><div class="app-bubble-row app-bubble-row-assistant">Report</div>';
    let height=delayed?0:1000;
    feed.querySelector('#answer').getBoundingClientRect=()=>({top:700,bottom:700+height,height});
    const f=fixture('',feed);f.flush();
    feed.dataset.requestActive='false';f.viewport.scrollHeight=2200;f.resize();f.flush();
    if(delayed){expect(f.viewport.scrollTop).toBe(800);height=1000;f.resize();f.flush();}
    expect(f.viewport.scrollTop).toBe(1500);
    f.viewport.scrollHeight=3000;f.resize();f.flush();expect(f.viewport.scrollTop).toBe(1500);
    f.listeners.get('forge:chat-submitted')();feed.dataset.requestActive='true';f.flush();
    expect(f.viewport.scrollTop).toBe(2600);f.dispose();
  });
  it('does not reposition someone reading when work completes', () => {
    const feed={dataset:{requestActive:'true'}},f=fixture('',feed);f.flush();
    f.listeners.get('wheel')({deltaY:-100});f.viewport.scrollTop=400;f.listeners.get('scroll')({});
    feed.dataset.requestActive='false';f.viewport.scrollHeight=2400;f.resize();f.flush();
    expect(f.viewport.scrollTop).toBe(400);f.dispose();
  });
  it.each([
    [{top:400,bottom:600,height:200},1200],
    [{top:50,bottom:300,height:250},800],
  ])('holds a short finished answer through later resize: %o', (bounds,expectedTop) => {
    const feed=document.createElement('div');feed.dataset.requestActive='true';
    feed.innerHTML='<div class="app-bubble-row app-bubble-row-user">Question</div><div class="app-bubble-row app-bubble-row-assistant">Answer</div>';
    feed.lastChild.getBoundingClientRect=()=>bounds;
    const f=fixture('',feed);f.flush();
    feed.dataset.requestActive='false';f.viewport.scrollHeight=1600;f.resize();f.flush();
    expect(f.viewport.scrollTop).toBe(expectedTop);
    f.viewport.scrollHeight=2400;f.resize();f.flush();expect(f.viewport.scrollTop).toBe(expectedTop);f.dispose();
  });
  it('opens at the bottom and follows late rendering and viewport resizing', () => {
    const f = fixture(); f.flush();
    expect(f.viewport.scrollTop).toBe(800);
    f.viewport.scrollHeight = 1800; f.resize(); f.flush();
    expect(f.viewport.scrollTop).toBe(1400);
    f.viewport.clientHeight = 250; f.resize(); f.flush();
    expect(f.viewport.scrollTop).toBe(1550);
    f.dispose();
  });

  it('cancels pending following immediately when the user scrolls upward', () => {
    const f = fixture(); f.flush();
    f.viewport.scrollHeight = 1800; f.resize();
    f.listeners.get('wheel')({ deltaY: -100 });
    f.viewport.scrollTop = 500; f.listeners.get('scroll')({}); f.flush();
    expect(f.viewport.scrollTop).toBe(500);
    f.resize(); f.flush();
    expect(f.viewport.scrollTop).toBe(500);
    f.viewport.scrollTop = 1400; f.listeners.get('scroll')({});
    f.viewport.scrollHeight = 2000; f.resize(); f.flush();
    expect(f.viewport.scrollTop).toBe(1600);
    f.dispose();
  });

  it('resets following on navigation and cleans up old observers and frames', () => {
    const f = fixture();
    f.dispose();
    expect(f.frames.size).toBe(0);
    expect(f.listeners.size).toBe(0);
    expect(f.disconnect).toHaveBeenCalledOnce();
    const next = fixture(); next.flush();
    expect(next.viewport.scrollTop).toBe(800);
    next.dispose();
  });
});


it('restores a chat reading position and does not pull it down on late rendering', () => {
  const first = fixture('saved-reading'); first.flush();
  first.listeners.get('wheel')({deltaY: -100}); first.viewport.scrollTop = 400;
  first.listeners.get('scroll')({}); first.dispose();
  const second = fixture('saved-reading'); second.flush(); expect(second.viewport.scrollTop).toBe(400);
  second.viewport.scrollHeight = 2400; second.resize(); second.flush(); expect(second.viewport.scrollTop).toBe(400);
  second.dispose();
});
