import { addWindow, activeWindows, getWindowContext, removeWindow, selectedTabId, selectedWindowId } from 'forge/core';
import { MAIN_CHAT_WINDOW_ID } from '../services/conversationWindow';
import { extractPlannerElicitationMeta, prepareRenderableRequestedSchema } from './elicitationHelpers';
import { selectPath } from '../services/feedForgeWiring';
import logo from '../viant-logo.png';

const remoteOrInlineImageRefPattern = /^(https?:\/\/|data:image\/|blob:|\/)/i;

export function resolveBrandLogoSrc(iconRef = '') {
  const raw = String(iconRef || '').trim();
  const normalized = raw.toLowerCase();
  if (!normalized || normalized === 'builtin:viant') {
    return logo;
  }
  if (remoteOrInlineImageRefPattern.test(raw)) {
    return raw;
  }
  return '';
}

export function resolveBrandMonogram(appName = '') {
  const words = String(appName || '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2);
  if (words.length === 0) {
    return 'A';
  }
  const letters = words.map((word) => {
    const match = word.match(/[A-Za-z0-9]/);
    return match ? match[0].toUpperCase() : '';
  }).join('');
  return letters || 'A';
}

export function resolveStartupAuthAction(providers) {
  const normalized = Array.isArray(providers) ? providers : [];
  const hasOAuthProvider = normalized.some((entry) => {
    const type = String(entry?.type || '').trim().toLowerCase();
    return type === 'bff' || type === 'oidc' || type === 'jwt';
  });
  if (hasOAuthProvider) {
    return { type: 'oauth' };
  }
  const localProvider = normalized.find((entry) => String(entry?.type || '').trim().toLowerCase() === 'local' && String(entry?.defaultUsername || '').trim());
  if (!localProvider) {
    return { type: 'none' };
  }
  return { type: 'local', username: String(localProvider.defaultUsername || '').trim() };
}

export function refreshWindowDataSources(windowId, dataSourceRefs = []) {
  if (!windowId || !Array.isArray(dataSourceRefs) || dataSourceRefs.length === 0) return;
  const base = getWindowContext?.(windowId);
  if (!base?.Context) return;
  dataSourceRefs.forEach((dataSourceRef) => {
    if (!dataSourceRef) return;
    try {
      base.Context(dataSourceRef)?.handlers?.dataSource?.fetchCollection?.();
    } catch (_) {}
  });
}

export function canonicalWindowParameters(value = {}) {
  const normalize = (entry) => {
    if (Array.isArray(entry)) return entry.map(normalize);
    if (entry && typeof entry === 'object') {
      return Object.fromEntries(Object.keys(entry).sort().map((key) => [key, normalize(entry[key])]));
    }
    return entry;
  };
  return JSON.stringify(normalize(value || {}));
}

export function openWindow(windowKey, windowTitle, refreshDataSources = [], options = {}) {
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  const replaceTabbedWindows = options?.replaceTabbedWindows === true;
  const replaceMainChatTree = options?.replaceMainChatTree === true;
  const desiredConversationId = String(options?.conversationId || '').trim() || null;
  const desiredParentKey = options?.parentKey ?? null;
  const desiredPresentation = String(options?.presentation || '').trim() || null;
  const desiredRegion = String(options?.region || '').trim() || null;
  const desiredProviderIdentity = String(options?.providerIdentity || '').trim() || null;
  const desiredResourceIdentity = canonicalWindowParameters(options?.resourceRef || {});
  const desiredParameters = options?.parameters && typeof options.parameters === 'object' && !Array.isArray(options.parameters) ? options.parameters : {};
  const desiredParameterIdentity = canonicalWindowParameters(desiredParameters);
  const matchesDestination = (entry) => entry?.windowKey === windowKey
    && (String(entry?.conversationId || '').trim() || null) === desiredConversationId
    && (entry?.parentKey ?? null) === desiredParentKey
    && (String(entry?.presentation || '').trim() || null) === desiredPresentation
    && (String(entry?.region || '').trim() || null) === desiredRegion
    && (String(entry?.providerIdentity || '').trim() || null) === desiredProviderIdentity
    && canonicalWindowParameters(entry?.resourceRef || {}) === desiredResourceIdentity
    && canonicalWindowParameters(entry?.parameters || {}) === desiredParameterIdentity;
  let existing = windows.find(matchesDestination);
  if (replaceMainChatTree) {
    const subtreeIds = windows
      .filter((entry) => {
        const windowId = String(entry?.windowId || '').trim();
        const parentKey = String(entry?.parentKey || '').trim();
        return windowId === MAIN_CHAT_WINDOW_ID || parentKey === MAIN_CHAT_WINDOW_ID;
      })
      .map((entry) => String(entry?.windowId || '').trim())
      .filter(Boolean);
    subtreeIds.forEach((windowId) => removeWindow(windowId));
  }
  if (replaceTabbedWindows) {
    const keepWindowId = String(existing?.windowId || '').trim();
    const currentWindows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
    activeWindows.value = currentWindows.filter((entry) => {
      if (entry?.inTab === false) return true;
      const windowId = String(entry?.windowId || '').trim();
      if (keepWindowId && windowId === keepWindowId) return true;
      if (desiredRegion) {
        const entryRegion = String(entry?.region || '').trim() || null;
        return entryRegion !== desiredRegion;
      }
      return false;
    });
  }
  const currentWindows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  existing = currentWindows.find(matchesDestination);
  if (existing) {
    const currentParentKey = existing?.parentKey ?? null;
    const currentConversationId = String(existing?.conversationId || '').trim() || null;
    const currentPresentation = String(existing?.presentation || '').trim() || null;
    const currentRegion = String(existing?.region || '').trim() || null;
    if (currentParentKey !== desiredParentKey || currentConversationId !== desiredConversationId || currentPresentation !== desiredPresentation || currentRegion !== desiredRegion) {
      activeWindows.value = currentWindows.map((entry) => {
        if (entry?.windowId !== existing.windowId) return entry;
        return {
          ...entry,
          conversationId: desiredConversationId || undefined,
          parentKey: desiredParentKey,
          presentation: desiredPresentation || undefined,
          region: desiredRegion || undefined,
        };
      });
      existing = activeWindows.peek().find((entry) => entry?.windowId === existing.windowId) || existing;
    }
  }
  if (!existing) {
    const uniqueWindowId = `layout-${String(windowKey).replace(/[^a-zA-Z0-9_-]/g, '-')}-${globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(36).slice(2)}`}`;
    existing = addWindow(windowTitle, desiredParentKey, windowKey, null, true, desiredParameters, {
      autoIndexTitle: false,
      windowId: uniqueWindowId,
      conversationId: desiredConversationId || undefined,
      presentation: desiredPresentation || undefined,
      region: desiredRegion || undefined,
      providerIdentity: desiredProviderIdentity || undefined,
      resourceRef: options.resourceRef ? { ...options.resourceRef } : undefined,
    });
  }
  if (existing?.windowId) {
    if (desiredPresentation === 'hosted') activeWindows.value = activeWindows.peek().map(entry => entry.windowId === existing.windowId ? {...entry, workspaceCollapsed: false} : entry);
    selectedTabId.value = existing.windowId;
    selectedWindowId.value = existing.windowId;
    refreshWindowDataSources(existing.windowId, refreshDataSources);
  }
  return existing;
}

export const APPROVALS_PAGE_SIZE = 8;

export function paginateApprovalItems(items = [], page = 0, pageSize = APPROVALS_PAGE_SIZE) {
  const list = Array.isArray(items) ? items : [];
  const normalizedPageSize = Number.isFinite(Number(pageSize)) && Number(pageSize) > 0
    ? Math.max(1, Math.floor(Number(pageSize)))
    : APPROVALS_PAGE_SIZE;
  const pageCount = Math.max(1, Math.ceil(list.length / normalizedPageSize));
  const safePage = Math.min(Math.max(0, Math.floor(Number(page) || 0)), pageCount - 1);
  const start = safePage * normalizedPageSize;
  const end = Math.min(list.length, start + normalizedPageSize);
  return {
    items: list.slice(start, end),
    page: safePage,
    pageCount,
    start,
    end,
    total: list.length,
    hasPrevious: safePage > 0,
    hasNext: safePage < pageCount - 1
  };
}

function normalizeQueueJSON(value = null) {
  if (!value) return {};
  if (typeof value === 'object') return value;
  if (typeof value !== 'string') return {};
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (_) {
    return {};
  }
}

function setNestedPath(root, path, value) {
  if (!root || typeof root !== 'object') return;
  const normalized = String(path || '').trim().replace(/\[(\d+)\]/g, '.$1').replace(/^\./, '');
  const parts = normalized.split('.').filter(Boolean);
  if (parts.length === 0) return;
  let cursor = root;
  for (let index = 0; index < parts.length - 1; index += 1) {
    const token = parts[index];
    const nextToken = parts[index + 1];
    if (!cursor[token] || typeof cursor[token] !== 'object') {
      cursor[token] = /^\d+$/.test(nextToken) ? [] : {};
    }
    cursor = cursor[token];
  }
  cursor[parts[parts.length - 1]] = value;
}

function applyQueueReviewSeeds(requestedSchema = null, seeds = [], source = null) {
  try {
    if (!requestedSchema || typeof requestedSchema !== 'object') return requestedSchema;
    if (!Array.isArray(seeds) || seeds.length === 0) return requestedSchema;
    const clone = JSON.parse(JSON.stringify(requestedSchema));
    seeds.forEach((seed) => {
      const schemaPath = String(seed?.schemaPath || '').trim();
      const selector = String(seed?.selector || '').trim();
      if (!schemaPath || !selector) return;
      const value = selectPath(selector, source);
      if (value === null || value === undefined) return;
      setNestedPath(clone, schemaPath, JSON.parse(JSON.stringify(value)));
    });
    return clone;
  } catch (_) {
    return requestedSchema;
  }
}

export function normalizeQueueApprovalDialog(item = null) {
  const selected = item && typeof item === 'object' ? item : {};
  const metadata = normalizeQueueJSON(selected?.metadata);
  const review = metadata?.review && typeof metadata.review === 'object' ? metadata.review : {};
  const approval = metadata?.approval && typeof metadata.approval === 'object' ? metadata.approval : {};
  const argumentsPayload = normalizeQueueJSON(selected?.arguments);
  const requestedSchema = review?.requestedSchema && typeof review.requestedSchema === 'object'
    ? review.requestedSchema
    : null;
  const seededSchema = applyQueueReviewSeeds(requestedSchema, review?.seeds, argumentsPayload);
  const preparedSchema = prepareRenderableRequestedSchema(seededSchema, argumentsPayload);
  const plannerMeta = extractPlannerElicitationMeta(preparedSchema);
  const preparedFormSchema = (() => {
    if (!plannerMeta?.field || !preparedSchema?.properties || typeof preparedSchema.properties !== 'object') {
      return preparedSchema;
    }
    const clone = JSON.parse(JSON.stringify(preparedSchema));
    delete clone.properties[plannerMeta.field];
    if (Array.isArray(clone.required)) {
      clone.required = clone.required.filter((key) => key !== plannerMeta.field);
    }
    return clone;
  })();
  return {
    approval,
    review,
    argumentsPayload,
    requestedSchema,
    plannerMeta,
    preparedSchema,
    preparedFormSchema,
  };
}

export function extractSchemaDefaultValues(schema = null) {
  if (!schema || typeof schema !== 'object') return {};
  const properties = schema.properties && typeof schema.properties === 'object' ? schema.properties : {};
  const values = {};
  Object.entries(properties).forEach(([key, field]) => {
    if (!field || typeof field !== 'object') return;
    if (field.default === undefined) return;
    values[key] = JSON.parse(JSON.stringify(field.default));
  });
  return values;
}

export function findApprovalItemById(items = [], approvalId = '') {
  const wanted = String(approvalId || '').trim();
  if (!wanted) return null;
  return (Array.isArray(items) ? items : []).find((item) => String(item?.id || '').trim() === wanted) || null;
}

export function captureReturnFocusTarget(doc = typeof document !== 'undefined' ? document : null) {
  const activeElement = doc && typeof doc === 'object' ? doc.activeElement : null;
  if (activeElement && typeof activeElement.focus === 'function') {
    return activeElement;
  }
  return null;
}

export function resolveReturnFocusTarget(existingTarget = null, doc = typeof document !== 'undefined' ? document : null) {
  if (existingTarget && typeof existingTarget.focus === 'function') {
    return existingTarget;
  }
  return captureReturnFocusTarget(doc);
}

export function restoreReturnFocusTarget(target = null) {
  if (!target || typeof target.focus !== 'function') return false;
  try {
    target.focus();
    return true;
  } catch (_) {
    return false;
  }
}

export function nextApprovalDialogFocusReturn({ wasOpen = false, isOpen = false, target = null } = {}) {
  if (isOpen) {
    return { wasOpen: true, restoreTarget: null };
  }
  if (!wasOpen) {
    return { wasOpen: false, restoreTarget: null };
  }
  return { wasOpen: false, restoreTarget: target ?? null };
}
