function normalizeNameList(value) {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item || '').trim()).filter(Boolean);
}

export async function normalizeSelection({ editedFields = {}, originalArgs = {} } = {}) {
  const requested = normalizeNameList(originalArgs.names);
  const selected = new Set(
    normalizeNameList(editedFields.names).length > 0
      ? normalizeNameList(editedFields.names)
      : requested
  );
  return {
    editedFields: {
      names: requested.filter((name) => selected.has(name))
    }
  };
}

export async function filterEnvNames(args = {}) {
  return normalizeSelection(args);
}

export const approvalService = {
  normalizeSelection,
  filterEnvNames
};
