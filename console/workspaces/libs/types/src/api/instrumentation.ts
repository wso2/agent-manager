/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

// AMP instrumentation versions the platform supports. Each version maps to a
// pre-built init-container image (`amp-python-instrumentation-provider:<version>-python<X.Y>`)
// with a specific pinned `traceloop-sdk`. Server-side source of truth:
// agent-manager-service `OTEL_SUPPORTED_INSTRUMENTATION_VERSIONS` env (default `["0.2.0"]`).
export const SUPPORTED_INSTRUMENTATION_VERSIONS = ['0.2.0'] as const;
export const DEFAULT_INSTRUMENTATION_VERSION: SupportedInstrumentationVersion = '0.2.0';
export type SupportedInstrumentationVersion =
  (typeof SUPPORTED_INSTRUMENTATION_VERSIONS)[number];

// Python versions supported by the platform's buildpack. The instrumentation
// init-container image is ABI-locked to the agent's Python runtime, so this
// set must match the `python_versions` entries in
// `.github/release-config.json` for the `python-instrumentation-provider`.
export const SUPPORTED_PYTHON_VERSIONS = ['3.10', '3.11', '3.12', '3.13'] as const;
export const DEFAULT_PYTHON_VERSION: SupportedPythonVersion = '3.11';
export type SupportedPythonVersion = (typeof SUPPORTED_PYTHON_VERSIONS)[number];
