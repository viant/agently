// Stateful conformance check against the actual assembled backend and local model.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { randomUUID } from 'node:crypto';
const require=createRequire(new URL('../../../agently-core-ag-ui/sdk/ts/package.json',import.meta.url));
const {HttpAgent}=require('@ag-ui/client');
const {EventSchema}=require('@ag-ui/core/schemas');
const url=process.env.AGENTLY_AGUI_URL??'http://127.0.0.1:18083/v1/ag-ui/run';
for(let attempt=0;attempt<30;attempt++){
  try{if((await fetch(new URL('/healthz',url))).ok)break}catch{}
  if(attempt===29)throw new Error('Assembly backend did not become ready');
  await new Promise(resolve=>setTimeout(resolve,500));
}
let cookie='';
const requests=[];const streams=[];
const transport=async(url,init)=>{
  requests.push(JSON.parse(init.body));
  const response=await fetch(url,{...init,headers:{...init.headers,...(cookie?{cookie}:{})}});
  const set=response.headers.get('set-cookie');if(set)cookie=set.split(';')[0];
  if(!response.ok)throw new Error(`HTTP ${response.status}: ${await response.text()}`);
  streams.push(response.clone().text());return response;
};
const threadId=randomUUID();
const agent=new HttpAgent({url,fetch:transport,threadId,initialMessages:[{id:randomUUID(),role:'user',content:'fixture-client-tool please'}]});
const tools=[{name:'ui_lookup',description:'A browser-executed lookup',parameters:{type:'object',properties:{value:{type:'string'}},required:['value'],additionalProperties:false}}];
const forwardedProps={agently:{version:'1',operation:'chat',payload:{agentId:'simple',model:'local_mock'}}};
const firstRun=randomUUID();let failure;let outcome;
await agent.runAgent({runId:firstRun,tools,forwardedProps},{onRunErrorEvent:({event})=>{failure=event.message},onRunFinishedEvent:({event})=>{outcome=event.outcome}});
assert.equal(failure,undefined);assert.equal(outcome.pendingToolCallIds.length,1);
const publicCallId=outcome.pendingToolCallIds[0];
assert.match(publicCallId,/^agui\.tool\/[^/]+\/fixture-client-call$/);
const call=agent.messages.flatMap(m=>m.toolCalls??[]).find(c=>c.id===publicCallId);
assert.deepEqual(JSON.parse(call.function.arguments),{value:'client fixture'});
const firstInput=requests[0];
const replay=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify(firstInput)});
assert.equal(replay.status,200);const original=await streams[0];assert.equal(await replay.text(),original,'a retry must replay committed events');
const tampered=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify({...firstInput,state:{tampered:true}})});
assert.equal(tampered.status,409,'same run identity must reject different input');
agent.messages=[...agent.messages,{id:randomUUID(),role:'tool',toolCallId:call.id,content:'value returned by the browser'}];
failure=undefined;
await agent.runAgent({runId:randomUUID(),tools,forwardedProps},{onRunErrorEvent:({event})=>{failure=event.message}});
assert.equal(failure,undefined);
assert.ok(agent.messages.some(m=>m.role==='assistant'&&m.content==='Hello from the local Agently AG-UI assembly fixture.'));
for(const wire of await Promise.all(streams)){for(const frame of wire.split(/\r?\n\r?\n/)){const data=frame.split(/\r?\n/).filter(l=>l.startsWith('data:')).map(l=>l.slice(5).trimStart()).join('\n');if(data)EventSchema.parse(JSON.parse(data));}}
console.log('Actual client tool handoff, durable replay, conflict rejection, and continuation passed');
