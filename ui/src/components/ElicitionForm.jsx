import React, {useMemo, useState} from 'react';
import {Dialog, Classes, Button, Spinner} from '@blueprintjs/core';
import {SchemaBasedForm} from 'forge/components';

import {endpoints} from '../endpoint';
import {joinURL} from '../utils/url';
import { markElicitationShown } from '../utils/elicitationBus';


/**
 * ElicitionForm – blueprint Dialog that renders a SchemaBasedForm based on the
 *          elicitation embedded in a message. Handles Accept / Decline
 *          / Cancel actions and POSTs the result to message.callbackURL.
 *
 * Props:
 *   message  – memory.Message with role=="mcp"
 *   context  – Forge window context (for chat.submitMessage & signals)
 */
export default function ElicitionForm({message, context}) {
    if (!message || !message.elicitation) return null;

    const {elicitation, callbackURL, id} = message;
    const {requestedSchema, message: prompt, url, mode} = elicitation;

    const [submitting, setSubmitting] = useState(false);
    const [error, setError] = useState('');
    const [formValues, setFormValues] = useState(null);

    const normalizeValues = (v) => {
        if (!v) return null;
        // Many Forge components pass {values: {...}} or {data: {...}}
        if (v.values && typeof v.values === 'object') return v.values;
        if (v.data && typeof v.data === 'object') return v.data;
        if (typeof v === 'object') return v;
        return null;
    };

    const defaultsPayload = useMemo(() => {
        try {
            const props = (requestedSchema && requestedSchema.properties) || {};
            const out = {};
            Object.keys(props).forEach((k) => {
                if (props[k] && props[k].default !== undefined) {
                    out[k] = props[k].default;
                }
            });
            return out;
        } catch (e) {
            return {};
        }
    }, [requestedSchema]);

    // Prepare schema to fix array/object defaults so form controls are editable
    const preparedSchema = useMemo(() => {
        try {
            if (!requestedSchema || typeof requestedSchema !== 'object') return requestedSchema;
            const clone = JSON.parse(JSON.stringify(requestedSchema));
            const props = (clone.properties = clone.properties || {});
            Object.keys(props).forEach((key) => {
                const p = props[key];
                if (!p || typeof p !== 'object') return;
                const t = (p.type || '').toLowerCase();
                if (t === 'array') {
                    if (p.default === undefined) p.default = [];
                    // Coerce mis-specified defaults like {} to [] to allow typing
                    if (p.default && !Array.isArray(p.default)) p.default = [];
                    const it = p.items || {};
                    const itType = (it.type || '').toLowerCase();
                    if (itType === 'string' && !p['x-ui-widget']) {
                        p['x-ui-widget'] = 'tags';
                    }
                } else if (t === 'object') {
                    if (p.default === undefined) p.default = {};
                }
                props[key] = p;
            });
            return clone;
        } catch(_) {
            return requestedSchema;
        }
    }, [requestedSchema]);
    // no-op

    const closeLocal = () => {
        const collSig = context.Context('messages').signals?.collection;
        if (collSig) {
            collSig.value = (collSig.value || []).filter((m) => m.id !== id);
        }
        try { markElicitationShown(id, 2500); } catch (_) {}
    };

    const post = async (action, payload) => {
        if (!callbackURL) return; // callbackURL is the contract; do not guess
        try {
            setSubmitting(true);
            setError('');
            // If callbackURL is absolute or root-relative, use as-is to avoid double prefixes
            const isAbsolute = /^https?:\/\//i.test(callbackURL);
            // Always target agentlyAPI when URL is not absolute (handles both relative and root-relative)
            const URL = isAbsolute
                ? callbackURL
                : joinURL(endpoints.agentlyAPI.baseURL, callbackURL);
            
            const resp = await fetch(URL, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({action, payload}),
            });
            if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`);
            
            closeLocal(); // optimistic; server filter will hide it next poll
        } catch (e) {
            
            setError(e.message || String(e));
        } finally {
            setSubmitting(false);
        }
    };

    React.useEffect(() => { return () => {}; }, []);

    const wrapperId = `elic-form-${id}`;
    const hasSchemaProps = !!(requestedSchema && requestedSchema.properties && Object.keys(requestedSchema.properties).length > 0);
    const simpleArrayKey = useMemo(() => {
        try {
            if (!hasSchemaProps) return null;
            const keys = Object.keys(requestedSchema.properties || {});
            if (keys.length !== 1) return null;
            const k = keys[0];
            const p = requestedSchema.properties[k] || {};
            if ((p.type || '').toLowerCase() !== 'array') return null;
            const it = p.items || {};
            if ((it.type || '').toLowerCase() !== 'string') return null;
            return k;
        } catch(_) { return null; }
    }, [requestedSchema, hasSchemaProps]);
    const isSingleArrayStringSchema = !!simpleArrayKey;
    const [arrayInput, setArrayInput] = useState(() => {
        try {
            if (!simpleArrayKey) return '';
            const d = defaultsPayload[simpleArrayKey];
            if (Array.isArray(d)) return d.join(', ');
            return '';
        } catch(_) { return ''; }
    });
    const parseArrayInput = (s) => (s || '')
        .split(/[,\n]/)
        .map(v => (v||'').trim())
        .filter(v => v.length > 0);
    const isWebOnly = (mode === 'webonly');
    const isOOB = (mode === 'oob') || isWebOnly || (!!url && !hasSchemaProps);
    try { console.debug('[ElicitionForm:init]', {id, mode, url, callbackURL, hasSchemaProps, isOOB}); } catch(_) {}
    try { markElicitationShown(id, 2500); } catch (_) {}
    const pickBoundValues = () => {
        try {
            // Forge stores dataBinding under window.state.answers.{id}
            const ans = (window && window.state && window.state.answers && window.state.answers[id]) || null;
            if (ans && typeof ans === 'object') {
                // accept plain, or values/data shapes
                if (ans.values && typeof ans.values === 'object') return ans.values;
                if (ans.data && typeof ans.data === 'object') return ans.data;
                return ans;
            }
        } catch(_) {}
        return null;
    };

    const collectFormFields = () => {
        try {
            const root = document.getElementById(wrapperId);
            if (!root) return null;
            const fields = root.querySelectorAll('input, select, textarea');
            const out = {};
            fields.forEach(el => {
                if (!el) return;
                const type = (el.getAttribute('type') || '').toLowerCase();
                const name = el.name || el.getAttribute('data-field') || el.getAttribute('id');
                if (!name) return;
                if (type === 'checkbox') {
                    out[name] = !!el.checked;
                } else if (type === 'radio') {
                    if (el.checked) out[name] = el.value;
                } else {
                    out[name] = el.value;
                }
            });
            // If we didn't capture anything, return null to allow other sources
            const hasAny = Object.keys(out).some(k => out[k] !== undefined && out[k] !== '');
            const res = hasAny ? out : null;
            return res;
        } catch (_) {
            return null;
        }
    };

    const triggerInnerSubmit = () => {
        try {
            const root = document.getElementById(wrapperId);
            if (!root) return;
            // Prefer a visible primary submit button if present
            const btn = root.querySelector('button[type="submit"], input[type="submit"]');
            if (btn && typeof btn.click === 'function') {
                btn.click();
                return;
            }
            // Fallback: submit the nearest form element
            const form = root.querySelector('form');
            if (form) {
                if (typeof form.requestSubmit === 'function') form.requestSubmit();
                else if (typeof form.submit === 'function') form.submit();
            }
        } catch (_) {}
    };

    // Attempt to extract values by schema keys heuristically from the DOM
    const collectBySchemaKeys = () => {
        try {
            const root = document.getElementById(wrapperId);
            if (!root) return null;
            const props = (requestedSchema && requestedSchema.properties) || {};
            const out = {};
            const matched = [];
            Object.keys(props).forEach((key) => {
                // Try multiple selectors to find a matching control
                const sel = [
                    `[name="${key}"]`, `[id="${key}"]`, `[data-field="${key}"]`,
                    `[name$=".${key}"]`, `[name$="[${key}]"]`, `[id$=".${key}"]`, `[id$="[${key}]"]`
                ].join(',');
                let el = root.querySelector(sel);
                if (!el) {
                    // Try label[for] → control mapping
                    const label = Array.from(root.querySelectorAll('label')).find(l => (l.textContent || '').trim().toLowerCase() === key.toLowerCase());
                    if (label) {
                        const forId = label.getAttribute('for');
                        if (forId) el = root.querySelector(`#${forId}`);
                    }
                }
                if (!el) return;
                const type = (el.getAttribute('type') || '').toLowerCase();
                if (type === 'checkbox') {
                    out[key] = !!el.checked;
                } else if (type === 'radio') {
                    if (el.checked) out[key] = el.value;
                } else {
                    out[key] = el.value;
                }
                matched.push(key);
            });
            const hasAny = Object.keys(out).some(k => out[k] !== undefined && out[k] !== '');
            const res = hasAny ? out : null;
            return res;
        } catch(_) {
            return null;
        }
    };

    return (
        <Dialog
            isOpen={true}
            canEscapeKeyClose={!submitting}
            canOutsideClickClose={!submitting}
            title="Additional input required"
            style={{width: '100%', maxWidth: 520}}>

            <fieldset className={Classes.DIALOG_BODY}>
                {/* Use the built-in form submit controls to capture values reliably */}
                {prompt && <p style={{marginBottom: 12}}>{prompt}</p>}
                {/* Out-of-band URL mode */}
                {url && (() => {
                    let display = url;
                    try {
                        const u = new URL(url);
                        display = u.host; // present domain only
                    } catch (_) {}
                    return (
                        <div style={{marginBottom: 12}}>
                            <span style={{marginRight: 6}}>Open in browser:</span>
                            <span style={{fontWeight: 600}}>{display}</span>
                        </div>
                    );
                })()}

                {/* Inline JSON schema mode */}
                {hasSchemaProps && !isSingleArrayStringSchema && (
                    <div id={wrapperId}>
                        <SchemaBasedForm
                            showSubmit={false}
                            schema={preparedSchema}
                            data={defaultsPayload}
                            dataBinding={`window.state.answers.${id}`}
                            transport="post"
                            onChange={(payload) => setFormValues(normalizeValues(payload))}
                            onSubmit={(payload) => {
                                const normalized = normalizeValues(payload) || payload || {};
                                post('accept', normalized);
                            }}
                        />
                    </div>

                )}
                {isSingleArrayStringSchema && (
                    <div style={{marginTop: 8}}>
                        <label htmlFor={`arr-${id}`} style={{display:'block', fontWeight:600}}>
                            {(simpleArrayKey || 'Items').charAt(0).toUpperCase() + (simpleArrayKey || 'Items').slice(1)}
                        </label>
                        <textarea id={`arr-${id}`} rows={3} style={{width:'100%'}}
                                  placeholder="Enter comma or newline separated values"
                                  value={arrayInput}
                                  onChange={e => setArrayInput(e.target.value)} />
                    </div>
                )}

                {error && <p style={{color: 'red', marginTop: 8}}>{error}</p>}
            </fieldset>

            <div className={Classes.DIALOG_FOOTER}>
                <div className={Classes.DIALOG_FOOTER_ACTIONS}>
                    {submitting && <Spinner size={16}/>}
                    <Button minimal onClick={() => post('decline')} disabled={submitting}>
                        Decline
                    </Button>
                    {!isOOB && (
                      <Button onClick={() => post('cancel')} disabled={submitting} style={{marginRight:8}}>
                          Cancel
                      </Button>
                    )}
                    {isOOB ? (
                      <>
                        <Button intent="primary" onClick={() => { try { markElicitationShown(id, 2500); } catch(_) {}; if (url) { window.open(url, '_blank', 'noopener,noreferrer'); } post('accept', {}); }} disabled={submitting} style={{marginRight: 8}}>
                          Open
                        </Button>
                        {isWebOnly && (
                          <Button onClick={() => post('accept', {})} disabled={submitting} style={{marginRight: 8}}>
                            OK
                          </Button>
                        )}
                      </>
                    ) : (
                      hasSchemaProps && !isSingleArrayStringSchema ? (
                        <Button intent="primary" onClick={triggerInnerSubmit} disabled={submitting}>
                          Submit
                        </Button>
                      ) : (
                        <Button intent="primary" onClick={() => {
                            if (isSingleArrayStringSchema) {
                                const payload = { [simpleArrayKey]: parseArrayInput(arrayInput) };
                                post('accept', payload);
                            } else {
                                post('accept', {});
                            }
                        }} disabled={submitting}>
                          Accept
                        </Button>
                      )
                    )}
                </div>
            </div>
        </Dialog>
    );
}
