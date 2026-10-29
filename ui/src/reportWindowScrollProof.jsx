import React from 'react';
import {createRoot} from 'react-dom/client';
import {SettingProvider, addWindow} from 'forge/core';
import {registerContainerRenderer} from 'forge/runtime/containerRendererRegistry.js';
import AppSectionContent from './components/AppSectionContent.jsx';
import './index.css';
import './components/sectionNavigation.css';

const query=new URLSearchParams(location.search);
const placement = query.get('placement') || 'top';
const kind = query.get('kind') || 'report';
const pin = {providerIdentity:'example',uri:'report://example/summary',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
const blocks = Array.from({length:12}, (_,i) => ({id:`section-${i}`,kind:'markdownBlock',title:i===11?'Last report section':`Section ${i+1}`,markdown:Array(8).fill('Synthetic report content for scroll verification.').join('\n\n')}));
const services = {reportStore:{
  getReport:async()=>({resource:pin,resourceDefinition:{builderDefinition:{reportBuilder:{filterPresentation:placement,predicates:[{id:'query',label:'Query',kind:'text'}]}}}}),
  runReport:async()=>({resource:pin,reportSpec:{title:'Example report',datasets:[],blocks,layoutIntent:{blockOrder:blocks.map(block=>block.id),items:blocks.map(block=>({blockId:block.id}))}},reportFill:{datasets:[],diagnostics:[],blocks:blocks.map(block=>({...block,content:{title:block.title,markdown:block.markdown}}))}}),
}};
registerContainerRenderer('scroll-proof',()=> <div>{blocks.map(block=><section key={block.id}><h2>{block.title}</h2>{Array.from({length:8},(_,i)=><p key={i} style={{paddingBlock:12}}>Synthetic window content for scroll verification.</p>)}</section>)}</div>);
const binding={actor:{issuer:'example',subject:'fixture'},instanceId:'fixture',parametersDigest:'empty',token:'fixture'};
const key=kind==='report'?pin.uri:kind==='window-mcp'?'window://example/records':'records';
const metadata={selectionBinding:binding,resource:{uri:'window://example/records',revision:'1'},dataSource:{},view:{content:{id:'long-content',kind:'scroll-proof',sizingMode:'content'}}};
const entry=addWindow('Example report',null,key,'',true,{}, {region:'app.section',...(kind!=='report'?{selectionBinding:binding,inlineMetadata:metadata}:{})});
createRoot(document.getElementById('root')).render(<SettingProvider services={services} connectorConfig={{window:{service:{uri:'/unused-fixture'}}}}>
  <div style={{height:'100dvh',display:'flex',flexDirection:'column',overflow:'hidden'}}>
    <header style={{flex:'0 0 54px'}}>Report scroll fixture</header>
    <main className="app-chat-pane"><AppSectionContent window={entry} title="Example report"/></main>
    <footer style={{flex:'0 0 24px'}}>Ready</footer>
  </div>
</SettingProvider>);
