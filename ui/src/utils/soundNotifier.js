// soundNotifier.js – simple WebAudio-based finish notification with de-dup

const notified = new Set();

function playTone({ freq = 880, durationMs = 140, volume = 0.2 } = {}) {
  try {
    const ctx = new (window.AudioContext || window.webkitAudioContext)();
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = 'sine';
    o.frequency.setValueAtTime(freq, ctx.currentTime);
    g.gain.value = volume;
    o.connect(g);
    g.connect(ctx.destination);
    o.start();
    setTimeout(() => {
      try { o.stop(); ctx.close(); } catch(_) {}
    }, durationMs);
  } catch (_) { /* ignore */ }
}

export function notifyFinishOnce(turnId, opts = {}) {
  const { enabled = true, volume = 0.25, freq = 880 } = opts;
  if (!enabled) return false;
  const id = String(turnId || '');
  if (!id) return false;
  if (notified.has(id)) return false;
  notified.add(id);
  playTone({ freq, volume });
  return true;
}

export function resetNotified() {
  notified.clear();
}

