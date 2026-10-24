// Follow the actual chat viewport, including lazy content and resized panels.
// Each conversation gets a new controller; scrolling upward releases following.
const readingPositions = new Map();
export function attachHistoryScroll(feed, viewport, environment = globalThis, conversationId = '') {
  let saved = readingPositions.get(conversationId);
  if (!saved && conversationId) {
    try {const value = JSON.parse(environment.sessionStorage?.getItem(`agently.chatReading:${conversationId}`) || 'null');
      if (value && Number.isFinite(value.top) && value.top >= 0 && typeof value.following === 'boolean') saved = value;
    } catch {}
  }
  let restoreTop = saved && !saved.following ? saved.top : null;
  let following = saved?.following ?? true;
  if (saved && !following) viewport.scrollTop = saved.top;
  let frame = null;
  let lastTop = viewport.scrollTop;
  const bottom = () => viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop <= 48;
  const remember = () => {
    viewport.dataset && (viewport.dataset.following = String(following));
    if (conversationId && viewport.clientHeight > 0) {
      const value = {top: restoreTop ?? viewport.scrollTop, following};
      readingPositions.set(conversationId, value);
      try {environment.sessionStorage?.setItem(`agently.chatReading:${conversationId}`, JSON.stringify(value));} catch {}
    }
  };
  remember();
  const scroll = () => {
    frame = null;
    if (viewport.clientHeight <= 0) return;
    if (restoreTop !== null) {
      viewport.scrollTop = Math.min(restoreTop, Math.max(0, viewport.scrollHeight - viewport.clientHeight));
      if (viewport.scrollHeight - viewport.clientHeight >= restoreTop) restoreTop = null;
    } else if (following) viewport.scrollTop = viewport.scrollHeight;
    else return;
    lastTop = viewport.scrollTop;
    remember();
  };
  const schedule = () => {
    if ((following || restoreTop !== null) && frame === null) frame = environment.requestAnimationFrame(scroll);
  };
  const onScroll = () => {
    if (bottom()) following = true;
    else if (viewport.scrollTop < lastTop) following = false;
    lastTop = viewport.scrollTop;
    remember();
  };
  const onWheel = event => { if (event.deltaY < 0) {restoreTop = null; following = false; remember();} };
  const onKey = event => {
    if (event.target?.closest?.('input, textarea, [contenteditable="true"]')) return;
    if (['ArrowUp', 'PageUp', 'Home'].includes(event.key)) {restoreTop = null; following = false; remember();}
  };
  let touchY = null;
  const onTouchStart = event => { touchY = event.touches?.[0]?.clientY ?? null; };
  const onTouchMove = event => {
    const nextY = event.touches?.[0]?.clientY;
    if (touchY !== null && nextY > touchY) {restoreTop = null; following = false; remember();}
    touchY = nextY ?? null;
  };
  const submitted = () => {restoreTop = null; following = true; remember(); schedule();};
  viewport.closest?.('[data-testid="chat-root"]')?.addEventListener('forge:chat-submitted', submitted);
  viewport.addEventListener('scroll', onScroll, { passive: true });
  viewport.addEventListener('wheel', onWheel, { passive: true });
  viewport.addEventListener('keydown', onKey);
  viewport.addEventListener('touchstart', onTouchStart, { passive: true });
  viewport.addEventListener('touchmove', onTouchMove, { passive: true });
  const observer = environment.ResizeObserver ? new environment.ResizeObserver(schedule) : null;
  observer?.observe(feed);
  observer?.observe(viewport);
  schedule();
  return () => {
    remember();
    viewport.closest?.('[data-testid="chat-root"]')?.removeEventListener('forge:chat-submitted', submitted);
    observer?.disconnect();
    if (frame !== null) environment.cancelAnimationFrame(frame);
    viewport.removeEventListener('scroll', onScroll);
    viewport.removeEventListener('wheel', onWheel);
    viewport.removeEventListener('keydown', onKey);
    viewport.removeEventListener('touchstart', onTouchStart);
    viewport.removeEventListener('touchmove', onTouchMove);
  };
}
