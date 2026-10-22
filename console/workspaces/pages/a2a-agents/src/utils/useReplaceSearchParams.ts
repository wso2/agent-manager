/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';

export type SearchParamsMutation = (params: URLSearchParams) => void;

interface PendingWrite {
  base: URLSearchParams;
  next: URLSearchParams;
}

/**
 * [current params, updater]; the updater applies the mutation to a copy of the
 * latest params and replaces history.
 *
 * React Router hands a functional setSearchParams the params of the last
 * render, so two writes in one tick would drop the first. Each write records
 * the params it was based on; a later write from the same render builds on the
 * pending result, while a render with new params starts from those.
 *
 * Use one instance per page: two instances writing in the same tick still lose a write.
 */
export function useReplaceSearchParams(): [
  URLSearchParams,
  (mutate: SearchParamsMutation) => void,
] {
  const [searchParams, setSearchParams] = useSearchParams();
  const pending = useRef<PendingWrite | null>(null);

  const replaceSearchParams = useCallback(
    (mutate: SearchParamsMutation) => {
      const latest =
        pending.current?.base === searchParams ? pending.current.next : searchParams;
      const next = new URLSearchParams(latest);
      mutate(next);
      pending.current = { base: searchParams, next };
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams],
  );
  return [searchParams, replaceSearchParams];
}
