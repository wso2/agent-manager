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

// Default console UI modules and their pages: true turns one on, false turns
// it off. A turned-off module or page has its routes, menu items and panels
// left unregistered, its activation hooks never run, and its page code is never
// loaded. The AI module manager cannot switch it back on.
//
// Each module is followed by its pages, keyed "<module>/<page>". A page runs
// only when both it and its module are on: turning a module off turns off all
// of its pages, whatever their own setting. An unlisted module or page is on.
//
// acp-core is built in and always on, so it is not listed.
//
// The console loads this file as-is before it starts. To change the defaults
// for a deployment, edit it or mount a replacement over it; no rebuild needed.
window.__UI_MODULES__ = {
  'agent-runtime': false,
  'agent-runtime/build': true,
  'agent-runtime/deploy': true,
  'agent-runtime/test': true,
  'agent-runtime/deployment-pipelines': true,

  'ai-gateway': true,
  'ai-gateway/gateways': true,
  'ai-gateway/llm-providers': true,
  'ai-gateway/mcp-proxies': true,
  'ai-gateway/agent-security': false,
  'ai-gateway/configure-agent': true,

  'ai-observability': true,
  'ai-observability/traces': true,
  'ai-observability/logs': false,
  'ai-observability/metrics': false,
  'ai-observability/eval': false,

  'agent-identity': true,
  'agent-identity/agent-identity': true,

  'agent-portal': false,
  'agent-portal/agent-kind': true,
};
