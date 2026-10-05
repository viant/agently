/** Stateless presentation of the official AG-UI message graph. No accumulation,
 * native transcript hydration, metadata authority or host resources. */
export function remoteNamespace(connectionId,threadId,id='') { return `agui-remote:${JSON.stringify([connectionId,threadId,id])}`; }
export function passiveProtocolContent(content) {
  if(typeof content==='string')return content;
  if(Array.isArray(content))return content.map(part=>part?.type==='text' ? String(part.text || '') : JSON.stringify(part,null,2)).join('\n');
  return content === undefined ? '' : JSON.stringify(content,null,2);
}
export function remoteRows(connectionId,threadId,snapshot) {
  let turnId=remoteNamespace(connectionId,threadId,'initial');
  const trust={connectionProfile:'standard',hostEffectsAllowed:false};
  const results=new Map((snapshot?.messages||[]).filter(message=>message.role==='tool').map(message=>[message.toolCallId,message]));
  return (snapshot?.messages || []).flatMap(message=>{
    if(message.role==='system'||message.role==='developer'||message.role==='activity')return [];
    const id=remoteNamespace(connectionId,threadId,message.id);
    if(message.role==='user')turnId=id;
    const base={...trust,renderKey:id,turnId,messageId:id};
    const content=passiveProtocolContent(message.content);
    if(message.role==='user')return [{...base,kind:'user',content}];
    if(message.role==='assistant'||message.role==='reasoning') {
      const rows=content ? [{...base,kind:'assistant',content:message.role==='reasoning'?`Reasoning\n${content}`:content,status:'completed'}] : [];
      if(message.role==='assistant'&&message.toolCalls?.length)rows.push({ ...base,renderKey:`${id}:tools`,kind:'iteration',lifecycle:'completed',rounds:[{...trust,renderKey:`${id}:round`,pageId:`${id}:round`,iteration:0,content:'',toolCalls:message.toolCalls.map(call=>({...trust,renderKey:remoteNamespace(connectionId,threadId,call.id),toolCallId:call.id,toolName:call.function.name,status:results.has(call.id)?(results.get(call.id).error?'failed':'completed'):'requested',content:call.function.arguments})),modelSteps:[]}],linkedConversations:[] });
      return rows;
    }
    // Standard tool output is passive; protocol-only messages stay in history.
    if(message.role==='tool')return [{...base,kind:'assistant',content:`Tool result (${message.toolCallId})\n${content}`}];
    return [];
  });
}
