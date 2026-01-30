import { createRoot } from "react-dom/client";
import { AsgardeoProvider } from "@asgardeo/react";
import App from "./App";
import authConfig from "./config/auth";
import "./index.css";

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <AsgardeoProvider
      clientId={authConfig.clientId}
      baseUrl={authConfig.baseUrl}
      scopes={authConfig.scopes}
      afterSignInUrl={authConfig.afterSignInUrl}
      afterSignOutUrl={authConfig.afterSignOutUrl}
      storage={authConfig.storage}
    >
      <App />
    </AsgardeoProvider>
  );
}
