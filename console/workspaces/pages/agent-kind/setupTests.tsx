import "@testing-library/jest-dom";

import type { AppConfig } from "@agent-management-platform/types";

// Page components transitively import @agent-management-platform/auth via
// api-client hooks, which read globalConfig.disableAuth at module-evaluation
// time. The real app gets window.__RUNTIME_CONFIG__ from a server-injected
// config.js; stub the field actually read at import time.
window.__RUNTIME_CONFIG__ = { disableAuth: true } as AppConfig;
