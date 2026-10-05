// Native host RPC turns remain in the authoritative transcript for recovery.
// Their results belong to the app host, not the conversational message surface.
export function projectChatConversation(conversation) {
  if (!Array.isArray(conversation?.turns)) return conversation;
  const turns = conversation.turns.filter(turn => turn?.origin !== 'host_request');
  return turns.length === conversation.turns.length ? conversation : { ...conversation, turns };
}
