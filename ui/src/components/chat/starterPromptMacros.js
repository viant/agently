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
 */
function dateToken(name, label, value) {
  return `@{${name}:${value} "${label}: ${value}"}`;
}

export function resolveStarterPromptMacros(prompt, now = new Date(), options = {}) {
  const source = String(prompt || '');
  const current = now instanceof Date ? new Date(now.getTime()) : new Date(now);
  if (Number.isNaN(current.getTime())) return source;
  const dateTokens = options?.dateTokens === true;

  return source.replace(/\$\{\s*(?:(now|today)|(\d+)\s*days?\s+ago)\s*\}/gi, (_, currentToken, daysToken) => {
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
