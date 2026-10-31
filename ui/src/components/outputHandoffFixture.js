export const conversationId = 'specimen-report-handoff';
export const renderedContent = {schemaVersion:'1',parts:[],reports:[{id:'overview',scope:'message',grammar:'report-document-v1',status:'committed',sequence:0,
  source:{title:'Delivery overview',blocks:[{id:'summary',kind:'markdownBlock',markdown:'Existing report content remains available beside the conversation.'}]},dataSources:{}}],diagnostics:[]};
export const rows = [
  {kind:'assistant',renderKey:'findings',messageId:'findings',turnId:'turn',content:'The selected record is ready for review.\n\nCheck the supporting details before deciding on a change.'},
  {kind:'assistant',renderKey:'answer',messageId:'answer',turnId:'turn',content:'',renderedContent},
];

export const conversation = {conversationId,turns:[{turnId:'turn',status:'completed',messages:[{messageId:'findings',role:'assistant',content:rows[0].content,interim:0,mode:'task'}],assistant:{final:{messageId:'answer',content:'',renderedContent}}}]};

const pendingContent={...renderedContent,reports:renderedContent.reports.map(report=>({...report,status:'rendering',source:{title:report.source.title,blocks:[]}}))};
export const pendingRows=[rows[0],{kind:'iteration',renderKey:'answer',turnId:'turn',lifecycle:'running',rounds:[],
  assistantFinal:{messageId:'answer',content:'[Interactive content]',renderedContent:pendingContent}}];
export const pendingConversation={conversationId,turns:[{turnId:'turn',status:'running',createdAt:new Date().toISOString(),
  messages:conversation.turns[0].messages,assistant:{final:{messageId:'answer',content:'[Interactive content]',renderedContent:pendingContent}}}]};
