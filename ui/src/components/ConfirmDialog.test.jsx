// @vitest-environment jsdom
import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import ConfirmDialog from './ConfirmDialog.jsx';
import { closeConfirmDialog, openConfirmDialog } from '../utils/dialogBus.js';

describe('ConfirmDialog', () => {
  it('displays inline error feedback inside the dialog when onConfirm rejects', async () => {
    closeConfirmDialog();
    openConfirmDialog({
      title: 'Delete conversation',
      message: 'Delete "Old chat" and its history?',
      confirmText: 'Delete',
      onConfirm: vi.fn().mockRejectedValue(new Error('Only the conversation owner can delete this conversation.')),
    });

    render(<ConfirmDialog />);
    expect(screen.getByText('Delete "Old chat" and its history?')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(screen.getByRole('alert').textContent).toContain(
        'Only the conversation owner can delete this conversation.'
      );
    });
    closeConfirmDialog();
  });
});
