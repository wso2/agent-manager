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

// Default console UI modules: true turns a module on, false turns it off.
// A turned-off module's routes, menu items and panels are not registered, and
// its page code is never loaded. The AI module manager cannot switch it back on.
//
// To turn off a single page, add "<module>/<page>": false
// (e.g. "ai-observability/logs": false).
//
// acp-core is built in and always on, so it is not listed.
//
// The console loads this file as-is before it starts. To change the defaults
// for a deployment, edit it or mount a replacement over it; no rebuild needed.
window.__UI_MODULES__ = {
  'agent-runtime': false,
  'ai-gateway': true,
  'ai-observability': true,
  'agent-identity': true,
  'agent-portal': true,
};
