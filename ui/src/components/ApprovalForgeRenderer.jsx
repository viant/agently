import React, { useEffect, useMemo } from 'react';
import { Container } from 'forge/components';
import { activeWindows, getWindowContext, selectedWindowId } from 'forge/core';
import { buildApprovalForgeSchema } from 'agently-core-ui-sdk';

function resolveWindowContext(windowRef = '') {
  const targetRef = String(windowRef || '').trim();
  if (targetRef) {
    const windows = Array.isArray(activeWindows?.peek?.()) ? activeWindows.peek() : [];
    const match = windows.find((entry) => String(entry?.windowKey || '').trim() === targetRef || String(entry?.windowId || '').trim() === targetRef);
    if (match?.windowId) {
      return getWindowContext?.(match.windowId) || null;
    }
  }
  const currentWindowId = String(selectedWindowId?.value || '').trim();
  if (!currentWindowId) return null;
  return getWindowContext?.(currentWindowId) || null;
}

function findContainerById(container = null, targetId = '') {
  if (!container || !targetId) return null;
  if (String(container.id || '').trim() === targetId) return container;
  const nested = Array.isArray(container.containers) ? container.containers : [];
  for (const item of nested) {
    const found = findContainerById(item, targetId);
    if (found) return found;
  }
  return null;
}

function resolveApprovalContainer(baseContext, containerRef = '') {
  const targetId = String(containerRef || '').trim();
  const rootContainers = Array.isArray(baseContext?.metadata?.view?.content?.containers)
    ? baseContext.metadata.view.content.containers
    : [];
  for (const item of rootContainers) {
    const found = findContainerById(item, targetId);
    if (found) return found;
  }
  const dialogs = Array.isArray(baseContext?.metadata?.dialogs) ? baseContext.metadata.dialogs : [];
  for (const dialog of dialogs) {
    const dialogContent = dialog?.content;
    if (!dialogContent || typeof dialogContent !== 'object') continue;
    const found = findContainerById(dialogContent, targetId);
    if (found) return found;
    const nested = Array.isArray(dialogContent?.containers) ? dialogContent.containers : [];
    for (const item of nested) {
      const nestedFound = findContainerById(item, targetId);
      if (nestedFound) return nestedFound;
    }
  }
  return null;
}

export default function ApprovalForgeRenderer({ meta, approvalValues, originalArgs, onReady, onError, isActive = true }) {
  const forge = meta?.forge;
  const baseContext = useMemo(() => resolveWindowContext(forge?.windowRef), [forge?.windowRef]);
  const container = useMemo(() => resolveApprovalContainer(baseContext, forge?.containerRef), [baseContext, forge?.containerRef]);
  const dataSourceRef = String(forge?.dataSource || container?.dataSourceRef || baseContext?.identity?.dataSourceRef || '').trim();
  const renderContext = useMemo(() => {
    if (!baseContext || !dataSourceRef) return null;
    try {
      return baseContext.Context(dataSourceRef);
    } catch (_) {
      return null;
    }
  }, [baseContext, dataSourceRef]);

  useEffect(() => {
    if (!renderContext?.handlers?.dataSource?.setFormData) return;
    renderContext.handlers.dataSource.setFormData({
      values: {
        approval: meta || {},
        editedFields: approvalValues || {},
        originalArgs: originalArgs || {},
        approvalSchemaJSON: JSON.stringify(buildApprovalForgeSchema(meta)),
      },
    });
    onReady?.(renderContext);
  }, [approvalValues, meta, onReady, originalArgs, renderContext]);

  useEffect(() => {
    if (renderContext && container) {
      onError?.('');
      return;
    }
    const reasons = [];
    if (!baseContext) reasons.push('window context not found');
    if (!container) reasons.push(`container "${String(forge?.containerRef || '').trim()}" not found`);
    if (!dataSourceRef) reasons.push('dataSource not resolved');
    if (!renderContext) reasons.push('data source context not available');
    onReady?.(null);
    onError?.(`Forge approval view unavailable: ${reasons.join(', ')}`);
  }, [baseContext, container, dataSourceRef, forge?.containerRef, onError, onReady, renderContext]);

  if (!renderContext || !container) {
    return (
      <div className="app-approval-forge app-approval-forge-error">
        Forge approval view unavailable.
      </div>
    );
  }
  return (
    <div className="app-approval-forge">
      <Container context={renderContext} container={container} isActive={isActive} />
    </div>
  );
}
