// Follow the actual chat viewport, including lazy content and resized panels.
// Each conversation gets a new controller; scrolling upward releases following.
export function attachHistoryScroll(feed, viewport, environment = globalThis) {
  let following = true;
  let frame = null;
  let lastTop = viewport.scrollTop;
  const bottom = () => viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop <= 48;
  const scroll = () => {
    frame = null;
    if (!following) return;
    viewport.scrollTop = viewport.scrollHeight;
    lastTop = viewport.scrollTop;
  };
  const schedule = () => {
    if (following && frame === null) frame = environment.requestAnimationFrame(scroll);
  };
  const onScroll = () => {
    if (bottom()) following = true;
    else if (viewport.scrollTop < lastTop) following = false;
    lastTop = viewport.scrollTop;
  };
  const onWheel = event => { if (event.deltaY < 0) following = false; };
  const onKey = event => {
    if (event.target?.closest?.('input, textarea, [contenteditable="true"]')) return;
    if (['ArrowUp', 'PageUp', 'Home'].includes(event.key)) following = false;
  };
  let touchY = null;
  const onTouchStart = event => { touchY = event.touches?.[0]?.clientY ?? null; };
  const onTouchMove = event => {
    const nextY = event.touches?.[0]?.clientY;
    if (touchY !== null && nextY > touchY) following = false;
    touchY = nextY ?? null;
  };
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
    observer?.disconnect();
    if (frame !== null) environment.cancelAnimationFrame(frame);
    viewport.removeEventListener('scroll', onScroll);
    viewport.removeEventListener('wheel', onWheel);
    viewport.removeEventListener('keydown', onKey);
    viewport.removeEventListener('touchstart', onTouchStart);
    viewport.removeEventListener('touchmove', onTouchMove);
  };
}
