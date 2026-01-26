export type MessageBuffer = {
  byMessageId: Record<string, string>;
};

export function newMessageBuffer(): MessageBuffer {
  return {byMessageId: {}};
}

export function applyEvent(buffer: MessageBuffer, ev: any): {id: string; text: string; final: boolean} | null {
  if (!buffer || !ev || !ev.message) return null;
  const msgId = String(ev.message.id || ev.message.Id || '').trim();
  if (!msgId) return null;
  const content = ev.content || {};
  if (typeof content.delta === 'string' && content.delta) {
    const next = (buffer.byMessageId[msgId] || '') + content.delta;
    buffer.byMessageId[msgId] = next;
    return {id: msgId, text: next, final: false};
  }
  if (typeof content.text === 'string' && content.text) {
    buffer.byMessageId[msgId] = content.text;
    return {id: msgId, text: content.text, final: true};
  }
  const msgContent = ev.message.content || ev.message.Content;
  if (typeof msgContent === 'string' && msgContent.trim()) {
    buffer.byMessageId[msgId] = msgContent.trim();
    return {id: msgId, text: msgContent.trim(), final: true};
  }
  return null;
}

export function reconcileFromTranscript(buffer: MessageBuffer, transcript: any): void {
  if (!buffer || !transcript) return;
  const turns = transcript.transcript || transcript.Transcript || [];
  for (const turn of turns) {
    const messages = turn?.message || turn?.Message || [];
    for (const m of messages) {
      const role = String(m?.role || m?.Role || '').toLowerCase();
      if (role !== 'assistant') continue;
      const content = m?.content ?? m?.Content;
      if (typeof content === 'string' && content.trim()) {
        const id = String(m?.id || m?.Id || '').trim();
        if (id) buffer.byMessageId[id] = content.trim();
      }
    }
  }
}
