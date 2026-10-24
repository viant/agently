import { describe, it, expect, vi } from 'vitest';
import { attachHistoryScroll } from './historyScroll.js';

function fixture(conversationId = '') {
  const listeners = new Map();
  const frames = new Map();
  let sequence = 0;
  let resize;
  const viewport = {
    scrollTop: 0, scrollHeight: 1200, clientHeight: 400,
    addEventListener: (name, callback) => listeners.set(name, callback),
    removeEventListener: name => listeners.delete(name),
  };
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
  const dispose = attachHistoryScroll({}, viewport, environment, conversationId);
  const flush = () => {
    const pending = [...frames.values()]; frames.clear();
    for (const callback of pending) callback();
    viewport.scrollTop = Math.min(viewport.scrollTop, viewport.scrollHeight - viewport.clientHeight);
    listeners.get('scroll')?.({});
  };
  return { viewport, listeners, frames, dispose, flush, resize: () => resize(), disconnect };
}

describe('history viewport scrolling', () => {
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
