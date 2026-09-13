// Skill activation is a leading prompt command, not a mid-sentence mention.
export function skillHintQuery(text, caret = String(text || '').length) {
  const prefix = String(text || '').slice(0, caret);
  return /^\$[a-zA-Z0-9_./-]*$/.test(prefix) ? prefix.slice(1) : null;
}
export function matchingSkills(items = [], query = '') {
  const q = String(query).toLowerCase();
  return items.filter(item => /^[a-zA-Z0-9][a-zA-Z0-9_./-]*$/.test(item?.name || ''))
    .filter(item => `${item.name} ${item.description || ''}`.toLowerCase().includes(q))
    .sort((a,b) => Number(b.name.toLowerCase().startsWith(q))-Number(a.name.toLowerCase().startsWith(q)) || a.name.localeCompare(b.name));
}
export function insertSkillPrefix(value, name) {
  const text = String(value || '');
  const match = text.match(/^\$[^\s]*[ \t]*/);
  if (!match) return null;
  const prefix = `$${name} `;
  return {value: prefix + text.slice(match[0].length), prefix, end: match[0].length, caret: prefix.length};
}
