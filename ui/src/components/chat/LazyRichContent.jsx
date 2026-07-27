import React from 'react';

const RichContent = React.lazy(() => import('./RichContent.jsx'));

function RichContentFallback() {
  return (
    <span className="app-rich-content-loading" role="status" aria-live="polite">
      Loading content...
    </span>
  );
}

export default function LazyRichContent(props) {
  return (
    <React.Suspense fallback={<RichContentFallback />}>
      <RichContent {...props} />
    </React.Suspense>
  );
}
