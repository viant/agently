import React, { forwardRef } from 'react';

export const MCP_UI_SANDBOX = 'allow-scripts';

const AppFrame = forwardRef(function AppFrame({ srcDoc = '', src = '', sandbox = MCP_UI_SANDBOX, title = 'MCP UI Resource', onLoad }, ref) {
  return (
    <iframe
      ref={ref}
      title={title}
      sandbox={sandbox}
      src={src || undefined}
      srcDoc={src ? undefined : srcDoc}
      onLoad={onLoad}
      style={{
        width: '100%',
        minHeight: '240px',
        border: '1px solid #d8dde6',
        borderRadius: '10px',
        background: '#fff',
      }}
    />
  );
});

export default AppFrame;
