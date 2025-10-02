// CollapsibleCard.jsx – simple, reusable card with colored header and toggle
import React from 'react';
import { Card, Icon, Elevation } from '@blueprintjs/core';

export default function CollapsibleCard({
  title,
  icon = 'time',
  right = null,
  defaultOpen = true,
  width = '100%',
  intent = 'primary',
  children,
}) {
  const [open, setOpen] = React.useState(!!defaultOpen);
  const toggle = () => setOpen(v => !v);

  const palette = intent === 'success'
    ? { bg: '#ecfdf5', border: '#a7f3d0', fg: '#065f46' }
    : intent === 'warning'
      ? { bg: '#fffbeb', border: '#fde68a', fg: '#92400e' }
      : intent === 'danger'
        ? { bg: '#fef2f2', border: '#fecaca', fg: '#991b1b' }
        : { bg: '#eef2ff', border: '#c7d2fe', fg: '#3730a3' }; // primary

  return (
    <Card elevation={Elevation.ONE} style={{ width, maxWidth: '100%', padding: 0, overflow: 'hidden' }}>
      <div
        role="button"
        onClick={toggle}
        style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          gap: 8, padding: '8px 12px', cursor: 'pointer',
          background: palette.bg, borderBottom: `1px solid ${palette.border}`, color: palette.fg,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Icon icon={icon} size={14} />
          <span style={{ fontWeight: 600 }}>{title}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {right}
          <Icon icon={open ? 'chevron-down' : 'chevron-right'} />
        </div>
      </div>
      {open && (
        <div style={{ padding: 12 }}>
          {children}
        </div>
      )}
    </Card>
  );
}
