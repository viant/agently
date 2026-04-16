import React from 'react';
import { Button, Classes, Dialog } from '@blueprintjs/core';
import { closeConfirmDialog, updateConfirmDialog, useConfirmDialogState } from '../utils/dialogBus';

export default function ConfirmDialog() {
  const state = useConfirmDialogState();

  const handleClose = () => {
    if (state.loading) return;
    closeConfirmDialog();
  };

  const handleConfirm = async () => {
    if (state.loading) return;
    updateConfirmDialog({ loading: true });
    try {
      await state.onConfirm?.();
      closeConfirmDialog();
    } catch (_) {
      updateConfirmDialog({ loading: false });
    }
  };

  return (
    <Dialog
      isOpen={state.open}
      onClose={handleClose}
      title={state.title}
      style={{ width: 'min(92vw, 480px)' }}
    >
      <div className={Classes.DIALOG_BODY}>
        <p>{state.message}</p>
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          <Button onClick={handleClose} disabled={state.loading}>
            {state.cancelText}
          </Button>
          <Button intent={state.intent} onClick={handleConfirm} loading={state.loading} disabled={state.loading}>
            {state.loading ? 'Deleting...' : state.confirmText}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
