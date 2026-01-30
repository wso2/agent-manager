const authConfig = {
  clientId: "HYzBc0mxP4KI6_HjoMr8KfZEVCEa",
  baseUrl: "https://api.asgardeo.io/t/smthing",
  scopes: ["openid", "profile", "email"],
  afterSignInUrl: "http://localhost:3000",
  afterSignOutUrl: "http://localhost:3000",
  storage: "localStorage" as const,
};

export default authConfig;
