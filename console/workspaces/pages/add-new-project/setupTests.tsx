/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import '@testing-library/jest-dom';

import type { AppConfig } from "@agent-management-platform/types";

// Page components transitively import @agent-management-platform/auth via
// api-client hooks, which read globalConfig.disableAuth at module-evaluation
// time. The real app gets window.__RUNTIME_CONFIG__ from a server-injected
// config.js; stub the field actually read at import time.
window.__RUNTIME_CONFIG__ = { disableAuth: true } as AppConfig;

// React Router's v7 migration warnings and MUI/Oxygen's internal DOM-attribute
// and pseudo-class warnings are noise from third-party internals, not bugs in
// this page - silence them so real console errors/warnings stand out in test output.
const IGNORED_CONSOLE_PATTERNS = [
  /does not recognize the `.+` prop on a DOM element/,
];

const originalConsoleError = console.error;
const originalConsoleWarn = console.warn;

const isIgnoredConsoleMessage = (message: unknown) =>
  typeof message === "string" &&
  IGNORED_CONSOLE_PATTERNS.some((pattern) => pattern.test(message));

console.error = (...args: Parameters<typeof console.error>) => {
  if (isIgnoredConsoleMessage(args[0])) return;
  originalConsoleError(...args);
};

console.warn = (...args: Parameters<typeof console.warn>) => {
  if (isIgnoredConsoleMessage(args[0])) return;
  originalConsoleWarn(...args);
};
