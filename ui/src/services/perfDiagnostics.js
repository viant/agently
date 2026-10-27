import {installForgePerfDiagnostics} from 'forge/runtime/perfDiagnostics';

const metadataPath = path => /\/v1\/api\/agently\/forge\/window\//.test(path);
const dataPath = path => /\/v1\/api\/datasources\/[^/]+\/fetch$/.test(path);

// Host-only route and visible-panel selectors; timing collection lives in Forge.
export function installPerfDiagnostics(env = globalThis) {
  return installForgePerfDiagnostics({
    env,
    panelId:'agently-timing-diagnostics',armId:'agently-timing-arm',
    serverId:'agently-timing-server',outputId:'agently-timing-output',
    globalKey:'__agentlyPerfDiagnostics',panelTitle:'Timing diagnostics',
    classifyFetch:path=>metadataPath(path)?'metadata':dataPath(path)?'data':'',
    resourcePath:path=>metadataPath(path) || dataPath(path) || path==='/v1/ui/rpc',
    serverTimingPath:dataPath,
    findVisiblePanel:(document,label)=>[...document.querySelectorAll('.app-summary-workspace:not(.is-surface-hidden)')]
      .filter(section=>section.querySelector('.app-summary-workspace-title')?.textContent?.trim()===label)
      .map(section=>section.querySelector('.app-workspace-renderer[role="tabpanel"]:not([hidden]):not([aria-hidden="true"])'))
      .find(Boolean),
  });
}

// Root supplies the existing workspace-open callback only while mounted.
// Normal URLs install no diagnostics panel or diagnostic opener.
export function registerDiagnosticPublishedWindowOpener(callback) {
  const diagnostics=installPerfDiagnostics();
  if (!diagnostics) return () => {};
  diagnostics.setOpenPublishedWindow(callback);
  return () => diagnostics.setOpenPublishedWindow(null);
}

export async function registerReadyDiagnosticPublishedWindowOpener({waitReady,getTransport,isCurrent=()=>true,onOpen,register=registerDiagnosticPublishedWindowOpener}) {
  if (!await waitReady() || !isCurrent() || !getTransport()) return () => {};
  return register(onOpen);
}

export function createDiagnosticPublishedWindowOpener({open,activate,parentKey,conversationId='',isReady=()=>true}) {
  return ({providerIdentity,uri,title}) => {
    if (!isReady()) throw new Error('Workspace transport is unavailable');
    open(uri,title,[],{
      parameters:{},conversationId:conversationId || undefined,parentKey,
      presentation:'hosted',region:'chat.top',providerIdentity,resourceRef:{uri},
    });
    activate();
  };
}

installPerfDiagnostics();
