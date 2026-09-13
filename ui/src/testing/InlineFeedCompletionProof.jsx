import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import ChatFeedFromChatStore from '../components/chat/ChatFeedFromChatStore.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
import {applyFeedEvent,updateFeedData,feedTracker,makeFeedKey} from '../services/toolFeedBus.js';
import 'forge/packs/blueprint/index.jsx';
const conversationId='inline-completion-proof';
const turnId='original-turn';
const payload = text => ({
  presentation:{target:'inline'},
  data:{output:{text}},
  dataSources:{output:{source:'output'}},
  ui:{renderMode:'forge',dataSources:{output:{source:'output'}},containers:[{id:'output-panel',dataSourceRef:'output',items:[{id:'feed-text',type:'label',dataBind:'text',dataField:'text'}]}]},
});
function activate(feedId,text){updateFeedData(feedId,payload(text),conversationId);feedTracker.setActive({feedId:makeFeedKey(feedId,conversationId),rawFeedId:feedId,conversationId,turnId,presentation:{target:'inline'}});}
activate('original-feed','Streaming tool output');
function Proof(){
 const [rows,setRows]=useState([{kind:'assistant',renderKey:'streaming-row',turnId,messageId:'streaming-message',content:'Working'}]);
 return <ConversationViewContext.Provider value={{toolFeedDock:'inline',developerMode:false}}>
  <button id="complete" onClick={()=>{updateFeedData('original-feed',payload('Final tool output'),conversationId);setRows([{kind:'assistant',renderKey:'final-row',turnId,messageId:'final-message',content:'Done'}]);}}>Complete original turn</button>
  <button id="replace" onClick={()=>{applyFeedEvent({type:'tool_feed_inactive',feedId:'original-feed',conversationId});activate('replacement-feed','Different feed');}}>Different identity</button>
  <ChatFeedFromChatStore conversationId={conversationId} rowsOverride={rows}/>
 </ConversationViewContext.Provider>;
}
createRoot(document.getElementById('root')).render(<Proof/>);
