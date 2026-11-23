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

import { getProductFullName } from '@agent-management-platform/types';

/**
 * The full product name broken into two lines for the agent diagrams, at the word
 * break that leaves the lines closest in length, e.g. "Choreo AI & Agent" / "Management".
 * Never breaks next to "&", which reads as a dangling join.
 */
export function productNameLines(): [string, string] {
  const words = getProductFullName().split(' ');
  let best: [string, string] = [words.join(' '), ''];
  let bestDiff = Infinity;
  for (let i = 1; i < words.length; i++) {
    if (words[i - 1] === '&' || words[i] === '&') {
      continue;
    }
    const first = words.slice(0, i).join(' ');
    const second = words.slice(i).join(' ');
    const diff = Math.abs(first.length - second.length);
    if (diff < bestDiff) {
      best = [first, second];
      bestDiff = diff;
    }
  }
  return best;
}
