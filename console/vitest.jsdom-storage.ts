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

// Node 25 and later define their own localStorage and sessionStorage globals,
// which shadow jsdom's and fail without --localstorage-file. Point the globals
// back at jsdom's, so tests behave the same on every Node version.
declare const jsdom: { window: Window } | undefined;

if (typeof jsdom !== "undefined") {
  for (const name of ["localStorage", "sessionStorage"] as const) {
    Object.defineProperty(globalThis, name, {
      value: jsdom.window[name],
      configurable: true,
      writable: true,
    });
  }
}
