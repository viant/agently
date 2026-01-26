export function isPreamble(ev: any): boolean {
  const meta = ev?.content?.meta;
  const kind = typeof meta?.kind === 'string' ? meta.kind : '';
  return kind.trim().toLowerCase() === 'preamble';
}

export function toolPhase(ev: any): string {
  const meta = ev?.content?.meta;
  const phase = typeof meta?.phase === 'string' ? meta.phase : '';
  return phase.trim();
}

export function toolName(ev: any): string {
  const name = typeof ev?.content?.name === 'string' ? ev.content.name : '';
  return name.trim();
}

export function toolCallId(ev: any): string {
  const id = typeof ev?.content?.toolCallId === 'string' ? ev.content.toolCallId : '';
  return id.trim();
}
