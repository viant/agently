function object(value) {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value;
  if (typeof value === 'string') { try { return object(JSON.parse(value)); } catch {} }
  return {};
}
// Raw tool arguments are display data, not implicitly authorized editors.
export function approvalDecisionInput(item, action, values = null) {
  const input = { action };
  if (action !== 'approve') return input;
  const metadata = object(item?.metadata);
  const form = object(values);
  const editors = Array.isArray(metadata.approval?.editors) ? metadata.approval.editors : [];
  const fields = {};
  for (const editor of editors) {
    const name = String(editor?.name || '').trim();
    if (name && Object.hasOwn(form, name)) fields[name] = form[name];
  }
  if (Object.keys(fields).length) input.editedFields = fields;
  const schema = object(metadata.review?.requestedSchema);
  if (metadata.review && Object.keys(schema).length) {
    const payload = schema.additionalProperties === true ? { ...form } : {};
    for (const name of Object.keys(object(schema.properties))) if (Object.hasOwn(form, name)) payload[name] = form[name];
    if (Object.keys(payload).length) input.payload = payload;
  }
  return input;
}
