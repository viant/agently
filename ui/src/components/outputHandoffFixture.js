export const conversationId = 'specimen-report-handoff';
export const renderedContent = {schemaVersion:'1',parts:[],reports:[{id:'overview',scope:'message',grammar:'report-document-v1',status:'committed',sequence:0,
  source:{title:'Delivery overview',blocks:[{id:'summary',kind:'markdownBlock',markdown:'Existing report content remains available beside the conversation.'}]},dataSources:{}}],diagnostics:[]};
export const rows = [{kind:'assistant',renderKey:'answer',messageId:'answer',turnId:'turn',content:'The report is ready for review.',renderedContent}];

export const conversation = {conversationId,turns:[{turnId:'turn',status:'completed',assistant:{final:{messageId:'answer',content:rows[0].content,renderedContent}}}]};
