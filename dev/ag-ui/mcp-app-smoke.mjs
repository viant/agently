// Real upstream HttpAgent -> assembled backend -> configured loopback MCP server.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {randomUUID} from 'node:crypto';
const require=createRequire(new URL('../../../agently-core-ag-ui/examples/ag-ui-shell/package.json',import.meta.url));
const {HttpAgent}=require('@ag-ui/client');const {EventSchema}=require('@ag-ui/core/schemas');
const url=process.env.AGENTLY_AGUI_URL??'http://127.0.0.1:18241/v1/ag-ui/run';
let cookie='';const requests=[];const wires=[];
const transport=async(input,init)=>{requests.push(JSON.parse(init.body));const response=await fetch(input,{...init,headers:{...init.headers,...(cookie?{cookie}:{})}});if(response.headers.get('set-cookie'))cookie=response.headers.get('set-cookie').split(';')[0];if(!response.ok)throw Error(`HTTP ${response.status}: ${await response.text()}`);wires.push(response.clone().text());return response;};
async function run(threadId,forwardedProps,messages=[],resume){const runId=forwardedProps.__proxiedMCPRequest&&!resume?threadId:randomUUID();const agent=new HttpAgent({url,threadId,fetch:transport,initialMessages:messages});const events=[];const result=await agent.runAgent({runId,forwardedProps,...(resume?{resume}:{})},{onEvent:({event})=>{EventSchema.parse(event);events.push(event)},onRunErrorEvent:()=>{}});return{events,result,runId,agent};}
const activities=[];
for(let i=0;i<2;i++){const threadId=randomUUID();const initial=await run(threadId,{agently:{version:'1',operation:'chat',payload:{agentId:process.env.MCP_APP_AGENT??'mcp_app_fixture',model:'local_mock'}}},[{id:randomUUID(),role:'user',content:`fixture-mcp-app ${i}`}]);const activity=initial.events.find(e=>e.type==='ACTIVITY_SNAPSHOT'&&e.activityType==='mcp-apps');assert.ok(activity,'backend must issue an MCP Apps activity; events='+JSON.stringify(initial.events));assert.ok(activity.content.serverId);activities.push({threadId,activity});}
assert.notEqual(activities[0].activity.content.serverId,activities[1].activity.content.serverId,'two app bindings need unique opaque public aliases');assert.equal(activities[0].activity.content.serverHash,activities[1].activity.content.serverHash);
for(const {activity} of activities){const {serverHash,serverId,resourceUri}=activity.content;for(const method of ['resources/read','tools/call']){const instance=randomUUID();const isolated=randomUUID();const reply=await run(isolated,{__proxiedMCPRequest:{serverHash,serverId,method,params:method==='resources/read'?{uri:resourceUri}:{name:'fixture_view',arguments:{instance}}}});const result=reply.result.result;assert.ok(result,'RUN_FINISHED result required');if(method==='tools/call'){assert.equal(result._meta['fixture/private'].instance,instance);assert.deepEqual(result.structuredContent,{instance,nested:{values:[1,false,null]}});assert.equal(result.content[1].type,'image');assert.equal(result.content[2].type,'resource_link');assert.equal(result.isError,false);}else{assert.equal(result._meta['fixture/read'],'host-only');assert.deepEqual(result.contents[0]._meta['fixture/resource'],{preserve:true});}console.log(JSON.stringify({scope:'assembled-backend',serverId,method,result}));const wire=await wires.at(-1);const replay=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify(requests.at(-1))});assert.equal(replay.status,200);assert.equal(await replay.text(),wire);}}
console.log('Actual assembled MCP Apps resource/tool/replay and two unique binding tests passed. Approval/restart/browser require separate checks.');

if(process.argv.includes('--approval')) {
 const {readFile}=await import('node:fs/promises');
 const calls=async instance=>{assert.ok(process.env.MCP_APP_LOG,'MCP_APP_LOG required for approval effect evidence');let log='';try{log=await readFile(process.env.MCP_APP_LOG,'utf8')}catch(e){if(e.code!=='ENOENT')throw e;}return log.split('\n').filter(Boolean).map(JSON.parse).filter(r=>r.method==='tools/call'&&r.params?.name==='fixture_approved'&&r.params.arguments?.instance===instance).length;};
 for(const action of ['approve','reject']) {
  const {serverId,serverHash}=activities[0].activity.content;const instance=randomUUID();const isolated=randomUUID();const props={__proxiedMCPRequest:{serverId,serverHash,method:'tools/call',params:{name:'fixture_approved',arguments:{instance}}}};
  const pending=await run(isolated,props);const outcome=pending.events.at(-1).outcome;assert.ok(outcome,'approval pending requires interrupted outcome: '+JSON.stringify(pending.events));
  assert.equal(outcome.type,'interrupt');assert.equal(outcome.interrupts.length,1);assert.equal(outcome.interrupts[0].reason,'approval');assert.equal(await calls(instance),0,'tool must not execute before decision');
  const resumed=await run(isolated,props,[],[{interruptId:outcome.interrupts[0].id,status:'resolved',payload:{action}}]);if(action==='approve'){assert.ok(resumed.events.at(-1).outcome,'approval resume requires success: '+JSON.stringify(resumed.events));assert.equal(resumed.events.at(-1).outcome.type,'success');}else{assert.equal(resumed.events.at(-1).type,'RUN_ERROR');assert.equal(resumed.events.at(-1).code,'MCP_PROXY_REJECTED');}
  assert.equal(await calls(instance),action==='approve'?1:0);
  if(action==='approve'){assert.equal(resumed.result.result._meta['fixture/private'].instance,instance);assert.equal(resumed.result.result.structuredContent.instance,instance);}else assert.equal(resumed.result.result,undefined,'rejected call must not fabricate remote host result');
  const replay=await fetch(url,{method:'POST',headers:{'content-type':'application/json',cookie},body:JSON.stringify(requests.at(-1))});assert.equal(replay.status,200);assert.equal(await replay.text(),await wires.at(-1));assert.equal(await calls(instance),action==='approve'?1:0);
  console.log(`Assembled MCP approval ${action}: exact side effect count and replay verified`);
 }
}
