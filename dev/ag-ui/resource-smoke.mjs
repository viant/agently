// Actual upstream HttpAgent -> assembled native resource commands.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {randomUUID} from 'node:crypto';
const require=createRequire(new URL('../../../agently-core-ag-ui/sdk/ts/package.json',import.meta.url));
const {HttpAgent}=require('@ag-ui/client');
const {EventSchema}=require('@ag-ui/core/schemas');
const url=process.env.AGENTLY_AGUI_URL??'http://127.0.0.1:18192/v1/ag-ui/run';
const expiry=process.argv.includes('--expiry');
for(let attempt=0;attempt<30;attempt++){try{if((await fetch(new URL('/healthz',url))).ok)break}catch{}if(attempt===29)throw new Error('Assembly backend did not become ready');await new Promise(r=>setTimeout(r,500));}
let cookie='';let wireCount=0;const requests=[];
const wires=[];
const transport=async(input,init)=>{if(init?.body)requests.push(JSON.parse(init.body));const response=await fetch(input,{...init,headers:{...init.headers,...(cookie?{cookie}:{})}});const set=response.headers.get('set-cookie');if(set)cookie=set.split(';')[0];if(!response.ok)throw new Error(`HTTP ${response.status}: ${await response.text()}`);wires.push(response.clone().text());return response;};
const threadId=randomUUID();
async function command(operation,payload={},hooks={}){const agent=new HttpAgent({url,fetch:transport,threadId});const events=[];let failure;await agent.runAgent({runId:randomUUID(),forwardedProps:{agently:{version:'1',requestId:randomUUID(),operation,payload}}},{onEvent:({event})=>{events.push(event);hooks.onEvent?.(event)},onRunErrorEvent:({event})=>{failure=event},...hooks.subscriber});return{agent,events,failure,result:events.find(e=>e.type==='CUSTOM'&&e.name===`agently.${operation.split('.')[0]}.result`)?.value};}
const initial=await command('state.get');assert.equal(initial.failure,undefined);assert.equal(initial.result.state,null);
const patch=[{op:'add',path:'',value:{'a/b':{'~k':1},arr:[1,2]}},{op:'test',path:'/a~1b/~0k',value:1},{op:'add',path:'/arr/-',value:3},{op:'copy',from:'/a~1b',path:'/copy'},{op:'move',from:'/copy',path:'/moved'},{op:'replace',path:'/moved/~0k',value:42},{op:'remove',path:'/arr/0'}];
const changed=await command('state.patch',{ifMatch:initial.result.hash,patch});assert.equal(changed.failure,undefined);const expected={'a/b':{'~k':1},arr:[2,3],moved:{'~k':42}};assert.deepEqual(changed.agent.state,expected);assert.deepEqual(changed.result.state,expected);
const stale=await command('state.patch',{ifMatch:initial.result.hash,patch:[{op:'replace',path:'',value:false}]});assert.ok(stale.failure,'stale precondition must fail');assert.deepEqual((await command('state.get')).result.state,expected);
console.log('state: all six RFC6902 operations reduced correctly; stale hash rejected without write');
// Goal mutations require an owned native conversation, established by normal chat.
const chat=new HttpAgent({url,fetch:transport,threadId,initialMessages:[{id:randomUUID(),role:'user',content:'initialize goal conversation'}]});await chat.runAgent({forwardedProps:{agently:{version:'1',operation:'chat',payload:{agentId:'simple',model:'local_mock'}}}},{onRunErrorEvent:({event})=>{throw new Error(event.message)}});
const transcript=async()=>{const response=await fetch(new URL(`/v1/conversations/${threadId}/transcript`,url),{headers:{cookie}});assert.equal(response.status,200);return response.json();};
const beforeGraph=await transcript();const beforeGoalState=(await command('state.get')).result.state;
const spec={continueMode:'idle_only',onTurnFinished:'wait',onAsyncCompleted:'wait'};
const created=await command('goal.create',{objective:'verify live resource commands',tokenBudget:1000,controllerSpec:spec});assert.equal(created.failure,undefined);
const first=created.result.result.goal;assert.ok(first.id);assert.equal(first.conversationId,threadId);assert.equal(first.objective,'verify live resource commands');assert.equal(first.status,'active');assert.equal(first.tokenBudget,1000);assert.deepEqual(JSON.parse(first.controllerSpec),spec);assert.equal(first.tokensUsed,0);assert.equal(first.timeUsedSeconds,0);assert.equal(first.controllerSchedule,null);assert.equal(first.statusReason,'');assert.equal(first.pauseReason,'');
const updated=await command('goal.update',{objective:'updated HTTP objective',tokenBudget:2000});assert.equal(updated.failure,undefined);assert.deepEqual(updated.result.result.goal,{...first,objective:'updated HTTP objective',tokenBudget:2000});
const accepted=requests.at(-1);const acceptedWire=await wires.at(-1);
const replay=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify(accepted)});assert.equal(replay.status,200);assert.equal(await replay.text(),acceptedWire);
const conflict=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify({...accepted,forwardedProps:{agently:{...accepted.forwardedProps.agently,payload:{objective:'conflicting objective',tokenBudget:999}}}})});assert.equal(conflict.status,409);
assert.deepEqual((await command('goal.get')).result.result.goal,updated.result.result.goal,'replay/conflict must preserve committed goal');
const zero=await command('goal.update',{tokenBudget:0});assert.deepEqual(zero.result.result.goal,{...updated.result.result.goal,tokenBudget:0});
// This versioned update schema disallows null budgets; omission preserves budget.
const invalid=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify({...accepted,runId:randomUUID(),forwardedProps:{agently:{...accepted.forwardedProps.agently,requestId:randomUUID(),payload:{tokenBudget:null}}}})});assert.equal(invalid.status,400);assert.equal((await command('goal.get')).result.result.goal.tokenBudget,0);
await command('goal.update',{tokenBudget:2000});
let firstSnapshot;const ready=new Promise(resolve=>{firstSnapshot=resolve});const subscriptionRun=randomUUID();const observer=new HttpAgent({url,fetch:transport,threadId});const observed=[];
const watching=observer.runAgent({runId:subscriptionRun,forwardedProps:{agently:{version:'1',requestId:randomUUID(),operation:'goal.subscribe',payload:{durationSeconds:expiry?4:30}}}},{onEvent:({event})=>{observed.push(event);if(event.type==='ACTIVITY_SNAPSHOT')firstSnapshot()},onRunErrorEvent:({event})=>{throw new Error(event.message)}});
await Promise.race([ready,new Promise((_,reject)=>setTimeout(()=>reject(new Error('goal subscription did not start')),5000))]);
async function observedGoal(predicate,label){for(let i=0;i<50;i++){if(observed.some(e=>e.type==='ACTIVITY_SNAPSHOT'&&predicate(e.content.goal)))return;await new Promise(r=>setTimeout(r,100));}assert.fail(`subscription missing ${label}`);}
const paused=await command('goal.pause',{reason:'HTTP acceptance pause'});assert.equal(paused.result.result.goal.status,'paused');assert.equal(paused.result.result.goal.pauseReason,'HTTP acceptance pause');await observedGoal(g=>g?.status==='paused'&&g.pauseReason==='HTTP acceptance pause','custom pause');
const resumed=await command('goal.resume');assert.equal(resumed.result.result.goal.status,'active');assert.equal(resumed.result.result.goal.pauseReason,'');assert.equal(resumed.result.result.goal.statusReason,'');await observedGoal(g=>g?.status==='active'&&g.pauseReason==='','resume');
const completed=await command('goal.update',{status:'complete',statusReason:'verified actual assembly'});assert.equal(completed.result.result.goal.status,'complete');assert.equal(completed.result.result.goal.statusReason,'verified actual assembly');assert.equal(completed.result.result.goal.objective,'updated HTTP objective');assert.equal(completed.result.result.goal.tokenBudget,2000);await observedGoal(g=>g?.status==='complete'&&g.statusReason==='verified actual assembly','completion');
assert.equal((await command('goal.clear')).result.result.cleared,true);assert.equal((await command('goal.get')).result.result.goal,null);await observedGoal(g=>g===null,'clear');
if(!expiry){const cancelled=await command('run.cancel',{runId:subscriptionRun});assert.equal(cancelled.failure,undefined);}
await watching;assert.equal(observed.at(-1).outcome.type,expiry?'success':'cancelled');
assert.deepEqual(await transcript(),beforeGraph,'resource mutations and observer must preserve native chat graph');
assert.deepEqual((await command('state.get')).result.state,beforeGoalState,'goal commands must preserve shared state');
console.log(`goal: full snapshot, sparse update/zero/null rejection, custom pause/resume reasons, complete/clear live snapshots, mutation replay/conflict, unchanged chat graph/state; ${expiry?'bounded expiry':'subscription cancellation'}`);
for(const wire of await Promise.all(wires))for(const frame of wire.split(/\r?\n\r?\n/)){const data=frame.split(/\r?\n/).filter(l=>l.startsWith('data:')).map(l=>l.slice(5).trimStart()).join('\n');if(data){EventSchema.parse(JSON.parse(data));wireCount++}}
console.log(`Resource smoke passed; ${wireCount} official-schema wire events`);
