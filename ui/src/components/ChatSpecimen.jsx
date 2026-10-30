import React from 'react';
import OutputHandoffSpecimen from './OutputHandoffSpecimen.jsx';
import RequestLifecycleSpecimen from './RequestLifecycleSpecimen.jsx';
import {Typography} from 'forge/components';
import Chat from 'forge/components/Chat.jsx';
import {signal} from '@preact/signals-react';
import Composer from 'forge/components/chat/Composer.jsx';
import StarterTaskBrowser from 'forge/components/chat/StarterTaskBrowser.jsx';
import HomeRecentConversations from './HomeRecentConversations.jsx';
import BubbleMessage from './chat/BubbleMessage.jsx';
import TurnFeedback from './chat/TurnFeedback.jsx';
import {onTranscript, useChatProjection} from '../services/chatStore.js';
import {ConversationViewContext} from '../context/ConversationViewContext.js';

const recentExamples = async () => ({data: [{id: 'example', title: 'Example conversation', createdAt: '2026-10-21T16:42:00Z'}]});

const homeContext = {identity: {windowId: 'specimen-home'}, signals: {collection: signal([]), control: signal({}), selection: signal(null), message: signal('')}, handlers: {chat: {}}, Context: () => null, lookupHandler: name => name === 'chat.renderFeed' ? () => null : name === 'chat.resolveComposerProps' ? () => ({commandCenter: true, starterTasks: [{id:'home-example',title:'Review an example',description:'Prepare an editable request.',resultLabel:'Example output',prompt:'Review this example.'}]}) : undefined};

export default function ChatSpecimen() {
  const [draft, setDraft] = React.useState('');
  const [messages, setMessages] = React.useState([]);
  const feedback = useChatProjection('specimen-feedback').find(row => row.kind === 'iteration')?.feedback;
  React.useEffect(() => {
    onTranscript('specimen-feedback', {conversationId:'specimen-feedback',turns:[{
      turnId:'example-answer',status:'completed',feedbackEditable:true,
      feedback:{rating:0,comment:null,updatedAt:null},assistant:{final:{messageId:'example-answer',content:'Example response'}}},
    ]});
  }, []);
  const tasks = [{id: 'example', categoryId: 'review', title: 'Review an example', description: 'Fill an editable prompt without sending it.', resultLabel: 'Example output', prompt: 'Review this example.'}];
  return <><section id="foundation-chat" aria-label="Chat components" className="forge-document-chat">
    <Typography as="h2" typeRole="section">Chat components</Typography>
    <ConversationViewContext.Provider value={{presentation: 'document'}}>
      <BubbleMessage message={{id: 'example-user', role: 'user', content: 'Keep my question compact.\nPreserve line breaks.'}}/>
      <BubbleMessage message={{id: 'example-answer', role: 'assistant', content: 'The reply uses the shared body role, with no surrounding card.\n\nParagraphs keep a compact, consistent gap.\nDeliberate line breaks are preserved.', interim: 0}}/>
      <TurnFeedback conversationId="specimen-feedback" turnId="example-answer" feedback={feedback} editable/>
      {messages.map((content, index) => <BubbleMessage key={index} message={{id: `local-${index}`, role: 'user', content}}/>)}
    </ConversationViewContext.Provider>
    <Composer appearance="document" draftKey="specimen" draftValue={draft} onDraftChange={setDraft}
      agentOptions={[{value: 'example', label: 'Example agent'}]} agentValue="example"
      modelOptions={[{value: 'example', label: 'Example model'}]} modelValue="example"
      reasoningOptions={[{value: 'medium', label: 'Medium'}]} reasoningValue="medium"
      onSubmit={({content}) => setMessages(previous => [...previous, content])}/>
    <StarterTaskBrowser categories={[{id:'review',title:'Review'}]} tasks={tasks} onSelect={task => setDraft(task.prompt)}/>
    <HomeRecentConversations loadConversations={recentExamples}/>
  </section>
  <section id="foundation-chat-home" aria-label="Native chat Home" style={{height:700,display:'flex',flexDirection:'column'}}><Chat context={homeContext} container={{chat:{appearance:'document',showUpload:true,showMic:true}}}/></section><OutputHandoffSpecimen/><RequestLifecycleSpecimen/></>;
}
