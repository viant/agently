import WorkspaceAppearanceSettings from "../../../ui/src/components/WorkspaceAppearanceSettings.jsx";
import {collectPreviewAccessOptions, withPreviewPrincipal, withPreviewTableMode, withPreviewTableWidth, authoredPreviewTableMode, authoredPreviewTableWidth} from './previewAccess.js';
import WorkspaceStyleProvider from "../../../ui/src/components/WorkspaceStyleProvider.jsx";
import React, { useEffect, useMemo, useRef, useState } from "react";
import { SettingProvider } from "forge/core/context/Setting.jsx";
import { activeWindows, selectedWindowId, getMetadataSignal } from "forge/core/store/signals.js";
import { runUICommand } from "forge/core/ui/commands.js";
import WindowManager from "forge/components/WindowManager.jsx";
import { readRoute, windowURL } from "./route.js";
import "./preview.css";
export default function WindowPreview() {
  const [appearanceOpen, setAppearanceOpen] = useState(() => window.innerWidth > 900);
  const [workspace, setWorkspace] = useState(null),
    [error, setError] = useState("");
  const [access, setAccess] = useState({
      roles: [],
      features: [],
      capabilities: {},
    });
  const [accessOptions, setAccessOptions] = useState({roles:[], features:[]});
  const selectionRef = useRef(null);
  const [tableMode, setTableMode] = useState('compact');
  const tableModeRef = useRef(null);
  const [tableWidthMode,setTableWidthMode] = useState('adaptive');
  const tableWidthModeRef = useRef(null);
  const [selected, setSelected] = useState(""),
    [variant, setVariant] = useState("default"),
    [parameters, setParameters] = useState("{}"),
    [query, setQuery] = useState("{}");
  const [initial, setInitial] = useState(null);
  const open = async (id, params = {}, catalog = workspace) => {
    if (!catalog?.windows[id]) throw new Error(`Unknown preview window: ${id}`);
    const response = await fetch(`/api/windows/${encodeURIComponent(id)}`, {
      cache: "no-store",
    });
    if (response.ok) {
      const payload = await response.json();
      const snapshot = payload?.data?.authorizationSnapshot || {};
      if(tableModeRef.current === null) setTableMode(authoredPreviewTableMode(payload?.data));
      if(tableWidthModeRef.current === null) setTableWidthMode(authoredPreviewTableWidth(payload?.data));
      setAccessOptions(collectPreviewAccessOptions(payload?.data || {}));
      setAccess({
        roles: selectionRef.current?.roles || snapshot?.principal?.roles || [],
        features: selectionRef.current?.features || snapshot?.principal?.features || [],
        capabilities: snapshot?.resource?.capabilities || {},
      });
    }
    await runUICommand({
      method: "ui.window.open",
      params: {
        windowKey: id,
        windowTitle: catalog.windows[id].title || id,
        parameters: params,
        inTab: true,
      },
    });
  };
  useEffect(() => {
    let live = true,
      timer;
    const refreshStyle = async (initialLoad = false) => {
      const response = await fetch("/api/workspace", { cache: "no-store" });
      if (!response.ok) throw new Error(await response.text());
      const catalog = await response.json();
      if (!live) return;
      setWorkspace(catalog);
      if (!initialLoad) return;
      const route = readRoute(location.search, catalog.defaultWindow);
      setWorkspace(catalog);
      setInitial(route);
      setVariant(route.variant);
      setQuery(JSON.stringify(route.query || {}, null, 2));
      setParameters(JSON.stringify(route.parameters, null, 2));
      setSelected(route.windowKey);
      await open(route.windowKey, route.parameters, catalog);
    };
    refreshStyle(true).catch((e) => setError(e.message));
    timer = setInterval(
      () => refreshStyle(false).catch((e) => setError(e.message)),
      1000,
    );
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, []);
  useEffect(() => {
    if (!workspace) return;
    const sync = () => {
      const entry = activeWindows
        .peek()
        .find((w) => w.windowId === selectedWindowId.peek());
      if (!entry) return;
      setSelected(entry.windowKey);
      setParameters(JSON.stringify(entry.parameters || {}, null, 2));
      const next = windowURL(location.href, entry);
      if (next !== location.pathname + location.search)
        history.pushState({}, "", next);
    };
    const a = activeWindows.subscribe(sync),
      b = selectedWindowId.subscribe(sync);
    const pop = () => {
      try {
        const route = readRoute(location.search, workspace.defaultWindow);
        open(route.windowKey, route.parameters).catch((e) =>
          setError(e.message),
        );
      } catch (e) {
        setError(e.message);
      }
    };
    addEventListener("popstate", pop);
    return () => {
      a();
      b();
      removeEventListener("popstate", pop);
    };
  }, [workspace]);
  useEffect(() => {
    const subscriptions = new Map();
    const sync = () => {
      const windows = activeWindows.peek();
      const ids = new Set(windows.map(w => w.windowId));
      for (const [id, unsubscribe] of subscriptions) if (!ids.has(id)) {unsubscribe();subscriptions.delete(id);}
      for (const win of windows) {
        if (subscriptions.has(win.windowId)) continue;
        const signal = getMetadataSignal(win.windowId);
        const applySelection = () => {
          const current = signal.peek();
          const next = withPreviewTableWidth(withPreviewTableMode(withPreviewPrincipal(current, selectionRef.current), tableModeRef.current),tableWidthModeRef.current);
          if (next !== current) signal.value = next;
        };
        subscriptions.set(win.windowId, signal.subscribe(applySelection));
      }
    };
    const unsubscribe = activeWindows.subscribe(sync);
    return () => {unsubscribe();for (const stop of subscriptions.values()) stop();};
  }, []);
  const changeTableWidth = mode => {
    tableWidthModeRef.current=mode;setTableWidthMode(mode);
    for(const win of activeWindows.peek()) {
      const signal=getMetadataSignal(win.windowId),current=signal.peek();
      const next=withPreviewTableWidth(current,mode);if(next!==current)signal.value=next;
    }
  };
  const changeTableMode = mode => {
    tableModeRef.current=mode;setTableMode(mode);
    for(const win of activeWindows.peek()) {
      const signal=getMetadataSignal(win.windowId), current=signal.peek();
      const next=withPreviewTableMode(current,mode);
      if(next!==current)signal.value=next;
    }
  };
  const changeAccess = (kind, value, checked) => {
    const next = {...access, [kind]: checked ? [...new Set([...access[kind],value])] : access[kind].filter(v=>v!==value)};
    selectionRef.current = {roles:next.roles,features:next.features};
    setAccess(next);
    for (const win of activeWindows.peek()) {
      const signal = getMetadataSignal(win.windowId);
      const current = signal.peek();
      const updated = withPreviewPrincipal(current,selectionRef.current);
      if (updated !== current) signal.value = updated;
    }
  };
  const endpoints = useMemo(
    () => ({ preview: { baseURL: location.origin } }),
    [],
  );
  const connectorConfig = useMemo(
    () => ({
      window: { service: { endpoint: "preview", uri: "/api/windows" } },
      tablePreferences: workspace?.tablePreferences || {adapter: 'browser'},
    }),
    [JSON.stringify(workspace?.tablePreferences || {})],
  );
  const services = useMemo(
    () => ({
      prepareDataConnectorRequest(request) {
        if (!request.url.includes("/datasources/")) return request;
        const body = {
          ...(request.body || {}),
          inputs: {
            ...(request.windowState?.parameters || {}),
            ...(request.body?.inputs || {}),
          },
          variant: initial?.variant || "default",
        };
        if (
          initial?.query &&
          request.windowState?.windowKey === initial.windowKey
        )
          body.inputs = { ...(body.inputs || {}), query: initial.query };
        return { ...request, body };
      },
    }),
    [initial],
  );
  const apply = () => {
    try {
      const params = JSON.parse(parameters),
        q = JSON.parse(query);
      const url = new URL(location.href);
      url.searchParams.set("window", selected);
      url.searchParams.set("variant", variant);
      url.searchParams.set("parameters", JSON.stringify(params));
      if (Object.keys(q).length)
        url.searchParams.set("query", JSON.stringify(q));
      else url.searchParams.delete("query");
      location.assign(url.href);
    } catch (e) {
      setError(e.message);
    }
  };
  const allowed = Object.values(access.capabilities).filter(Boolean).length;
  return (
    <div className="preview-app agently-workspace">
      <header className="preview-header">
        <h1>Window preview</h1>
        <span>Local simulation · read-only</span>
      </header>
      <main className="preview-main">
        <aside className="preview-sidebar">
          <label>
            Window
            <select
              value={selected}
              onChange={(e) => {
                setError("");
                open(e.target.value).catch((e) => setError(e.message));
              }}
            >
              {Object.entries(workspace?.windows || {}).map(([id, w]) => (
                <option key={id} value={id}>
                  {w.title || id}
                </option>
              ))}
            </select>
          </label>
          <label>
            Variant
            <select
              value={variant}
              onChange={(e) => setVariant(e.target.value)}
            >
              {["default", "empty", "error", "large"].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </label>
      <div className="preview-access-controls" aria-label="Simulated access">
        {['roles','features'].map(kind => <details className="preview-multiselect" key={kind}>
          <summary>{kind === 'roles' ? 'Roles' : 'Features'} <span>{access[kind].length} selected</span></summary>
          <fieldset><legend>{kind === 'roles' ? 'Simulated roles' : 'Simulated features'}</legend>
            {accessOptions[kind].length ? accessOptions[kind].map(value => <label key={value}>
              <input type="checkbox" checked={access[kind].includes(value)} onChange={event=>changeAccess(kind,value,event.target.checked)}/><span>{value}</span>
            </label>) : <p>No declared options</p>}
          </fieldset>
        </details>)}
        <label className="preview-table-mode">Table layout <select aria-label="Table layout" value={tableMode} onChange={e=>changeTableMode(e.target.value)}><option value="compact">Compact</option><option value="reserve10">Reserve 10 rows</option></select></label>
        <label className="preview-table-mode">Table width <select aria-label="Table width" value={tableWidthMode} onChange={e=>changeTableWidth(e.target.value)}><option value="adaptive">Adaptive</option><option value="trailing-space">Full width with trailing space</option></select></label>
        <span className="preview-capabilities">{allowed} resource capabilities allowed · unchanged by simulation</span>
      </div>
          <details>
            <summary>Preview inputs</summary>
            <label>
              Window parameters (JSON)
              <textarea
                value={parameters}
                onChange={(e) => setParameters(e.target.value)}
              />
            </label>
            <label>
              Query override (JSON)
              <textarea
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <button onClick={apply}>Apply and reload</button>
          </details>
          <a href="/api/workspace" target="_blank" rel="noreferrer">
            Workspace diagnostics
          </a>
        </aside>
        <section className="preview-window-host">
          {error && <div role="alert">{error}</div>}
          {variant === "empty" && (
            <div className="preview-state" role="status">
              Empty fixture: no advertiser records were returned. Change the variant to continue reviewing populated states.
            </div>
          )}
          {variant === "error" && (
            <div className="preview-state is-error" role="alert">
              Error fixture: datasource failures are active. Change the variant to retry with synthetic data.
            </div>
          )}
          {workspace && initial && (
            <WorkspaceStyleProvider
              metadata={workspace}
              accountKey="preview"
              workspaceKey={location.origin}
            >
              <SettingProvider
                endpoints={endpoints}
                connectorConfig={connectorConfig}
                services={services}
              >
                <div className="preview-themed-layout">
                  <div className="preview-rendered-window"><WindowManager /></div>
                  <details className="preview-appearance" aria-label="Preview appearance" open={appearanceOpen} onToggle={event => setAppearanceOpen(event.currentTarget.open)}><summary>Appearance</summary><WorkspaceAppearanceSettings /></details>
                </div>
              </SettingProvider>
            </WorkspaceStyleProvider>
          )}
        </section>
      </main>
    </div>
  );
}
