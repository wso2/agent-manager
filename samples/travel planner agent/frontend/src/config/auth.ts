const authConfig = {
  clientId: process.env.REACT_APP_ASGARDEO_CLIENT_ID || "HYzBc0mxP4KI6_HjoMr8KfZEVCEa",
  baseUrl: process.env.REACT_APP_ASGARDEO_BASE_URL || "https://api.asgardeo.io/t/smthing",
  scopes: (process.env.REACT_APP_ASGARDEO_SCOPES || "openid profile").split(" "),
  afterSignInUrl: process.env.REACT_APP_REDIRECT_SIGN_IN || window.location.origin,
  afterSignOutUrl: process.env.REACT_APP_REDIRECT_SIGN_OUT || window.location.origin,
  storage: "localStorage" as const,
};

export default authConfig;
