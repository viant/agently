import React from 'react';
import { Card, Button, H3, H5, Spinner, Icon, Divider } from '@blueprintjs/core';
import { useSetting } from 'forge/core';

const SignIn = () => {
  const { useAuth } = useSetting();
  const { providers = [], ready, loginBFF, loginSPA, loginSPAWithToken, loginLocal } = useAuth();

  const bff = providers.find(p => p.type === 'bff');
  const hasBFF = !!bff;
  const oidc = providers.find(p => p.type === 'oidc');
  const local = providers.find(p => p.type === 'local');
  const providerCount = [hasBFF, !!oidc, !!local].filter(Boolean).length;
  const hasAnyProviders = providerCount > 0;

  const bffLabel = bff?.label || bff?.name || 'Your Provider';
  const oidcLabel = oidc?.label || oidc?.name || 'OIDC';

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', padding: 24 }}>
      <Card elevation={2} style={{ maxWidth: 560, width: '100%', textAlign: 'center', padding: 28 }}>
        <div style={{display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8}}>
          <Icon icon="log-in" size={28} style={{color: '#137CBD'}}/>
          <H3 style={{ margin: 0 }}>{providerCount > 1 ? 'Get started' : 'Welcome'}</H3>
          <H5 style={{ marginTop: 0, fontWeight: 400, color: '#5c7080' }}>
            {providerCount > 1 ? 'Choose a sign in method' : 'Continue to your workspace'}
          </H5>
        </div>

        {!ready ? (
          <div style={{marginTop: 24}}><Spinner/></div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 20 }}>
            {(hasBFF || typeof loginBFF === 'function') && (
              <Button large intent="primary" onClick={() => loginBFF && loginBFF()}>
                {hasBFF ? `Sign in with ${bffLabel}` : 'Sign in'}
              </Button>
            )}

            {oidc && (
              <Button large onClick={() => loginSPA && loginSPA()}>
                Continue with {oidcLabel}
              </Button>
            )}

            {local && (
              <Button large onClick={async () => {
                const name = local.defaultUsername || window.prompt('Enter username');
                if (name) { await (loginLocal && loginLocal(name)); }
              }}>
                {local.defaultUsername ? `Continue as ${local.defaultUsername}` : 'Continue as local user'}
              </Button>
            )}

            {ready && !hasAnyProviders && !(typeof loginBFF === 'function') && (
              <div style={{ color: '#5c7080', marginTop: 8 }}>No sign-in providers detected.</div>
            )}
          </div>
        )}
      </Card>
    </div>
  );
};

export default SignIn;
