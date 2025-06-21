import React, {createContext} from 'react';

/*
 * Simple mock authentication context used by the local UI while the real
 * authentication subsystem is not yet integrated.  The shape of the object
 * mirrors what `forge` expects (see useDataConnector) – it must expose
 * `authStates` and `defaultAuthProvider` so that a JWT token can be picked up
 * when REST calls are made.
 */

export const AuthContext = createContext({
  authStates: {},
  defaultAuthProvider: undefined,
});

/**
 * A very small provider that puts a dummy JWT token into the context so that
 * forge's data-connector does not crash when it tries to read it. Replace this
 * with the real auth provider once one is available.
 */
export const AuthProvider = ({children}) => {
  const defaultAuthProvider = 'default';

  // Fake token – **development only**. All we need is an object with the same
  // shape; backend endpoints that require auth will obviously reject this
  // token, but components depending on its mere presence (e.g. Navigation
  // fetch) will continue to work.
  const authStates = {
    [defaultAuthProvider]: {
      jwtToken: {
        id_token: 'dummy-token',
      },
    },
  };

  return (
    <AuthContext.Provider value={{authStates, defaultAuthProvider}}>
      {children}
    </AuthContext.Provider>
  );
};
