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

/**
 * Drops the auto-refresh timer id the ThunderID SDK stored on the previous
 * page. With localStorage the SDK keeps its refresh `setTimeout` id in
 * `temporary_data-instance_<n>-<clientId>`, and when the next page starts it
 * calls `clearTimeout` with that id. Timer ids are per page, so in the new page
 * the id can belong to any pending timer, typically the SDK's own 1 ms wait for
 * its initialisation (`isInitialized`); that wait then never ends and the
 * console never finishes loading. Nothing is lost: the old timer died with the
 * old page, and the SDK schedules a fresh one.
 *
 * A workaround for an SDK defect (still present in @thunderid/browser 1.26.1):
 * remove it once the SDK keeps the timer id in memory instead of in storage.
 * Another tab can still write its own id into the shared key while this page
 * starts; only the SDK change closes that.
 */
export function dropStaleRefreshTimer(clientId: string | undefined): void {
  if (!clientId) return;
  const suffix = `-${clientId}`;
  try {
    const storage = window.localStorage;
    Array.from({ length: storage.length }, (_, index) => storage.key(index))
      .filter(
        (key): key is string =>
          key !== null &&
          key.endsWith(suffix) &&
          /^temporary_data-instance_\d+$/.test(key.slice(0, -suffix.length)),
      )
      .forEach((key) => {
        try {
          const data = JSON.parse(storage.getItem(key) ?? "null");
          if (!data || typeof data !== "object" || !("refresh_token_timer" in data)) return;
          delete data.refresh_token_timer;
          storage.setItem(key, JSON.stringify(data));
        } catch {
          // Not the SDK's JSON: leave it to the SDK.
        }
      });
  } catch {
    return;
  }
}
