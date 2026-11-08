const asArray = value => Array.isArray(value) ? value : [];

export function readStarterTaskPreferences(metadata) {
  const preferences = metadata?.starterTaskPreferences;
  return {
    hidden: asArray(preferences?.hidden),
    order: asArray(preferences?.order),
    own: asArray(preferences?.own),
  };
}

export function starterTaskPreferenceEntries(metadata, preferences = readStarterTaskPreferences(metadata), {includeHidden = false} = {}) {
  const namespace = asArray(metadata?.namespaceStarterTasks?.tasks);
  const hidden = new Set(preferences.hidden);
  const ordered = new Map(preferences.order.map((id, index) => [id, index]));
  const entries = [
    ...namespace.map(task => ({...task, hidden: hidden.has(task.id)})),
    ...preferences.own.map(task => ({...task, id: `personal#${task.id}`, personal: true})),
  ];
  return entries
    .filter(task => includeHidden || !task.hidden)
    .sort((left, right) => (ordered.get(left.id) ?? Infinity) - (ordered.get(right.id) ?? Infinity) || 0);
}

export function namespaceFromStarterTaskID(id) {
  return /^startertask:\/\/([^/]+)\//.exec(id)?.[1] || '';
}
