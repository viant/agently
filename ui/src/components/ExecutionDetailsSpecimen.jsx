import React, {useState} from 'react';
import {Dialog} from '@blueprintjs/core';
import {Button, Typography, ForgeThemeBoundary} from 'forge/components';
import DetailPanel from './DetailPanel.jsx';

const model = {id:'specimen-model',kind:'model',model:'Example model',provider:'Example provider',status:'completed',latencyMs:240,
  requestPayload:{messages:[{role:'user',content:'Inspect the example scope.'}]},
  responsePayload:{result:'Ready for review.',usage:{input_tokens:64,output_tokens:16,total_tokens:80}},
  streamPayload:Array.from({length:64},(_,index)=>({event:'text_delta',index,text:'Example output fragment.'})),
  providerRequestPayload:{model:'example-model',stream:true},providerResponsePayload:{status:'complete'}};
const tool = {id:'specimen-tool',kind:'tool',toolName:'example/read',status:'completed',latencyMs:80,requestPayload:{scope:'example'},responsePayload:false};

export default function ExecutionDetailsSpecimen() {
  const [step,setStep]=useState(null);
  const headingId=React.useId();
  return <section id="foundation-execution-details" aria-label="Execution inspector specimen">
    <Typography as="h2" typeRole="section">Execution inspector</Typography>
    <Typography as="p" typeRole="body">Neutral examples use the actual step inspector. Payload views stay in one dialog.</Typography>
    <Button variant="ghost" onClick={()=>setStep(model)}>Open model details</Button>
    <Button variant="ghost" onClick={()=>setStep(tool)}>Open tool details</Button>
    <Dialog isOpen={!!step} onClose={()=>setStep(null)} className="app-execution-dialog" aria-labelledby={headingId}>
      <div className="app-detail-dialog-shell"><ForgeThemeBoundary presentation="document" windowKey="execution-details-specimen">
        <DetailPanel toolCall={step} headingId={headingId} appearance="document" onClose={()=>setStep(null)}/>
      </ForgeThemeBoundary></div>
    </Dialog>
  </section>;
}
