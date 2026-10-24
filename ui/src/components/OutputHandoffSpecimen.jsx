import React, {useEffect, useState} from 'react';
import {Typography} from 'forge/components';
import ChatFeedFromChatStore from './chat/ChatFeedFromChatStore.jsx';
import ConversationWorkspaceSurface from './ConversationWorkspaceSurface.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
import {onTranscript} from '../services/chatStore.js';
import {projectedWorkspaceEntries} from '../services/workspaceAttachment.js';

import {conversationId, rows, conversation} from './outputHandoffFixture.js';
const output = projectedWorkspaceEntries(rows,conversationId,{includeInlineReports:true})[0];

export default function OutputHandoffSpecimen() {
  const [headerMount, setHeaderMount] = useState(null);
  const [open, setOpen] = useState(false);
  const [closed, setClosed] = useState(false);
  const [visible, setVisible] = useState(false);
  const [mode, setMode] = useState('split');
  const [lifecycle, setLifecycle] = useState('ready');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {onTranscript(conversationId,conversation);},[]);
  const show = () => {if(lifecycle==='failed')setAttempt(value=>value+1);setOpen(true);setClosed(false);setVisible(true);};
  const entry = {...output, workspaceObject:{...output.workspaceObject,lifecycle:{state:closed?'closed':lifecycle,attempt}}};
  const chat = <ConversationViewContext.Provider value={{presentation:'document',reportsInWorkspace:true,onOpenWorkspace:show,workspaceWindows:[entry]}}>
    <ChatFeedFromChatStore conversationId={conversationId} rowsOverride={rows}/>
  </ConversationViewContext.Provider>;
  return <section id="foundation-output-handoff" className="app-output-specimen" aria-label="Output handoff">
    <Typography as="h2" typeRole="section">Output handoff</Typography>
    <div className="app-output-specimen-header" ref={setHeaderMount}/>
    <ConversationWorkspaceSurface presentation="document" headerMount={headerMount} workspaceWindow={open?entry:null} workspaceWindows={open?[entry]:[]}
      workspaceTabs={open?[{windowId:entry.windowId,label:'Delivery overview'}]:[]} activeSurface={visible?'workspace':'conversation'}
      workspaceMode={mode} onChangeWorkspaceMode={setMode} onBackToConversation={()=>setVisible(false)}
      onOpenWorkspace={show} onCloseWorkspaceTab={()=>{setOpen(false);setClosed(true);setVisible(false);}}
      onWorkspaceLifecycle={(_,state)=>setLifecycle(state)} savedOutputs={[entry]} conversationId={conversationId} onOpenSavedOutput={show}
      renderConversation={()=>chat} chatWindow={{windowId:'specimen-chat',windowKey:'chat/new'}}/>
  </section>;
}
