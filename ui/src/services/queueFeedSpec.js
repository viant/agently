/** Upgrade only the stock legacy Queue editor; preserve custom layout/actions. */
export function normalizeQueueEditorBinding(feedId, ui) {
  if (feedId !== 'queue' || !ui || typeof ui !== 'object') return ui;
  const hasStockSave = node => !!node && typeof node === 'object'
    && (node.handler === 'chat.saveQueuedTurnForm' || Object.values(node).some(hasStockSave));
  if (!hasStockSave(ui)) return ui;
  const visit = node => {
    if (!node || typeof node !== 'object') return node;
    const next = {...node};
    for (const key of ['containers','items']) {
      if (Array.isArray(node[key])) next[key] = node[key].map(visit);
    }
    const form = node.schemaBasedForm;
    const properties = form?.schema?.properties;
    if (form?.id === 'queueEditForm' && form.dataSourceRef === 'queueTurns'
      && properties?.preview && !Object.prototype.hasOwnProperty.call(properties,'content')) {
      const {preview,...rest} = properties;
      next.schemaBasedForm = {...form,schema:{...form.schema,properties:{...rest,content:preview}}};
    }
    return next;
  };
  return visit(ui);
}
