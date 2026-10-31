import React, {useEffect, useState} from 'react';
import {Button, Typography} from 'forge/components';
import ChatFeedFromChatStore from './chat/ChatFeedFromChatStore.jsx';
import ConversationWorkspaceSurface from './ConversationWorkspaceSurface.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
import {onTranscript, reset} from '../services/chatStore.js';
import {projectedWorkspaceEntries} from '../services/workspaceAttachment.js';

import {conversationId, rows, conversation, pendingRows, pendingConversation} from './outputHandoffFixture.js';
const output = projectedWorkspaceEntries(rows,conversationId,{includeInlineReports:true})[0];

export default function OutputHandoffSpecimen() {
  const [headerMount, setHeaderMount] = useState(null);
  const [open, setOpen] = useState(false);
  const [closed, setClosed] = useState(false);
  const [visible, setVisible] = useState(false);
  const [mode, setMode] = useState('split');
  const [lifecycle, setLifecycle] = useState('ready');
  const [attempt, setAttempt] = useState(0);
  const [preparing, setPreparing] = useState(false);
  useEffect(() => {onTranscript(conversationId,conversation);},[]);
  const show = () => {if(lifecycle==='failed')setAttempt(value=>value+1);setOpen(true);setClosed(false);setVisible(true);};
  const entry = {...output, workspaceObject:{...output.workspaceObject,lifecycle:{state:closed?'closed':lifecycle,attempt}}};
  const changePreparation = () => {
    setOpen(false);setVisible(false);setClosed(false);setLifecycle('ready');
    reset(conversationId);onTranscript(conversationId,preparing ? conversation : pendingConversation);
    setPreparing(value=>!value);
  };
  const chat = <ConversationViewContext.Provider value={{presentation:'document',reportsInWorkspace:true,onOpenWorkspace:show,workspaceWindows:preparing?[]:[entry]}}>
    <ChatFeedFromChatStore conversationId={conversationId} rowsOverride={preparing?pendingRows:rows}/>
  </ConversationViewContext.Provider>;
  return <section id="foundation-output-handoff" className="app-output-specimen" aria-label="Output handoff">
    <Typography as="h2" typeRole="section">Output handoff</Typography>
    <Button variant="ghost" size="compact" onClick={changePreparation}>{preparing?'Complete example report':'Prepare example report'}</Button>
    <div className="app-output-specimen-header" ref={setHeaderMount}/>
    <ConversationWorkspaceSurface presentation="document" headerMount={headerMount} workspaceWindow={open?entry:null} workspaceWindows={open?[entry]:[]}
      workspaceTabs={open?[{windowId:entry.windowId,label:'Delivery overview'}]:[]} activeSurface={visible?'workspace':'conversation'}
      workspaceMode={mode} onChangeWorkspaceMode={setMode} onBackToConversation={()=>setVisible(false)}
      onOpenWorkspace={show} onCloseWorkspaceTab={()=>{setOpen(false);setClosed(true);setVisible(false);}}
      onWorkspaceLifecycle={(_,state)=>setLifecycle(state)} savedOutputs={preparing?[]:[entry]} conversationId={conversationId} onOpenSavedOutput={show}
      renderConversation={()=>chat} chatWindow={{windowId:'specimen-chat',windowKey:'chat/new'}}/>
  </section>;
}
