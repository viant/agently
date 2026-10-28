import RequestReviewDialog, {ReviewAction,reviewDecisionError} from './RequestReviewDialog.jsx';
import {ReviewTable} from 'forge/components';
import { openLayoutURL } from './layoutNavigation';
import { resolveBrandLogoSrc, resolveBrandMonogram, resolveStartupAuthAction, openWindow, APPROVALS_PAGE_SIZE, paginateApprovalItems, normalizeQueueApprovalDialog, extractSchemaDefaultValues, findApprovalItemById, resolveReturnFocusTarget, restoreReturnFocusTarget, nextApprovalDialogFocusReturn } from './menuBarHelpers';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {createPortal} from 'react-dom';
import {Icon as SharedIcon, iconNames, Typography, SelectControl, typographyProps} from 'forge/components';
import {ShellIconButton, ShellLocation} from './ShellHeader.jsx';
import { Button, Dialog, Spinner, Tooltip } from '@blueprintjs/core';
import { activeWindows, getWindowContext, selectedTabId, selectedWindowId } from 'forge/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client, getAuthMeSilently } from '../services/agentlyClient';
import { subscribeMCPUIApprovalRequests } from '../services/mcpApps/approvalEvents.js';
import { MAIN_CHAT_WINDOW_ID, resolveConversationSelection } from '../services/conversationWindow';
import { uiSettingsHref } from '../services/uiSettingsNavigation';
import { conversationUsageHref, shouldShowConversationUsage } from '../services/conversationUsage';
import { getWorkspaceMetadataSnapshot, resolveWorkspaceBranding, subscribeWorkspaceMetadata } from '../services/workspaceMetadata';

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
  conversationId = '',
  topbarActions = [],
  backendConnections = [],
  activeBackendId = 'agently',
  onBackendChange,
  onNativeAction,
  brandMount = null,
  profileMount = null,
  sidebarToggleRef,
  inert = false,
  sectionNavigation = false,
  heading = 'New Chat',
  history,
  viewerHeaderRef,
  viewerVisible = false,
  viewerExpanded = false,
  sidebarCollapsed = false,
  compactNavigation = false,
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
  const [branding, setBranding] = useState(() => resolveWorkspaceBranding(getWorkspaceMetadataSnapshot(), {
    fallbackName: 'Agently',
    fallbackIconRef: 'builtin:viant',
  }));
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const userMenuRef = useRef(null);
  const [approvalPage, setApprovalPage] = useState(0);
  const [approvalSubmitting, setApprovalSubmitting] = useState(false);
  const [approvalError, setApprovalError] = useState('');
  const [pendingExternalApprovalId, setPendingExternalApprovalId] = useState('');
  const [queuePlannerRows, setQueuePlannerRows] = useState([]);
  const queueFormValuesRef = useRef({});
  const returnFocusRef = useRef(null);
  const wasApprovalDialogOpenRef = useRef(false);

  useEffect(() => {
    if (!userMenuOpen || typeof document === 'undefined') return undefined;
    const closeOutside = (event) => {
      if (!userMenuRef.current?.contains(event.target)) setUserMenuOpen(false);
    };
    const closeOnEscape = (event) => {
      if (event.key !== 'Escape') return;
      setUserMenuOpen(false);
      try { userMenuRef.current?.querySelector('[data-testid="user-menu-btn"]')?.focus(); } catch (_) {}
    };
    document.addEventListener('pointerdown', closeOutside);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('pointerdown', closeOutside);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [userMenuOpen]);

  const selectedQueueDialog = useMemo(() => normalizeQueueApprovalDialog(selected), [selected]);
  const selectedQueueSchema = selectedQueueDialog.preparedFormSchema;
  const selectedQueueArguments = selectedQueueDialog.argumentsPayload;
  const selectedQueueApproval = selectedQueueDialog.approval;
  const selectedQueuePlannerMeta = selectedQueueDialog.plannerMeta;
  const selectedQueueDisplayData = useMemo(
    () => extractSchemaDefaultValues(selectedQueueSchema),
    [selectedQueueSchema],
  );
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
      setBranding(resolveWorkspaceBranding(payload, {
        fallbackName: 'Agently',
        fallbackIconRef: 'builtin:viant',
      }));
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
  const appName = String(branding?.appName || 'Agently').trim() || 'Agently';
  const appIconSrc = resolveBrandLogoSrc(branding?.appIconRef);
  const appIconMonogram = resolveBrandMonogram(appName);

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
    return subscribeMCPUIApprovalRequests((request) => {
      const approvalId = String(request?.approvalId || '').trim();
      if (!approvalId) return;
      returnFocusRef.current = resolveReturnFocusTarget(returnFocusRef.current);
      const matched = findApprovalItemById(items, approvalId);
      if (matched) {
        setSelected?.(matched);
        setOpen?.(false);
        setPendingExternalApprovalId('');
        return;
      }
      setPendingExternalApprovalId(approvalId);
      setPage?.(0);
      setApprovalPage(0);
      setOpen?.(true);
    });
  }, [items, setOpen, setPage, setSelected]);

  useEffect(() => {
    const matched = findApprovalItemById(items, pendingExternalApprovalId);
    if (!matched) return;
    setSelected?.(matched);
    setOpen?.(false);
    setPendingExternalApprovalId('');
  }, [items, pendingExternalApprovalId, setOpen, setSelected]);

  useEffect(() => {
    const transition = nextApprovalDialogFocusReturn({
      wasOpen: wasApprovalDialogOpenRef.current,
      isOpen: !!selected?.id,
      target: returnFocusRef.current,
    });
    wasApprovalDialogOpenRef.current = transition.wasOpen;
    if (transition.restoreTarget) {
      returnFocusRef.current = null;
      restoreReturnFocusTarget(transition.restoreTarget);
    }
  }, [selected?.id]);

  useEffect(() => {
    setApprovalSubmitting(false);
    setApprovalError('');
    const initialValues = selectedQueueArguments && typeof selectedQueueArguments === 'object'
      ? { ...JSON.parse(JSON.stringify(selectedQueueArguments)), ...extractSchemaDefaultValues(selectedQueueSchema) }
      : extractSchemaDefaultValues(selectedQueueSchema);
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
      ? queueFormValuesRef.current
      : null;
    setApprovalSubmitting(true);
    setApprovalError('');
    try {
      await decide(item, action, payload);
    } catch (err) {
      setApprovalError(sectionNavigation ? reviewDecisionError(err) : String(err?.message || err || 'Failed to decide approval'));
    } finally {
      setApprovalSubmitting(false);
    }
  }, [decide, selectedQueueArguments, selectedQueuePlannerMeta, sectionNavigation]);

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

  const ActionButton = sectionNavigation ? ShellIconButton : Button;
  const initials = displayName.split(/\s+/).filter(Boolean).slice(0, 2).map(part => part[0]).join('').toUpperCase();
  const compactViewerUtilities = sectionNavigation && compactNavigation && viewerVisible;
  const renderUtilityActions = (inMenu = false) => (
        <div className={`app-topbar-actions${inMenu ? ' is-menu' : ''}`} onClick={event => {if (inMenu && event.target.closest('button')) setUserMenuOpen(false);}}>
          {!sectionNavigation ? <Tooltip content="Toggle conversations" placement="bottom">
            <Button ref={sidebarToggleRef} minimal icon="menu" aria-label="Toggle conversations" className="app-topbar-icon-btn is-conversations" data-testid="sidebar-toggle" onClick={onToggleSidebar} />
          </Tooltip> : null}
          {topbarActions.map((entry) => (
            <Tooltip key={entry.id} content={entry.title} placement="bottom">
              <Button minimal icon={iconNames.includes(entry.icon) ? <SharedIcon name={entry.icon}/> : entry.icon || 'application'} aria-label={entry.title} disabled={entry.disabled}
                className={`app-topbar-icon-btn${entry.className ? ` ${entry.className}` : ''}`} data-testid={`topbar-${entry.id}`}
                {...(sectionNavigation ? typographyProps('label') : {})} role={inMenu ? 'menuitem' : undefined} text={inMenu ? entry.title : undefined}
                onClick={() => {
                  const action = entry.action;
                  if (openLayoutURL(action)) return;
                  if (action?.type !== 'window') return;
                  onNativeAction?.();
                  const key = action.resourceUri || (action.provider && action.provider !== 'workspace'
                    ? `provider:${action.provider}:${action.windowKey}` : action.windowKey);
                  openWindow(key, entry.title, action.refreshDataSources || [], {
                    parameters: action.parameters || {},
                    ...(action.providerIdentity ? {providerIdentity: action.providerIdentity} : {}),
                    ...(action.providerIdentity && action.resourceUri ? {resourceRef: {uri: action.resourceUri}} : {}),
                  });
                }} />
            </Tooltip>
          ))}
          {backendConnections.length > 1 ? (
            sectionNavigation ? <SelectControl aria-label="Agent backend" value={activeBackendId}
              className="app-backend-select" {...typographyProps('label')} onChange={event => {onBackendChange?.(event.target.value);if(inMenu)setUserMenuOpen(false);}}>
              {backendConnections.map(connection => <option key={connection.id} value={connection.id}>{connection.label}</option>)}
            </SelectControl> : <select aria-label="Agent backend" value={activeBackendId}
              className="app-backend-select" onChange={event => {onBackendChange?.(event.target.value);if(inMenu)setUserMenuOpen(false);}}>
              {backendConnections.map(connection => <option key={connection.id} value={connection.id}>{connection.label}</option>)}
            </select>
          ) : null}
          <Tooltip content={pendingCount > 0 ? `${pendingCount} pending approvals` : 'Approvals'} placement="bottom">
            <ActionButton
              minimal
              icon={sectionNavigation ? "lucide:bell" : "notifications"}
              aria-label={pendingCount > 0 ? `${pendingCount} pending approvals` : 'Approvals'}
              intent={pendingCount > 0 ? 'warning' : 'none'}
              className={sectionNavigation ? `app-shell-approvals${pendingCount > 0 ? ' is-pending' : ''}` : pendingCount > 0 ? 'app-topbar-icon-btn app-approval-bell is-pending' : 'app-topbar-icon-btn app-approval-bell'}
              role={inMenu ? "menuitem" : undefined}
              data-testid="approval-bell"
              onClick={() => {if(inMenu)onNativeAction?.();setOpen?.(!open);}}
            >{inMenu ? pendingCount > 0 ? `${pendingCount} pending approvals` : 'Approvals' : null}</ActionButton>
          </Tooltip>
          {shouldShowConversationUsage(conversationId) ? (
            <Tooltip content="Conversation usage" placement="bottom">
              <ActionButton
                minimal
                icon={sectionNavigation ? "lucide:gauge" : "dashboard"}
                aria-label="Conversation usage"
                className={sectionNavigation ? "app-shell-usage" : "app-topbar-icon-btn is-usage"}
                role={inMenu ? "menuitem" : undefined}
                data-testid="conversation-usage-nav"
                onClick={() => { window.location.href = conversationUsageHref(conversationId); }}
              >{inMenu ? 'Conversation usage' : null}</ActionButton>
            </Tooltip>
          ) : null}
        </div>
  );
  const renderProfile = () => (
    <div className="app-topbar-right" ref={userMenuRef}>
        <Button
          {...(sectionNavigation ? typographyProps('label') : {})}
          minimal
          icon={sectionNavigation ? undefined : profileMount ? <SharedIcon name="lucide:user"/> : 'user'}
          className="app-user-btn"
          aria-label={profileMount ? `Account: ${displayName || "Profile"}` : undefined}
          title={profileMount ? displayName || "Profile" : undefined}
          data-testid="user-menu-btn"
          aria-haspopup="menu"
          aria-expanded={userMenuOpen}
          aria-controls={userMenuOpen ? 'app-user-menu' : undefined}
          onClick={() => {
            setUserMenuOpen((v) => !v);
          }}
        >
          {sectionNavigation ? <Typography as="span" typeRole="caption" className="app-account-initials">{initials || <SharedIcon name="lucide:user"/>}</Typography> : profileMount ? null : displayName}
        </Button>
        {userMenuOpen ? (
          <div className={`app-user-menu${sectionNavigation ? ' is-document' : ''}`} {...(sectionNavigation ? typographyProps('body') : {})} id="app-user-menu" data-testid="user-menu" role="menu">
            {user?.email ? <div className="app-user-menu-email">{user.email}</div> : null}
            {compactViewerUtilities ? renderUtilityActions(true) : null}
            {sectionNavigation && !compactNavigation ? <Button minimal role="menuitem" icon={<SharedIcon name="lucide:menu"/>}
              {...typographyProps('label')} text={sidebarCollapsed ? 'Expand navigation' : 'Collapse navigation'}
              onClick={() => {setUserMenuOpen(false); onToggleSidebar?.();}} className="app-navigation-collapse"/> : null}
            <Button
              {...(sectionNavigation ? typographyProps('label') : {})}
              minimal
              icon={sectionNavigation ? <SharedIcon name="lucide:settings"/> : 'cog'}
              text="UI Settings"
              className="app-user-menu-settings"
              data-testid="ui-settings-link"
              role="menuitem"
              onClick={() => { setUserMenuOpen(false); window.location.href = uiSettingsHref(); }}
            />
            <Button
              minimal
              {...(sectionNavigation ? typographyProps('label') : {})}
              icon={sectionNavigation ? <SharedIcon name="lucide:log-out"/> : "log-out"}
              text="Logout"
              className="app-user-menu-logout"
              data-testid="logout-btn"
              role="menuitem"
              onClick={() => { setUserMenuOpen(false); handleLogout(); }}
            />
          </div>
        ) : null}
      </div>

  );

  return (
    <header className={`app-topbar${sectionNavigation ? ' is-document' : ''}${viewerVisible ? ' has-viewer' : ''}${viewerVisible && viewerExpanded ? ' viewer-expanded' : ''}`} inert={inert ? '' : undefined}>
      <div className="app-topbar-left">
        {brandMount ? createPortal(<div className="app-brand" title={appName}>
          {appIconSrc ? <img src={appIconSrc} alt={`${appName} logo`} className="app-brand-logo"/> : <span className="app-brand-mark">{appIconMonogram}</span>}
          <Typography as="span" typeRole="section" className="app-brand-name">{appName}</Typography>
        </div>, brandMount) : <div className="app-brand">
          {appIconSrc ? (
            <img src={appIconSrc} alt={`${appName} logo`} className="app-brand-logo" />
          ) : (
            <span className="app-brand-mark" aria-hidden="true">{appIconMonogram}</span>
          )}
          <span className="app-brand-name">{appName}</span>
        </div>}
        {!brandMount ? <div className="app-topbar-divider" /> : null}
        {sectionNavigation ? <ShellLocation title={heading} history={history} onToggleNavigation={onToggleSidebar} navigationRef={sidebarToggleRef}/> : null}
        {!compactViewerUtilities ? renderUtilityActions() : null}
      </div>
      {sectionNavigation ? <div className="app-shell-viewer-header" ref={viewerHeaderRef}/> : null}
      {profileMount ? createPortal(renderProfile(), profileMount) : renderProfile()}

      {open && !selected ? (
        <div className={sectionNavigation ? "app-request-approval-list" : "app-approval-popover app-approval-popover-left"}>
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
                    <div {...(sectionNavigation ? typographyProps('label') : {})} className="app-approval-title">{item.title || item.toolName || item.id}</div>
                    <div {...(sectionNavigation ? typographyProps('metadata') : {})} className="app-approval-subtitle">{item.status}</div>
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

      <RequestReviewDialog appearance={sectionNavigation ? 'document' : undefined} busy={approvalSubmitting}
        isOpen={!!selected}
        onClose={() => {
          if (approvalSubmitting) return;
          setSelected?.(null);
        }}
        title={selected?.title || selected?.toolName || 'Approval detail'}
        style={{ width: '70vw', minWidth: '70vw', maxWidth: '95vw' }}
      >
        <div className="app-approval-dialog">
          <div {...(sectionNavigation ? typographyProps('metadata') : {})} className="app-request-review-tool"><strong>Action:</strong> {selected?.toolName}</div>
          {canRenderQueueSchema ? (
            <div className={sectionNavigation ? "app-request-review-body" : undefined} style={sectionNavigation ? undefined : { marginTop: 12 }}>
              {selectedQueuePlannerMeta && sectionNavigation ? <ReviewTable id={`approval-${selected.id}`} title={selectedQueuePlannerMeta.title}
                rows={queuePlannerRows} columns={selectedQueuePlannerMeta.columns} selectionField={selectedQueuePlannerMeta.selectionField}
                selectionLabelFields={selectedQueuePlannerMeta.columns.slice(0,2).map(column=>column.key)} disabled={approvalSubmitting} showDownload={false}
                onRowsChange={next=>{setQueuePlannerRows(next);queueFormValuesRef.current={...queueFormValuesRef.current,[selectedQueuePlannerMeta.field]:next};}}/> : selectedQueuePlannerMeta ? (
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
                layout={sectionNavigation ? {kind:'grid',appearance:'field-tracks',columns:1,labels:{mode:'top'}} : undefined}
                showSubmit={false}
                schema={selectedQueueSchema}
                data={selectedQueueDisplayData}
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
            <pre {...(sectionNavigation ? typographyProps('code') : {})}>{JSON.stringify(selected?.arguments || {}, null, 2)}</pre>
          )}
          {approvalError ? <p {...(sectionNavigation ? typographyProps('body') : {})} role="alert" className={sectionNavigation ? "app-request-review-error" : undefined} style={sectionNavigation ? undefined : { color: '#ef4444', marginTop: 8 }}>{approvalError}</p> : null}
          <div className="app-approval-dialog-actions">
            {approvalSubmitting ? <Spinner size={16} /> : null}
            <ReviewAction appearance={sectionNavigation ? 'document' : undefined} intent="danger" disabled={approvalSubmitting} onClick={() => handleApprovalDecision(selected, 'reject')}>Reject</ReviewAction>
            <ReviewAction appearance={sectionNavigation ? 'document' : undefined}
              intent="primary"
              disabled={approvalSubmitting}
              onClick={() => handleApprovalDecision(selected, 'approve')}
            >
              {selectedQueueApproval?.acceptLabel || 'Approve'}
            </ReviewAction>
          </div>
        </div>
      </RequestReviewDialog>
    </header>
  );
}
