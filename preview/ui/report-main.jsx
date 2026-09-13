import React, {useEffect, useState} from 'react';
import {createRoot} from 'react-dom/client';
import '@blueprintjs/core/lib/css/blueprint.css';
import '@blueprintjs/icons/lib/css/blueprint-icons.css';
import './report/preview.css';

window.global ||= window;
window.process ||= {env: {}};

function App() {
  const [artifact, setArtifact] = useState(null);
  const [error, setError] = useState('');
  const [ReportRuntime, setReportRuntime] = useState(null);

  const runtimeReportSpec = artifact?.reportSpec && artifact?.reportFill
    ? {
        ...artifact.reportSpec,
        blocks: (artifact.reportSpec.blocks || []).map((block) => {
          if (block?.kind !== 'sectionBlock') return block;
          const fillBlock = (artifact.reportFill.blocks || []).find((candidate) => candidate?.id === block.id);
          const blockIds = fillBlock?.content?.blockIds;
          return Array.isArray(blockIds)
            ? {...block, content: {...(block.content || {}), blockIds}}
            : block;
        }),
      }
    : artifact?.reportSpec;

  useEffect(() => {
    let active = true;
    const compileQuery = new URLSearchParams(window.location.search);
    compileQuery.set('full', 'true');
    Promise.all([
      import('forge-runtime'),
      import('forge/components/dashboard/ReportRuntime.jsx'),
      fetch(`/api/compile?${compileQuery.toString()}`).then(async (response) => {
        const body = await response.json();
        if (!response.ok) throw new Error(body?.message || `Preview compile failed (${response.status})`);
        return body;
      }),
    ]).then(([, runtime, compiled]) => {
      if (!active) return;
      setReportRuntime(() => runtime.default);
      setArtifact(compiled);
      document.title = compiled?.reportDocument?.title || 'Forge reporting preview';
    }).catch((cause) => active && setError(String(cause?.message || cause)));
    return () => { active = false; };
  }, []);

  if (error) return <main className="report-preview-state is-error"><h1>Report unavailable</h1><p>{error}</p></main>;
  if (!artifact || !ReportRuntime) return <main className="report-preview-state"><p>Loading report…</p></main>;
  return (
    <main className="report-preview-runtime" data-preview-source="report-catalog">
      <ReportRuntime
        reportDocument={artifact.reportDocument}
        reportSpec={runtimeReportSpec}
        reportFill={artifact.reportFill}
        conditionValues={artifact.conditionValues}
        title={artifact.reportDocument?.title || ''}
        subtitle={artifact.reportDocument?.subtitle || ''}
        presentationMode="report"
        showContextSummary={false}
        showDeveloperDiagnostics={false}
        defaultContextSummaryOpen={false}
      />
    </main>
  );
}

createRoot(document.getElementById('root')).render(<App/>);
