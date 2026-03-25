import React, { useEffect, useState } from 'react';
import { AgentlyClient } from 'agently-core-ui-sdk';
import { client } from '../services/agentlyClient';

/**
 * SPA route handler for /v1/api/auth/oauth/callback.
 *
 * After the IDP redirects back with ?code=...&state=..., this component
 * sends them to the backend via the SDK to complete the token exchange,
 * then redirects to the saved returnURL.
 */
export default function OAuthCallback() {
  const [status, setStatus] = useState('processing');
  const [error, setError] = useState('');

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code') || '';
    const state = params.get('state') || '';

    if (!code || !state) {
      setStatus('error');
      setError('Missing OAuth code or state parameter.');
      return;
    }

    client.oauthCallback({ code, state })
      .then(() => {
        setStatus('success');
        const returnTo = AgentlyClient.getLoginReturnURL();
        window.location.replace(returnTo);
      })
      .catch((err) => {
        setStatus('error');
        setError(String(err?.message || err || 'OAuth callback failed'));
      });
  }, []);

  if (status === 'error') {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', height: '100vh', gap: 12 }}>
        <div style={{ color: '#ef4444', fontWeight: 500 }}>Authentication failed</div>
        <div style={{ color: '#9ca3af', fontSize: 13 }}>{error}</div>
        <button onClick={() => window.location.replace('/')} style={{ marginTop: 8, cursor: 'pointer' }}>
          Return to app
        </button>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', color: '#9ca3af' }}>
      Completing authentication...
    </div>
  );
}
