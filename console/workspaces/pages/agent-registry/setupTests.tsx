import "@testing-library/jest-dom";

// globalConfig is window.__RUNTIME_CONFIG__ (config.js in the browser). Create
// it before any module reads it, so tests can set runtime config on it.
(window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> }).__RUNTIME_CONFIG__ ??= {};
