// Voice control command detection for mic / dictation input.

const defaultPhrases = {
  submit: [
    'submit it now',
    'submit now',
    'send it now',
    'send now',
  ],
  cancel: [
    'cancel it now',
    'cancel now',
    'never mind',
    'nevermind',
    'discard it',
    'discard',
  ],
};

function normalize(text) {
  return String(text || '')
    .toLowerCase()
    .replace(/[\u2019]/g, "'")
    .replace(/[\s\t\n\r]+/g, ' ')
    .trim();
}

function escapeRegExp(s) {
  return String(s).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function buildPhraseRegex(phrase) {
  // Match phrase as a standalone chunk, allowing punctuation around it.
  // Examples matched: "... submit it now", "submit it now.", "(submit it now)".
  const p = escapeRegExp(normalize(phrase));
  return new RegExp(`(^|[\\s,;:.!?()\"'])${p}([\\s,;:.!?()\"']|$)`, 'ig');
}

function stripPhrases(text, phrases) {
  let out = String(text || '');
  for (const phrase of phrases || []) {
    out = out.replace(buildPhraseRegex(phrase), ' ');
  }
  out = out.replace(/[\s\t\n\r]+/g, ' ').trim();
  out = out.replace(/\s+([,;:.!?])/g, '$1');
  out = out.replace(/([,;:.!?])(?:\s*\1)+/g, '$1');
  return out;
}

/**
 * Detects voice control commands embedded in dictation text.
 *
 * Returns:
 *  - action: 'submit' | 'cancel' | ''
 *  - cleanedText: input with the detected phrase(s) removed
 */
export function detectVoiceControl(text, opts = {}) {
  const phrases = opts.phrases || defaultPhrases;
  const normalized = normalize(text);
  if (!normalized) {
    return {action: '', cleanedText: ''};
  }

  const hasAny = (list) => (list || []).some((p) => normalized.includes(normalize(p)));
  const submit = hasAny(phrases.submit);
  const cancel = hasAny(phrases.cancel);

  // If both are present, prefer cancel (safer default).
  const action = cancel ? 'cancel' : submit ? 'submit' : '';
  if (!action) {
    return {action: '', cleanedText: String(text || '')};
  }

  const cleanedText = stripPhrases(text, [...(phrases.submit || []), ...(phrases.cancel || [])]);
  return {action, cleanedText};
}
