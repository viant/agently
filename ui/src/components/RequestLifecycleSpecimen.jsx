import React from 'react';
import {Button, Typography, ReviewTable} from 'forge/components';
import RunProgressView, {RunNotice} from './RunProgressView.jsx';
import RequestReviewDialog, {ReviewAction} from './RequestReviewDialog.jsx';

export default function RequestLifecycleSpecimen() {
  const [state, setState] = React.useState('running');
  const [open, setOpen] = React.useState(false);
  const [startedAt] = React.useState(()=>new Date(Date.now()-14000).toISOString());
  const [rows,setRows] = React.useState([{name:'Example item',rationale:'Keep the evidence with this decision.',selected:true}]);
  return <section id="foundation-request-lifecycle" aria-label="Request lifecycle specimen">
    <Typography as="h2" typeRole="section">Request lifecycle</Typography>
    {state === 'running' ? <RunProgressView activity="Reviewing evidence" narration="Checking the selected scope." startedAt={startedAt} onStop={()=>setState('canceled')}/>
      : <RunNotice title="Request canceled" message="The current request was stopped."><Button variant="secondary" onClick={()=>setState('running')}>Review and retry</Button></RunNotice>}
    <RunNotice tone="danger" title="Request couldn’t be completed" message="Check your connection and review the request before retrying."/>
    <Button variant="secondary" onClick={()=>setOpen(true)}>Open example review</Button>
    <RequestReviewDialog appearance="document" isOpen={open} title="Review example action" onClose={()=>setOpen(false)}>
      <div className="app-approval-dialog"><Typography as="p" typeRole="body">Review the selection before deciding. This specimen performs no business action.</Typography>
        <ReviewTable id="example-approval" rows={rows} onRowsChange={setRows} columns={[{key:'name',label:'Item'},{key:'rationale',label:'Rationale',flex:true}]} showDownload={false}/>
        <ReviewTable id="example-locked-approval" title="Resolved review" rows={rows} columns={[{key:'name',label:'Item'}]} disabled showDownload={false}/>
        <div className="app-approval-dialog-actions"><ReviewAction appearance="document" onClick={()=>setOpen(false)}>Close example</ReviewAction></div>
      </div>
    </RequestReviewDialog>
  </section>;
}
