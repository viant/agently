// forgeBridge.js — translate the agently overlay attachment into forge
// Item.Lookup shape, and register a DataSource binding that dispatches
// through /v1/api/datasources/{id}/fetch instead of forge's default Service.
//
// Server-side, service/lookupoverlay.Apply emits on each matched property:
//
//   x-ui-widget: "lookup"
//   x-ui-lookup:
//     dataSource: <datasource-id>
//     dialogId:   <forge-dialog-id>   (or windowId)
//     inputs:  [...]  forge Parameter list (defaults: :form → :query)
//     outputs: [...]  forge Parameter list (defaults: :output → :form)
//     display: "${label}"
//
// Forge's Item.Lookup expects the same fields on `item.lookup`, so for most
// cases this is a 1:1 rename. The bridge does two things:
//
//   1. translateSchema(schema):
//         walk properties; where x-ui-widget=="lookup", set
//         item.lookup = attachment; leave x-ui-* in place for future
//         consumers.
//
//   2. makeDatasourceService(id):
//         returns a forge-compatible Service that calls fetchDatasource(id, …)
//         under the hood. The forge dialog's DataSource.service can be swapped
//         to this at window-load time.

import { fetchDatasource } from './client.js';

/**
 * Walk a JSON Schema produced server-side, attach forge Item.Lookup metadata
 * where the overlay has annotated `x-ui-widget: lookup`, and return a
 * reference-mutated copy suitable for handing to forge.
 *
 * The schema object is mutated in place for performance; callers that need
 * immutability should structuredClone before passing it in.
 */
export function translateSchema(schema) {
  if (!schema || !schema.properties) return schema;
  for (const [key, value] of Object.entries(schema.properties)) {
    if (!value || typeof value !== 'object') continue;
    if (value['x-ui-widget'] !== 'lookup') continue;
    const att = value['x-ui-lookup'];
    if (!att) continue;
    // Forge expects `lookup` directly; keep x-ui-lookup for debugging but
    // also surface as item.lookup so the forge renderer picks it up.
    value.lookup = {
      dialogId: att.dialogId,
      windowId: att.windowId,
      inputs: att.inputs || [],
      outputs: att.outputs || [],
      // Intent / title are optional — the dialog YAML carries them.
    };
    // Stash the datasource id so the dialog's DataSource can be wired at
    // open time via resolveDatasourceBinding.
    value.lookup.dataSource = att.dataSource;
    value.lookup.title = att.title;
    value.lookup.display = att.display;
    value.lookup.queryInput = att.queryInput;
    value.lookup.resolveInput = att.resolveInput;
  }
  return schema;
}

/**
 * Build a forge-compatible Service object whose "call" routes through
 * /v1/api/datasources/{id}/fetch. Forge's default Service expects a URL +
 * method; we adapt that contract into an `executor` function the dialog's
 * DataSource can call.
 *
 * Consumers that have a custom forge DataSource.service adapter can use this
 * directly; otherwise they can call fetchDatasource(id, inputs) explicitly
 * from a DataSource.on("fetch") handler.
 */
export function makeDatasourceService(id) {
  return {
    kind: 'agently-datasource',
    id,
    async call(inputs) {
      const res = await fetchDatasource(id, inputs || {});
      // Return the forge-shaped envelope: rows on `data`, pagination on
      // `dataInfo`. Forge selectors will already be no-ops because we did
      // projection server-side.
      return { data: res.rows || [], dataInfo: res.dataInfo || null };
    },
  };
}

/**
 * Convenience: given a schema already passed through translateSchema, return
 * a flat list of (propertyName, datasourceId) pairs so the caller can pre-
 * register DataSource bindings on the forge window before rendering.
 */
export function extractLookupBindings(schema) {
  const out = [];
  if (!schema || !schema.properties) return out;
  for (const [name, value] of Object.entries(schema.properties)) {
    if (value && value.lookup && value.lookup.dataSource) {
      out.push({ property: name, dataSource: value.lookup.dataSource });
    }
  }
  return out;
}

// ─────────────────────────────────────────────────────────────────────────────
// Client-side datasource registry.
//
// Forge's JS runtime drives DataSource fetches through a signal-based
// `input.value.fetch = true` path rather than a `DataSource.service.call(...)`
// plug point. That means we cannot override the Service from JSON Schema
// metadata alone — forge will still dispatch via its own endpoint config.
//
// The registry below is the bridge: every datasource id that participates in
// a lookup (as emitted server-side into `x-ui-lookup`) gets a fetch closure
// registered here. A small amount of forge wiring (see below) consults this
// registry at open time: if the datasource id is registered, forge uses our
// fetch instead of its default one. This keeps the server-emitted schema as
// the single source of truth for which datasources should be lookup-backed.
//
// Until that forge wiring lands, calling `registerLookupDataSourceServices`
// still serves as the **handshake** the eventual forge integration will
// consult — populating the registry makes the translation side idempotent
// and observable in tests.
// ─────────────────────────────────────────────────────────────────────────────

const LOOKUP_DS_REGISTRY = new Map(); // dataSourceId -> Service

/**
 * Register a forge-compatible Service for each (property, dataSourceId)
 * binding produced by extractLookupBindings. Idempotent — re-registering the
 * same id replaces the Service.
 */
export function registerLookupDataSourceServices(bindings) {
  if (!Array.isArray(bindings)) return;
  for (const b of bindings) {
    if (!b || !b.dataSource) continue;
    if (!LOOKUP_DS_REGISTRY.has(b.dataSource)) {
      LOOKUP_DS_REGISTRY.set(b.dataSource, makeDatasourceService(b.dataSource));
    }
  }
}

/**
 * Look up the registered Service for a datasource id. Returns undefined when
 * the id is not lookup-backed. Forge integration code should consult this
 * before falling back to its default Service resolver.
 */
export function getLookupDataSourceService(id) {
  if (!id) return undefined;
  return LOOKUP_DS_REGISTRY.get(id);
}

/** Visible for tests. */
export function _resetLookupDataSourceRegistry() {
  LOOKUP_DS_REGISTRY.clear();
}
