export const INCOMPLETE_FORGE_WINDOW_CONTRACT_MESSAGE = 'This workspace definition is incomplete. Refresh the backend and retry.';

function resolveForgeWindowContractData(input = null) {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    return {};
  }
  if (input.data && typeof input.data === 'object' && !Array.isArray(input.data)) {
    return input.data;
  }
  return input;
}

export function getIncompleteWindowContractError(input = null) {
  const data = resolveForgeWindowContractData(input);
  const actionCode = String(data?.actions?.code || '').trim();
  if (actionCode) {
    return '';
  }
  const hooks = data?.reportBuilder?.hooks && typeof data.reportBuilder.hooks === 'object'
    ? Object.values(data.reportBuilder.hooks).map((value) => String(value || '').trim()).filter(Boolean)
    : [];
  if (hooks.length === 0) {
    return '';
  }
  return INCOMPLETE_FORGE_WINDOW_CONTRACT_MESSAGE;
}
