import { registerDynamicEvaluator } from 'forge/runtime/binding';
import { getLogger } from 'forge/utils/logger';

const log = getLogger('agently');

// Disable specific toolbar save buttons in UI only
const DISABLED_TOOLBAR_IDS = new Set(['saveAgent', 'saveModel', 'saveServer']);

export const guardrails = {
  onInit() {
    try {
      if (guardrails._readonlyHookInstalled) return;
      registerDynamicEvaluator('onReadonly', ({ item }) => {
        try {
          if (item && DISABLED_TOOLBAR_IDS.has(String(item.id))) {
            return true; // force disabled
          }
        } catch (_) { /* ignore */ }
        return undefined; // let other handlers decide
      });
      guardrails._readonlyHookInstalled = true;
    } catch (e) {
      log.warn('guardrails.onInit hook error', e);
    }
  },
};

