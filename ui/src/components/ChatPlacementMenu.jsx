import React from 'react';
import {Button, DropdownMenu} from 'forge/components';
import {Menu, MenuDivider, MenuItem} from '@blueprintjs/core';

// Both chat hosts use the same native menu, selection semantics and dismissal.
export default function ChatPlacementMenu({placement, onChange, splitDisabled = false, onResetPosition, resetDisabled = false}) {
  return <DropdownMenu placement="bottom-end" content={<Menu aria-label="Chat layout">
    <MenuItem text="Split on left" disabled={splitDisabled} aria-current={placement === 'left' ? 'true' : undefined} onClick={() => onChange('left')}/>
    <MenuItem text="Split on right" disabled={splitDisabled} aria-current={placement === 'right' ? 'true' : undefined} onClick={() => onChange('right')}/>
    <MenuItem text="Overlay" aria-current={placement === 'overlay' ? 'true' : undefined} onClick={() => onChange('overlay')}/>
    {placement === 'overlay' && onResetPosition ? <><MenuDivider/><MenuItem text="Reset position" disabled={resetDisabled} onClick={onResetPosition}/></> : null}
  </Menu>}>
    <Button variant="ghost" size="compact" hideLabel icon="lucide:ellipsis" aria-label="Chat options" title="Chat options"/>
  </DropdownMenu>;
}
