import { resolveMoneyPlaceholder, rehydrate } from '../lookups/tokens.js';

function formatLocalDate(value) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, '0');
  const day = String(value.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/**
 * Resolves date macros in starter prompts at click time so a long-running
 * browser session always inserts the current local date.
 *
 * Supported forms:
 *   ${now}
 *   ${today}
 *   ${7 days ago}
 *   ${7days ago}
 *   ${beginning of next month}
 *   ${end of next month}
 */
function dateToken(name, label, value) {
  return `@{${name}:${value} "${label}: ${value}"}`;
}

export function resolveStarterPromptMacros(prompt, now = new Date(), options = {}) {
  const source = resolveMoneyPlaceholder(String(prompt || ''));
  const current = now instanceof Date ? new Date(now.getTime()) : new Date(now);
  if (Number.isNaN(current.getTime())) return source;
  const dateTokens = options?.dateTokens === true;

  return source.replace(/\$\{\s*(?:(now|today)|(\d+)\s*days?\s+ago|(beginning|begining|start|end)\s+of\s+next\s+month)\s*\}/gi, (_, currentToken, daysToken, monthBoundary) => {
    if (monthBoundary) {
      const isEnd = monthBoundary.toLowerCase() === 'end';
      // Construct from calendar boundaries rather than adding a month to the
      // current day, which can overflow short months (e.g. January 31).
      const resolved = new Date(current.getFullYear(), current.getMonth() + (isEnd ? 2 : 1), isEnd ? 0 : 1);
      const value = formatLocalDate(resolved);
      return dateTokens ? dateToken(isEnd ? 'date_to' : 'date_from', isEnd ? 'To' : 'From', value) : value;
    }
    if (currentToken) {
      const value = formatLocalDate(current);
      return dateTokens && String(currentToken).toLowerCase() === 'now'
        ? dateToken('date_to', 'To', value)
        : value;
    }
    const resolved = new Date(current.getTime());
    resolved.setDate(resolved.getDate() - Number(daysToken));
    const value = formatLocalDate(resolved);
    return dateTokens ? dateToken('date_from', 'From', value) : value;
  });
}

// Forge can prefill the composer directly, bypassing StarterTasks. Resolve
// macros there too, while preserving existing chips and user-edited defaults.
export function resolveComposerPromptMacros(prompt, now = new Date()) {
  return rehydrate(String(prompt || '')).map(segment => segment.kind === 'text'
    ? resolveStarterPromptMacros(segment.value, now, { dateTokens: true })
    : segment.raw).join('');
}
