// Actual provider usage -> native goal runtime/publisher -> independent AG-UI SSE.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {randomUUID} from 'node:crypto';
const require=createRequire(new URL('../../../agently-core-ag-ui/sdk/ts/package.json',import.meta.url));
const {HttpAgent}=require('@ag-ui/client');const {EventSchema}=require('@ag-ui/core/schemas');
const url=process.env.AGENTLY_AGUI_URL??'http://127.0.0.1:20475/v1/ag-ui/run';
for(let attempt=0;attempt<30;attempt++){try{if((await fetch(new URL('/healthz',url))).ok)break}catch{}if(attempt===29)throw new Error('Assembly backend did not become ready');await new Promise(resolve=>setTimeout(resolve,500));}
let cookie='';const wires=[];
const transport=async(input,init)=>{const response=await fetch(input,{...init,headers:{...init.headers,...(cookie?{cookie}:{})}});if(!response.ok)throw new Error(`HTTP ${response.status}: ${await response.text()}`);const set=response.headers.get('set-cookie');if(set)cookie=set.split(';')[0];wires.push(response.clone().text());return response;};
const threadId=randomUUID();const forwardedProps={agently:{version:'1',operation:'chat',payload:{agentId:'simple',model:'local_mock'}}};
const chat=new HttpAgent({url,fetch:transport,threadId,initialMessages:[{id:randomUUID(),role:'user',content:'initialize automatic goal accounting'}]});
const fail={onRunErrorEvent:({event})=>{throw new Error(event.message)}};await chat.runAgent({forwardedProps},fail);
async function command(operation,payload={}){const agent=new HttpAgent({url,fetch:transport,threadId});const events=[];await agent.runAgent({runId:randomUUID(),forwardedProps:{agently:{version:'1',requestId:randomUUID(),operation,payload}}},{...fail,onEvent:({event})=>events.push(event)});return events;}
const created=(await command('goal.create',{objective:'prove automatic native accounting notification',tokenBudget:10,controllerSpec:{continueMode:'idle_only',onTurnFinished:'wait',onAsyncCompleted:'wait'}})).find(e=>e.type==='CUSTOM').value.result.goal;assert.equal(created.tokensUsed,0);
const observer=new HttpAgent({url,fetch:transport,threadId});const subscriptionRun=randomUUID();const snapshots=[];let ready;const first=new Promise(resolve=>{ready=resolve});
const watching=observer.runAgent({runId:subscriptionRun,forwardedProps:{agently:{version:'1',requestId:randomUUID(),operation:'goal.subscribe',payload:{durationSeconds:30}}}},{...fail,onActivitySnapshotEvent:({event})=>{snapshots.push(event.content.goal);ready();}});
await Promise.race([first,new Promise((_,reject)=>setTimeout(()=>reject(new Error('initial goal snapshot missing')),5000))]);
chat.messages=[...chat.messages,{id:randomUUID(),role:'user',content:'fixture-goal-accounting please'}];await chat.runAgent({runId:randomUUID(),forwardedProps},fail);
// No management mutation or synthetic event triggers this authoritative refresh.
for(let i=0;i<50&&!snapshots.some(g=>g?.status==='budget_limited'&&g.tokensUsed===15);i++)await new Promise(resolve=>setTimeout(resolve,100));
const accounted=snapshots.find(g=>g?.status==='budget_limited'&&g.tokensUsed===15);assert.ok(accounted,`native completed-turn publisher must automatically project committed accounting/budget status; snapshots=${JSON.stringify(snapshots)}`);assert.equal(accounted.id,created.id);assert.equal(accounted.tokenBudget,10);assert.ok(accounted.statusReason);assert.ok(accounted.timeUsedSeconds>=1,'actual delayed provider turn must account at least one elapsed second');
await command('run.cancel',{runId:subscriptionRun});await watching;
let count=0;for(const wire of await Promise.all(wires))for(const frame of wire.split(/\r?\n\r?\n/)){const data=frame.split(/\r?\n/).filter(l=>l.startsWith('data:')).map(l=>l.slice(5).trimStart()).join('\n');if(data){EventSchema.parse(JSON.parse(data));count++;}}
console.log(`Automatic goal accounting: actual provider15tokens/elapsed>=1second, native budget10 -> budget_limited, independent subscription refreshed without management mutation; ${count} official-schema events`);
