import React from 'react';
import {Button, Typography, ReviewTable, ForgeThemeBoundary} from 'forge/components';
import RunProgressView, {RunNotice} from './RunProgressView.jsx';
import RequestReviewDialog, {ReviewAction} from './RequestReviewDialog.jsx';
import IterationBlock from './chat/IterationBlock.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
import Composer from 'forge/components/chat/Composer.jsx';

export default function RequestLifecycleSpecimen() {
  const [state, setState] = React.useState('running');
  const [open, setOpen] = React.useState(false);
  const [startedAt,setStartedAt] = React.useState(()=>new Date(Date.now()-14000).toISOString());
  const [completed,setCompleted] = React.useState(2);
  const [rows,setRows] = React.useState([{name:'Example item',rationale:'Keep the evidence with this decision.',selected:true}]);
  return <ForgeThemeBoundary presentation="document" windowKey="request-lifecycle-specimen"><section id="foundation-request-lifecycle" aria-label="Request lifecycle specimen">
    <Typography as="h2" typeRole="section">Request lifecycle</Typography>
    <Typography as="p" typeRole="body">Illustrative progress. Leave this example unchanged for 30 seconds to see its last-update notice.</Typography>
    <ConversationViewContext.Provider value={{presentation:'document',developerMode:true}}>
      <IterationBlock canonicalRow={{kind:'iteration',turnId:'example-activity',lifecycle:state,turnStartedAt:startedAt,rounds:[{pageId:'example-step',phase:'main',status:state,toolCalls:Array.from({length:6},(_,index)=>({toolCallId:`example-check-${index}`,toolName:`example/check-${index+1}`,status:index<5?'completed':state}))}]}}
        suppressBubble showToolFeedDetail={false} progress={state==='running' ? <RunProgressView activity="Reviewing evidence" narration={'### Progress update\n- **Reviewing evidence:** checking the selected scope.\n- Comparing `requested` and `available` data.'} summary={`${completed} steps completed · 1 step running`} progressKey={completed} startedAt={startedAt}/> : null}/>
      </ConversationViewContext.Provider>
      {state==='running' ? <><Button variant="ghost" size="compact" onClick={()=>setCompleted(value=>value+1)}>Complete example step</Button>
      <Button variant="ghost" size="compact" onClick={()=>setState('completed')}>Complete example request</Button></> : null}
    {state==='canceled' ? <RunNotice title="Request canceled" message="The current request was stopped."><Button variant="secondary" onClick={()=>{setStartedAt(new Date().toISOString());setCompleted(0);setState('running');}}>Review and retry</Button></RunNotice> : null}
    <div className="forge-document-chat"><Composer appearance="document" draftKey="request-lifecycle-example" showAbort={state==='running'} onAbort={()=>setState('canceled')}/></div>
    <RunNotice tone="danger" title="Request couldn’t be completed" message="Check your connection and review the request before retrying."/>
    <Button variant="secondary" onClick={()=>setOpen(true)}>Open example review</Button>
    <RequestReviewDialog appearance="document" isOpen={open} title="Review example action" onClose={()=>setOpen(false)}>
      <div className="app-approval-dialog"><Typography as="p" typeRole="body">Review the selection before deciding. This specimen performs no business action.</Typography>
        <ReviewTable id="example-approval" rows={rows} onRowsChange={setRows} columns={[{key:'name',label:'Item'},{key:'rationale',label:'Rationale',flex:true}]} showDownload={false}/>
        <ReviewTable id="example-locked-approval" title="Resolved review" rows={rows} columns={[{key:'name',label:'Item'}]} disabled showDownload={false}/>
        <div className="app-approval-dialog-actions"><ReviewAction appearance="document" onClick={()=>setOpen(false)}>Close example</ReviewAction></div>
      </div>
    </RequestReviewDialog>
  </section></ForgeThemeBoundary>;
}
