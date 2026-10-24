import React, {useEffect, useState} from 'react';
import {Button, Typography} from 'forge/components';
import {client} from '../services/agentlyClient';
import {openConversationInMainWindow} from '../services/conversationWindow';
import {resolveConversationTitle} from '../services/conversationTitle';
import {subscribeWorkspaceMetadata} from '../services/workspaceMetadata';
import {conversationTimestamp, normalizeSidebarPage} from './Sidebar';
import './homeRecentConversations.css';

const fetchRecent = () => client.listConversations({excludeScheduled: true, page: {limit: 3, direction: 'latest'}});

export default function HomeRecentConversations({loadConversations = fetchRecent}) {
  const [state, setState] = useState({rows: [], loading: true, error: false});
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true, sequence = 0;
    const load = async () => {
      const request = ++sequence;
      setState({rows: [], loading: true, error: false});
      try {
        const page = await loadConversations();
        if (active && request === sequence) setState({rows: normalizeSidebarPage(page).rows.slice(0, 3), loading: false, error: false});
      } catch (_) {
        if (active && request === sequence) setState({rows: [], loading: false, error: true});
      }
    };
    void load();
    const events = ['agently:conversation-new', 'agently:conversation-meta-updated', 'agently:conversation-activity'];
    events.forEach(event => window.addEventListener(event, load));
    const unsubscribe = subscribeWorkspaceMetadata(load);
    return () => {active = false; sequence++; unsubscribe(); events.forEach(event => window.removeEventListener(event, load));};
  }, [retry, loadConversations]);
  if (state.loading) return <Typography className="app-home-recent-note" as="p" typeRole="small" role="status">Loading recent conversations…</Typography>;
  if (state.error) return <div className="app-home-recent-note"><Typography as="p" typeRole="small" role="status">Recent conversations couldn’t load.</Typography><Button variant="link" onClick={() => setRetry(value => value + 1)}>Try again</Button></div>;
  if (!state.rows.length) return <Typography className="app-home-recent-note" as="p" typeRole="small">Your work stays in Conversations so you can come back to it.</Typography>;
  return <section className="app-home-recent" aria-label="Recent conversations">
    <div className="app-home-recent-heading"><Typography as="h2" typeRole="section">Pick up where you left off</Typography><Typography as="span" typeRole="small">Recent conversations</Typography></div>
    {state.rows.map(row => {
      const id = String(row.id || row.Id || ''), time = conversationTimestamp(row);
      const running = row.running || row.Running || ['running', 'processing'].includes(String(row.status || row.Status || '').toLowerCase());
      const label = running ? 'Preparing' : time ? new Intl.DateTimeFormat('en-US', {month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short'}).format(time) : '';
      return <a key={id} href={`/conversation/${encodeURIComponent(id)}`} onClick={event => {
        if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
        event.preventDefault(); openConversationInMainWindow(id);
      }}><Typography as="span" typeRole="body">{resolveConversationTitle(row)}</Typography><Typography as="small" typeRole="small">{label}</Typography></a>;
    })}
  </section>;
}
