window.__RUNTIME_CONFIG__ = {
    authConfig: {
      signInRedirectURL: '$SIGN_IN_REDIRECT_URL',
      signOutRedirectURL: '$SIGN_OUT_REDIRECT_URL',
      clientID: '$AUTH_CLIENT_ID',
      baseUrl: '$AUTH_BASE_URL',
      scope: ['openid', 'profile'],
      storage: 'sessionStorage'
    },
    disableAuth: '$DISABLE_AUTH' === 'true',
    apiBaseUrl: '$API_BASE_URL',
    obsApiBaseUrl: '$OBS_API_BASE_URL'
  };
