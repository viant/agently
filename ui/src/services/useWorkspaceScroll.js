import {useEffect, useRef} from 'react';
import {getViewSignal} from 'forge/core';

// Capture renderer scroll containers without coupling the shell to their CSS or
// datasource grammar. Paths are relative to a stable, object-owned renderer root.
export function useWorkspaceScroll(windowId, active) {
  const ref = useRef(null);
  useEffect(() => {
    const root = ref.current;
    if (!root || !windowId || !active) return undefined;
    const view = getViewSignal(windowId);
    const saved = view.peek()?.workspaceScrollContainers || {};
    const pending = new Set(Object.keys(saved));
    const scope = () => [...root.children].find((element) => element.dataset.workspaceRendererId === windowId) || root;
    const find = (path) => path ? path.split('.').reduce((element, part) => element?.children?.[Number(part)], scope()) : scope();
    const pathFor = (element) => {
      const parts = [];
      const owner = scope();
      let node = element;
      while (node && node !== owner) {
        if (!node.parentElement) return null;
        parts.unshift([...node.parentElement.children].indexOf(node));
        node = node.parentElement;
      }
      return node === owner ? parts.join('.') : null;
    };
    let observer;
    const restore = () => {
      for (const path of pending) {
        const element = find(path);
        if (!element || !element.getClientRects().length) continue;
        const position = saved[path];
        if (element.scrollHeight - element.clientHeight < position.top || element.scrollWidth - element.clientWidth < position.left) continue;
        element.scrollLeft = position.left;
        element.scrollTop = position.top;
        pending.delete(path);
      }
      if (pending.size === 0) observer?.disconnect();
    };
    const save = (event) => {
      const element = event.target;
      if (!element?.getClientRects?.().length) return;
      const path = pathFor(element);
      if (path == null || pending.has(path)) return;
      const current = view.peek() || {};
      const next = {left: element.scrollLeft, top: element.scrollTop};
      const previous = current.workspaceScrollContainers?.[path];
      if (previous?.left !== next.left || previous?.top !== next.top) {
        view.value = {...current, workspaceScrollContainers: {...current.workspaceScrollContainers, [path]: next}};
      }
    };
    const takeControl = () => {pending.clear(); observer?.disconnect();};
    observer = new MutationObserver(restore);
    observer.observe(root, {subtree: true, childList: true, attributes: true});
    const frame = requestAnimationFrame(restore);
    root.addEventListener('scroll', save, {passive: true, capture: true});
    root.addEventListener('wheel', takeControl, {passive: true});
    root.addEventListener('pointerdown', takeControl);
    root.addEventListener('keydown', takeControl);
    return () => {
      cancelAnimationFrame(frame); observer.disconnect();
      root.removeEventListener('scroll', save, true);
      root.removeEventListener('wheel', takeControl);
      root.removeEventListener('pointerdown', takeControl);
      root.removeEventListener('keydown', takeControl);
    };
  }, [windowId, active]);
  return ref;
}
