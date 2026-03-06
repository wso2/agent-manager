window.__RUNTIME_CONFIG__ = {
  authConfig: {
    signInRedirectURL: "$SIGN_IN_REDIRECT_URL",
    signOutRedirectURL: "$SIGN_OUT_REDIRECT_URL",
    clientID: "$AUTH_CLIENT_ID",
    baseUrl: "$AUTH_BASE_URL",
    scope: ["openid", "profile"],
    storage: "sessionStorage",
    validateIDToken: "$VALIDATE_ID_TOKEN" === "true",
    clockTolerance: 300,
  },
  disableAuth: true,
  apiBaseUrl: "http://localhost:9000",
  instrumentationUrl: "$INSTRUMENTATION_URL",
};
