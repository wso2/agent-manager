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

// AMP instrumentation + Python version options, fetched from
// GET /api/v1/orgs/{orgName}/agent-build-options. Replaces the previously
// hardcoded SUPPORTED_INSTRUMENTATION_VERSIONS / SUPPORTED_PYTHON_VERSIONS
// constants; the server's instrumentation catalog is the source of truth.
export type AgentBuildOptions = {
  python: {
    defaultVersion: string;
    supportedVersions: string[];
  };
  instrumentation: {
    defaultVersion: string;
    versions: Array<{
      version: string;
      /** Bundled OpenLLMetry (Traceloop) SDK version, when known. */
      traceloopSdk?: string;
      pythonVersions: string[];
    }>;
  };
};

export type InstrumentationVersionEntry =
  AgentBuildOptions["instrumentation"]["versions"][number];
