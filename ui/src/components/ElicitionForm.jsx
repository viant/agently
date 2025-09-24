import React, {useMemo, useState} from 'react';
import {Dialog, Classes, Button, Spinner} from '@blueprintjs/core';
import {SchemaBasedForm} from 'forge/components';

import {endpoints} from '../endpoint';
import {joinURL} from '../utils/url';


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
    // no-op

    const closeLocal = () => {
        const collSig = context.Context('messages').signals?.collection;
        if (collSig) {
            collSig.value = (collSig.value || []).filter((m) => m.id !== id);
        }
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
            try { console.debug('[ElicitionForm:post]', {id, action, payload, URL, isAbsolute}); } catch (_) {}
            const resp = await fetch(URL, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({action, payload}),
            });
            if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`);
            try { console.debug('[ElicitionForm:post:ok]', {status: resp.status}); } catch (_) {}
            closeLocal(); // optimistic; server filter will hide it next poll
        } catch (e) {
            try { console.debug('[ElicitionForm:post:error]', e); } catch (_) {}
            setError(e.message || String(e));
        } finally {
            setSubmitting(false);
        }
    };

    const wrapperId = `elic-form-${id}`;
    const hasSchemaProps = !!(requestedSchema && requestedSchema.properties && Object.keys(requestedSchema.properties).length > 0);
    const isOOB = (mode === 'oob') || (!!url && !hasSchemaProps);
    try { console.debug('[ElicitionForm:init]', {id, mode, url, callbackURL, hasSchemaProps, isOOB}); } catch(_) {}
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
                {url && (
                    <div style={{marginBottom: 12}}>
                        <a href={url} target="_blank" rel="noopener noreferrer">
                            {url}
                        </a>
                    </div>
                )}

                {/* Inline JSON schema mode */}
                {hasSchemaProps && (
                    <div id={wrapperId}>
                        <SchemaBasedForm
                            showSubmit={false}
                            schema={requestedSchema}
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
                      <Button intent="primary" onClick={() => { try { console.debug('[ElicitionForm:oob:open]', {id, url}); } catch(_) {}; if (url) { window.open(url, '_blank', 'noopener,noreferrer'); } post('accept', {}); }} disabled={submitting}>
                        Open
                      </Button>
                    ) : (
                      hasSchemaProps ? (
                        <Button intent="primary" onClick={triggerInnerSubmit} disabled={submitting}>
                          Submit
                        </Button>
                      ) : (
                        <Button intent="primary" onClick={() => post('accept', {})} disabled={submitting}>
                          Accept
                        </Button>
                      )
                    )}
                </div>
            </div>
        </Dialog>
    );
}
