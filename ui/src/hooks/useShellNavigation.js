import {useCallback, useEffect, useRef, useState} from 'react';

const KEY = 'agentlyShell';
const validSection = value => value === 'workspaces' || /^action:[\w-]+$/.test(value || '') ? value : 'chat';
const validDestination = (section, value) => section === 'workspaces' && typeof value === 'string' && value.length <= 1024 ? value : '';

// Own only entries created by this shell. Back never escapes the known trail.
export function useShellNavigation({enabled, section, destination = '', conversationId, onRestore}) {
  const restoreRef = useRef(onRestore);
  restoreRef.current = onRestore;
  const currentRef = useRef({section, destination, conversationId});
  currentRef.current = {section, destination, conversationId};
  const bounds = useRef({index: 0, end: 0});
  const restoring = useRef(false);
  const [position, setPosition] = useState(bounds.current);
  const publish = useCallback((nextSection, nextConversationId, pathname = window.location.pathname, destination = '') => {
    const nextDestination = validDestination(nextSection, destination);
    const url = new URL(window.location.href);
    url.pathname = pathname;
    if (nextSection === 'chat') url.searchParams.delete('section');
    else url.searchParams.set('section', nextSection);
    if (nextDestination) url.searchParams.set('destination', nextDestination);
    else url.searchParams.delete('destination');
    const current = window.history.state?.[KEY];
    if (current?.section === nextSection && current?.conversationId === nextConversationId && (current?.destination || '') === nextDestination && url.href === window.location.href) return;
    const index = bounds.current.index + 1;
    const entry = {index, section: nextSection, destination: nextDestination, conversationId: nextConversationId};
    window.history.pushState({...window.history.state, [KEY]: entry}, '', url);
    bounds.current = {index, end: index};
    setPosition(bounds.current);
  }, []);
  useEffect(() => {
    if (!enabled) return undefined;
    const query = new URL(window.location.href).searchParams;
    const initialSection = validSection(query.get('section'));
    const initial = {index: 0, section: initialSection, destination: validDestination(initialSection, query.get('destination')), conversationId: currentRef.current.conversationId};
    window.history.replaceState({...window.history.state, [KEY]: initial}, '');
    bounds.current = {index: 0, end: 0};
    setPosition(bounds.current);
    if (initial.section !== currentRef.current.section || initial.destination !== currentRef.current.destination) restoreRef.current?.(initial);
    const onPath = event => {
      event.preventDefault();
      if (restoring.current) return;
      publish('chat', event.detail.conversationId, event.detail.pathname);
      restoreRef.current?.({section: 'chat', destination: '', conversationId: event.detail.conversationId, selectionInProgress: true});
    };
    const onPop = event => {
      const entry = event.state?.[KEY];
      if (!entry) return;
      bounds.current = {...bounds.current, index: entry.index};
      setPosition(bounds.current);
      restoring.current = true;
      try {restoreRef.current?.(entry);} finally {restoring.current = false;}
    };
    const onChatLocation = () => {
      const index = bounds.current.index + 1;
      const entry = {index, ...currentRef.current};
      window.history.replaceState({...window.history.state, [KEY]: entry}, '');
      bounds.current = {index, end: index}; setPosition(bounds.current);
    };
    window.addEventListener('forge:chat-location', onChatLocation);
    window.addEventListener('agently:conversation-path', onPath);
    window.addEventListener('popstate', onPop);
    return () => {
      window.removeEventListener('agently:conversation-path', onPath);
      window.removeEventListener('popstate', onPop);
      window.removeEventListener('forge:chat-location', onChatLocation);
    };
  }, [enabled, publish]);
  const selectSection = useCallback((next, destination = '') => {
    const nextDestination = validDestination(next, destination);
    if (enabled) publish(next, currentRef.current.conversationId, window.location.pathname, nextDestination);
    restoreRef.current?.({section: next, destination: nextDestination, conversationId: currentRef.current.conversationId, selectionInProgress: true});
  }, [enabled, publish]);
  return {selectSection, canGoBack: position.index > 0, canGoForward: position.index < position.end,
    back: () => {if (bounds.current.index > 0) window.history.back();},
    forward: () => {if (bounds.current.index < bounds.current.end) window.history.forward();}};
}
