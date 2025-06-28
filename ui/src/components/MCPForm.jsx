import React, {useState} from 'react';
import {Dialog, Classes, Button, Spinner} from '@blueprintjs/core';
import {SchemaBasedForm} from 'forge/components';

import {endpoints} from '../endpoint';
import {joinURL} from '../utils/url';


/**
 * MCPForm – blueprint Dialog that renders a SchemaBasedForm based on the
 *          elicitation embedded in an MCP message. Handles Accept / Decline
 *          / Cancel actions and POSTs the result to message.callbackURL.
 *
 * Props:
 *   message  – memory.Message with role=="mcp"
 *   context  – Forge window context (for chat.submitMessage & signals)
 */
export default function MCPForm({message, context}) {
    if (!message || !message.elicitation) return null;

    const {elicitation, callbackURL, id} = message;
    const {requestedSchema, message: prompt} = elicitation;

    const [submitting, setSubmitting] = useState(false);
    const [error, setError] = useState('');

    const closeLocal = () => {
        const collSig = context.Context('messages').signals?.collection;
        if (collSig) {
            collSig.value = (collSig.value || []).filter((m) => m.id !== id);
        }
    };

    const post = async (action, payload) => {
        if (!callbackURL) return;
        try {
            setSubmitting(true);
            setError('');
            const URL = joinURL(endpoints.appAPI.baseURL, callbackURL)
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

    return (
        <Dialog
            isOpen={true}
            canEscapeKeyClose={!submitting}
            canOutsideClickClose={!submitting}
            title="Additional input required"
            style={{width: 'auto', maxWidth: 520}}>

            <fieldset className={Classes.DIALOG_BODY}>
                {prompt && <p style={{marginBottom: 12}}>{prompt}</p>}
                {requestedSchema && (
                    <div style={{width: '90vw', maxWidth: '90vw', margin: '0 auto'}}>
                        <SchemaBasedForm
                            schema={requestedSchema}
                            dataBinding={`window.state.answers.${id}`}
                            transport="post"
                            onSubmit={(payload) => post('accept', payload)}
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
                    <Button onClick={() => post('cancel')} disabled={submitting}>
                        Cancel
                    </Button>
                </div>
            </div>
        </Dialog>
    );
}
