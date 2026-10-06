// Owned loopback MCP server: deterministic rows only, no resource/host capabilities.
import http from 'node:http';
export function feedResult(version=1) {return {content:[{type:'text',text:JSON.stringify({rows:[{id:'one',label:'Synthetic feed row',value:version===2?'Updated fixture value':'Initial fixture value'}],version})}],isError:false};}
const port=Number(process.env.PRESENTATION_MCP_PORT||18255);
const server=http.createServer(async(req,res)=>{
  if(req.method==='GET'){res.writeHead(405);res.end();return;}
  if(req.method==='DELETE'){res.writeHead(200);res.end();return;}
  let body='';for await(const chunk of req)body+=chunk;
  let request;try{request=JSON.parse(body);}catch{res.writeHead(400);res.end();return;}
  if(request.id===undefined){res.writeHead(202);res.end();return;}
  let result;
  switch(request.method){
    case 'initialize':result={protocolVersion:request.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'Synthetic presentation fixture',version:'1'}};break;
    case 'tools/list':result={tools:[{name:'fixture_feed',description:'Return deterministic synthetic feed rows',inputSchema:{type:'object',properties:{version:{type:'integer',enum:[1,2]}}}}]};break;
    case 'tools/call':if(request.params.name!=='fixture_feed'){res.writeHead(400);res.end();return;}result=feedResult(request.params.arguments?.version);break;
    case 'ping':result={};break;
    default:res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify({jsonrpc:'2.0',id:request.id,error:{code:-32601,message:'Unsupported method'}}));return;
  }
  res.writeHead(200,{'content-type':'application/json','Mcp-Session-Id':'presentation-fixture'});res.end(JSON.stringify({jsonrpc:'2.0',id:request.id,result}));
});
if(import.meta.url===`file://${process.argv[1]}`)server.listen(port,'127.0.0.1',()=>console.log(`Presentation fixture http://127.0.0.1:${port}/mcp`));
