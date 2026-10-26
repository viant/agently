import {useEffect, useState} from 'react';
import {client} from '../services/agentlyClient';
import {getConversationSeedTitle, resolveConversationTitle} from '../services/conversationTitle';

export function useConversationHeading(conversationId, enabled = true) {
  const [heading, setHeading] = useState({id: '', title: ''});
  useEffect(() => {
    if (!enabled || !conversationId) return undefined;
    let live = true;
    let updated = false;
    const update = event => {
      if (event.detail?.id !== conversationId) return;
      const title = event.detail.patch?.title || event.detail.title;
      if (title) {updated = true; setHeading({id: conversationId, title});}
    };
    client.getConversation(conversationId, {includeTranscript: false}).then(row => {
      if (live && !updated) setHeading({id: conversationId, title: resolveConversationTitle(row)});
    }).catch(() => {});
    window.addEventListener('agently:conversation-meta-updated', update);
    window.addEventListener('agently:conversation-title-seed', update);
    return () => {
      live = false;
      window.removeEventListener('agently:conversation-meta-updated', update);
      window.removeEventListener('agently:conversation-title-seed', update);
    };
  }, [conversationId, enabled]);
  return !conversationId ? 'New Chat' : heading.id === conversationId && heading.title
    ? heading.title : getConversationSeedTitle(conversationId) || 'Conversation';
}
