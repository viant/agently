import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Dialog, Spinner, Switch } from '@blueprintjs/core';
import { addWindow, activeWindows, getWindowContext, removeWindow, selectedTabId, selectedWindowId } from 'forge/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client, getAuthMeSilently } from '../services/agentlyClient';
import { MAIN_CHAT_WINDOW_ID, resolveConversationSelection } from '../services/conversationWindow';
import { getWorkspaceMetadataSnapshot, resolveWorkspaceAppName, subscribeWorkspaceMetadata } from '../services/workspaceMetadata';
import { extractPlannerElicitationMeta, prepareRenderableRequestedSchema } from './elicitationHelpers';
import logo from '../viant-logo.png';

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

export function openWindow(windowKey, windowTitle, refreshDataSources = [], options = {}) {
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  const replaceTabbedWindows = options?.replaceTabbedWindows === true;
  const replaceMainChatTree = options?.replaceMainChatTree === true;
  const desiredConversationId = String(options?.conversationId || '').trim() || null;
  const desiredParentKey = options?.parentKey ?? null;
  const desiredPresentation = String(options?.presentation || '').trim() || null;
  const desiredRegion = String(options?.region || '').trim() || null;
  let existing = windows.find((entry) => entry?.windowKey === windowKey);
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
  existing = currentWindows.find((entry) => entry?.windowKey === windowKey);
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
    existing = addWindow(windowTitle, desiredParentKey, windowKey, null, true, {}, {
      autoIndexTitle: false,
      conversationId: desiredConversationId || undefined,
      presentation: desiredPresentation || undefined,
      region: desiredRegion || undefined,
    });
  }
  if (existing?.windowId) {
    selectedTabId.value = existing.windowId;
    selectedWindowId.value = existing.windowId;
    refreshWindowDataSources(existing.windowId, refreshDataSources);
  }
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

export function normalizeQueueApprovalDialog(item = null) {
  const selected = item && typeof item === 'object' ? item : {};
  const metadata = normalizeQueueJSON(selected?.metadata);
  const review = metadata?.review && typeof metadata.review === 'object' ? metadata.review : {};
  const approval = metadata?.approval && typeof metadata.approval === 'object' ? metadata.approval : {};
  const argumentsPayload = normalizeQueueJSON(selected?.arguments);
  const requestedSchema = review?.requestedSchema && typeof review.requestedSchema === 'object'
    ? review.requestedSchema
    : null;
  const preparedSchema = prepareRenderableRequestedSchema(requestedSchema, argumentsPayload);
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

const plannerHeaderStyle = {
  textAlign: 'left',
  fontSize: 12,
  letterSpacing: 0.3,
  textTransform: 'uppercase',
  color: 'var(--dark-gray3)',
  padding: '8px 10px',
  borderBottom: '1px solid #d8e1ee',
  whiteSpace: 'nowrap',
};

const plannerCellStyle = {
  padding: '10px',
  borderBottom: '1px solid #eef2f7',
  verticalAlign: 'top',
  fontSize: 13,
};

export default function MenuBar({
  approvals,
  onToggleSidebar,
  showExecutionDetails = true,
  onToggleExecutionDetails,
  showIntakeDetails = false,
  onToggleIntakeDetails,
  showWorkspaceWindow = true,
  onToggleWorkspaceWindow,
  showToolFeeds = true,
  onToggleToolFeeds
}) {
  const {
    items = [],
    pendingCount = 0,
    page: approvalsPage = 0,
    pageCount: approvalsPageCount = 1,
    start: approvalsStart = 0,
    end: approvalsEnd = 0,
    hasPrevious: approvalsHasPrevious = false,
    hasNext: approvalsHasNext = false,
    open,
    selected,
    setOpen,
    setPage,
    setSelected,
    decide
  } = approvals || {};
  const [user, setUser] = useState(null);
  const [appName, setAppName] = useState(() => resolveWorkspaceAppName(getWorkspaceMetadataSnapshot(), 'Agently'));
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [approvalPage, setApprovalPage] = useState(0);
  const [approvalSubmitting, setApprovalSubmitting] = useState(false);
  const [approvalError, setApprovalError] = useState('');
  const [queuePlannerRows, setQueuePlannerRows] = useState([]);
  const queueFormValuesRef = useRef({});

  const selectedQueueDialog = useMemo(() => normalizeQueueApprovalDialog(selected), [selected]);
  const selectedQueueSchema = selectedQueueDialog.preparedFormSchema;
  const selectedQueueArguments = selectedQueueDialog.argumentsPayload;
  const selectedQueueApproval = selectedQueueDialog.approval;
  const selectedQueuePlannerMeta = selectedQueueDialog.plannerMeta;
  const canRenderQueueSchema = !!selectedQueueSchema && typeof selectedQueueSchema === 'object';

  useEffect(() => {
    let mounted = true;
    const loadUser = async () => {
      try {
        const me = await getAuthMeSilently();
        if (mounted) setUser(me || null);
        if (me) return;
        const providers = await client.getAuthProviders();
        const action = resolveStartupAuthAction(providers);
        if (action.type !== 'local' || !action.username) return;
        await client.localLogin({ username: action.username });
        const autoMe = await getAuthMeSilently();
        if (!mounted || !autoMe) return;
        setUser(autoMe);
        try { window.dispatchEvent(new CustomEvent('forge:conversation-active', { detail: { id: '' } })); } catch (_) {}
        try { window.location.reload(); } catch (_) {}
      } catch (_) {}
    };
    const onAuthorized = () => { void loadUser(); };
    const unsubscribeWorkspaceMetadata = subscribeWorkspaceMetadata((payload) => {
      if (!mounted) return;
      setAppName(resolveWorkspaceAppName(payload, 'Agently'));
    });
    void loadUser();
    if (typeof window !== 'undefined') {
      window.addEventListener('agently:authorized', onAuthorized);
    }
    return () => {
      mounted = false;
      unsubscribeWorkspaceMetadata();
      if (typeof window !== 'undefined') {
        window.removeEventListener('agently:authorized', onAuthorized);
      }
    };
  }, []);

  const displayName = user?.displayName || user?.username || user?.email || user?.subject || '';
  const currentConversationId = resolveConversationSelection(MAIN_CHAT_WINDOW_ID);

  const approvalPageData = useMemo(() => {
    if (typeof setPage === 'function') {
      return {
        items,
        page: approvalsPage,
        pageCount: approvalsPageCount,
        start: Math.max(0, approvalsStart > 0 ? approvalsStart - 1 : 0),
        end: Math.max(0, approvalsEnd),
        total: pendingCount,
        hasPrevious: approvalsHasPrevious,
        hasNext: approvalsHasNext
      };
    }
    return paginateApprovalItems(items, approvalPage, APPROVALS_PAGE_SIZE);
  }, [items, approvalPage, approvalsPage, approvalsPageCount, approvalsStart, approvalsEnd, pendingCount, approvalsHasPrevious, approvalsHasNext, setPage]);

  useEffect(() => {
    setApprovalPage((current) => {
      const maxPage = Math.max(0, approvalPageData.pageCount - 1);
      return current > maxPage ? maxPage : current;
    });
  }, [approvalPageData.pageCount]);

  useEffect(() => {
    if (!open) {
      setApprovalPage(0);
      setPage?.(0);
    }
  }, [open, setPage]);

  useEffect(() => {
    if (selected?.id) {
      setOpen?.(false);
    }
  }, [selected?.id, setOpen]);

  useEffect(() => {
    setApprovalSubmitting(false);
    setApprovalError('');
    const initialValues = selectedQueueArguments && typeof selectedQueueArguments === 'object'
      ? JSON.parse(JSON.stringify(selectedQueueArguments))
      : {};
    if (selectedQueuePlannerMeta?.field) {
      const seededDefaultRows = Array.isArray(selectedQueuePlannerMeta.defaultRows) ? selectedQueuePlannerMeta.defaultRows : [];
      const initialRows = seededDefaultRows.length > 0
        ? JSON.parse(JSON.stringify(seededDefaultRows))
        : (Array.isArray(initialValues[selectedQueuePlannerMeta.field]) ? initialValues[selectedQueuePlannerMeta.field] : []);
      initialValues[selectedQueuePlannerMeta.field] = initialRows;
      setQueuePlannerRows(initialRows);
    } else {
      setQueuePlannerRows([]);
    }
    queueFormValuesRef.current = initialValues;
  }, [selected?.id]);

  const handleLogout = useCallback(async () => {
    setUserMenuOpen(false);
    try {
      await client.logout();
    } catch (_) {}
    try { window.localStorage?.removeItem('agently.userName'); } catch (_) {}
    window.location.reload();
  }, []);

  const handleApprovalDecision = useCallback(async (item, action) => {
    if (!item || !decide) return;
    const payload = action === 'approve'
      ? ((queueFormValuesRef.current && Object.keys(queueFormValuesRef.current).length > 0)
          ? queueFormValuesRef.current
          : (selectedQueueArguments && typeof selectedQueueArguments === 'object' ? selectedQueueArguments : null))
      : null;
    setApprovalSubmitting(true);
    setApprovalError('');
    try {
      await decide(item, action, payload);
    } catch (err) {
      setApprovalError(String(err?.message || err || 'Failed to decide approval'));
    } finally {
      setApprovalSubmitting(false);
    }
  }, [decide, selectedQueueArguments]);

  const toggleQueuePlannerRow = useCallback((rowIndex) => {
    if (!selectedQueuePlannerMeta?.field) return;
    const field = selectedQueuePlannerMeta.field;
    const selectionField = selectedQueuePlannerMeta.selectionField;
    const currentRows = Array.isArray(queueFormValuesRef.current?.[field]) ? queueFormValuesRef.current[field] : [];
    const nextRows = currentRows.map((row, index) => (
      index === rowIndex
        ? { ...row, [selectionField]: !row?.[selectionField] }
        : row
    ));
    setQueuePlannerRows(nextRows);
    queueFormValuesRef.current = {
      ...(queueFormValuesRef.current || {}),
      [field]: nextRows,
    };
    setSelected?.((current) => current ? { ...current } : current);
  }, [selectedQueuePlannerMeta, setSelected]);

  return (
    <header className="app-topbar">
      <div className="app-topbar-left">
        <div className="app-brand">
          <img src={logo} alt="Viant" className="app-brand-logo" />
          <span className="app-brand-name">{appName}</span>
        </div>
        <div className="app-topbar-divider" />
        <div className="app-topbar-actions">
          <Button minimal icon="menu" data-testid="sidebar-toggle" onClick={onToggleSidebar} />
          <Button
            minimal
            icon="time"
            text="Automation"
            className="app-topbar-nav-btn"
            data-testid="automation-nav"
            onClick={() => openWindow('schedule', 'Automation', ['schedules'], {
              replaceTabbedWindows: true,
              replaceMainChatTree: true,
            })}
          />
          <Button
            minimal
            icon="history"
            text="Runs"
            className="app-topbar-nav-btn"
            data-testid="runs-nav"
            onClick={() => openWindow('schedule/history', 'Runs', ['runs'], {
              replaceTabbedWindows: true,
              replaceMainChatTree: true,
            })}
          />
          <Button
            minimal
            icon="notifications"
            intent={pendingCount > 0 ? 'warning' : 'none'}
            className={pendingCount > 0 ? 'app-approval-bell is-pending' : 'app-approval-bell'}
            data-testid="approval-bell"
            onClick={() => setOpen?.(!open)}
          />
          <div className="app-topbar-settings-wrap">
            <Button
              minimal
              icon="eye-open"
              text="View settings"
              className="app-topbar-nav-btn app-topbar-settings-btn"
              data-testid="view-settings-btn"
              onClick={() => {
                setSettingsOpen((value) => !value);
                setUserMenuOpen(false);
              }}
            />
            {settingsOpen ? (
              <div className="app-topbar-settings-menu" data-testid="view-settings-menu">
                <div className="app-topbar-settings-title">View settings</div>
                <Switch
                  checked={!!showExecutionDetails}
                  label="Show execution details"
                  onChange={() => onToggleExecutionDetails?.()}
                />
                <Switch
                  checked={!!showIntakeDetails}
                  label="Show intake details"
                  onChange={() => onToggleIntakeDetails?.()}
                />
                <Switch
                  checked={!!showToolFeeds}
                  label="Show tool feeds"
                  onChange={() => onToggleToolFeeds?.()}
                />
                <Switch
                  checked={!!showWorkspaceWindow}
                  label="Show workspace view"
                  onChange={() => onToggleWorkspaceWindow?.()}
                />
              </div>
            ) : null}
          </div>
        </div>
      </div>
      <div className="app-topbar-right" style={{ position: 'relative' }}>
        <Button
          minimal
          icon="user"
          className="app-user-btn"
          data-testid="user-menu-btn"
          onClick={() => {
            setUserMenuOpen((v) => !v);
            setSettingsOpen(false);
          }}
        >
          {displayName}
        </Button>
        {userMenuOpen ? (
          <div className="app-user-menu" data-testid="user-menu">
            {user?.email ? <div className="app-user-menu-email">{user.email}</div> : null}
            <Button
              minimal
              icon="log-out"
              text="Logout"
              className="app-user-menu-logout"
              data-testid="logout-btn"
              onClick={handleLogout}
            />
          </div>
        ) : null}
      </div>

      {open && !selected ? (
        <div className="app-approval-popover app-approval-popover-left">
          {items.length === 0 ? (
            <div className="app-approval-empty">No pending approvals</div>
          ) : (
            <>
              <div className="app-approval-list">
                {approvalPageData.items.map((item) => (
                  <button
                    key={item.id}
                    className="app-approval-row"
                    onClick={() => {
                      setSelected?.(item);
                      setOpen?.(false);
                    }}
                  >
                    <div className="app-approval-title">{item.title || item.toolName || item.id}</div>
                    <div className="app-approval-subtitle">{item.status}</div>
                  </button>
                ))}
              </div>
              {approvalPageData.total > APPROVALS_PAGE_SIZE ? (
                <div className="app-approval-pagination">
                  <Button
                    small
                    minimal
                    disabled={!approvalPageData.hasPrevious}
                    onClick={() => {
                      if (typeof setPage === 'function') {
                        setPage(Math.max(0, approvalPageData.page - 1));
                        return;
                      }
                      setApprovalPage((current) => Math.max(0, current - 1));
                    }}
                  >
                    Previous
                  </Button>
                  <div className="app-approval-pagination-status">
                    {approvalPageData.total > 0 ? `${approvalPageData.start + 1}-${approvalPageData.end} of ${approvalPageData.total}` : '0 of 0'}
                  </div>
                  <Button
                    small
                    minimal
                    disabled={!approvalPageData.hasNext}
                    onClick={() => {
                      if (typeof setPage === 'function') {
                        setPage(approvalPageData.page + 1);
                        return;
                      }
                      setApprovalPage((current) => current + 1);
                    }}
                  >
                    Next
                  </Button>
                </div>
              ) : null}
            </>
          )}
        </div>
      ) : null}

      <Dialog
        isOpen={!!selected}
        onClose={() => {
          if (approvalSubmitting) return;
          setSelected?.(null);
        }}
        title={selected?.title || selected?.toolName || 'Approval detail'}
      >
        <div className="app-approval-dialog">
          <div><strong>Tool:</strong> {selected?.toolName}</div>
          {canRenderQueueSchema ? (
            <div style={{ marginTop: 12 }}>
              {selectedQueuePlannerMeta ? (
                <div style={{
                  display: 'grid',
                  gap: 14,
                  border: '1px solid #d8e1ee',
                  borderRadius: 16,
                  background: '#ffffff',
                  padding: '16px 18px',
                  marginBottom: 12,
                }}>
                  <div style={{ fontSize: 16, fontWeight: 700 }}>{selectedQueuePlannerMeta.title}</div>
                  <div style={{ overflowX: 'auto' }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                      <thead>
                        <tr>
                          <th style={plannerHeaderStyle}>Review</th>
                          {selectedQueuePlannerMeta.columns.map((column) => (
                            <th key={column.key} style={plannerHeaderStyle}>{column.label}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {queuePlannerRows.map((row, rowIndex) => (
                          <tr key={String(row?.id || row?.site_id || rowIndex)}>
                            <td style={plannerCellStyle}>
                              <button
                                type="button"
                                onClick={() => toggleQueuePlannerRow(rowIndex)}
                                disabled={approvalSubmitting}
                                style={{
                                  display: 'inline-flex',
                                  alignItems: 'center',
                                  gap: 8,
                                  fontWeight: 600,
                                  border: 'none',
                                  background: 'transparent',
                                  padding: 0,
                                  cursor: approvalSubmitting ? 'default' : 'pointer',
                                }}
                              >
                                <input
                                  type="checkbox"
                                  checked={Boolean(row?.[selectedQueuePlannerMeta.selectionField])}
                                  onClick={(event) => event.stopPropagation()}
                                  onChange={() => toggleQueuePlannerRow(rowIndex)}
                                  disabled={approvalSubmitting}
                                />
                                <span>{row?.[selectedQueuePlannerMeta.selectionField] ? 'Keep' : 'Drop'}</span>
                              </button>
                            </td>
                            {selectedQueuePlannerMeta.columns.map((column) => (
                              <td key={`${column.key}-${rowIndex}`} style={plannerCellStyle}>
                                {String(row?.[column.key] ?? '')}
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              ) : null}
              <SchemaBasedForm
                showSubmit={false}
                schema={selectedQueueSchema}
                data={{}}
                context={getWindowContext?.(selectedWindowId.value)}
                onChange={(payload) => {
                  const values = payload?.values || payload?.data || payload || {};
                  queueFormValuesRef.current = {
                    ...(queueFormValuesRef.current || {}),
                    ...(values && typeof values === 'object' ? values : {}),
                  };
                }}
                disabled={approvalSubmitting}
              />
            </div>
          ) : (
            <pre>{JSON.stringify(selected?.arguments || {}, null, 2)}</pre>
          )}
          {approvalError ? <p style={{ color: '#ef4444', marginTop: 8 }}>{approvalError}</p> : null}
          <div className="app-approval-dialog-actions">
            {approvalSubmitting ? <Spinner size={16} /> : null}
            <Button intent="danger" disabled={approvalSubmitting} onClick={() => handleApprovalDecision(selected, 'reject')}>Reject</Button>
            <Button
              intent="primary"
              disabled={approvalSubmitting}
              onClick={() => handleApprovalDecision(selected, 'approve')}
            >
              {selectedQueueApproval?.acceptLabel || 'Approve'}
            </Button>
          </div>
        </div>
      </Dialog>
    </header>
  );
}
