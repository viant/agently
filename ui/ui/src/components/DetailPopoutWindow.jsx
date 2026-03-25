import React from 'react';
import { createPortal } from 'react-dom';

function copyStyles(sourceDoc, targetDoc) {
  const links = Array.from(sourceDoc.querySelectorAll('link[rel="stylesheet"]'));
  for (const link of links) {
    const clone = targetDoc.createElement('link');
    clone.rel = 'stylesheet';
    clone.href = link.href;
    targetDoc.head.appendChild(clone);
  }
  const styles = Array.from(sourceDoc.querySelectorAll('style'));
  for (const style of styles) {
    const clone = targetDoc.createElement('style');
    clone.textContent = style.textContent || '';
    targetDoc.head.appendChild(clone);
  }
}

export default function DetailPopoutWindow({ title = 'Execution Detail', onClose, children }) {
  const [container, setContainer] = React.useState(null);
  const onCloseRef = React.useRef(onClose);
  React.useEffect(() => { onCloseRef.current = onClose; });

  React.useEffect(() => {
    const popup = window.open('', 'agently-app-execution-detail', 'width=1000,height=760,resizable=yes,scrollbars=yes');
    if (!popup) {
      if (typeof onCloseRef.current === 'function') onCloseRef.current();
      return undefined;
    }

    popup.document.title = title;
    copyStyles(document, popup.document);
    const root = popup.document.createElement('div');
    root.className = 'app-popout-root';
    popup.document.body.appendChild(root);
    setContainer(root);

    const closeHandler = () => {
      if (typeof onCloseRef.current === 'function') onCloseRef.current();
    };
    popup.addEventListener('beforeunload', closeHandler);

    return () => {
      popup.removeEventListener('beforeunload', closeHandler);
      try {
        if (!popup.closed) popup.close();
      } catch (_) {}
      setContainer(null);
    };
  }, [title]);

  if (!container) return null;
  return createPortal(children, container);
}
