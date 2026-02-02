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

import { useEffect, useRef } from 'react';

let initialBaseTitle: string | null = null;

export function useDocumentTitle(
  title?: string,
  baseTitleFallback = 'Agent Manager Console'
) {
  const baseTitleRef = useRef<string | null>(null);
  const prevTitleRef = useRef<string | null>(null);

  useEffect(() => {
    if (typeof document === 'undefined') {
      return;
    }

    if (initialBaseTitle === null) {
      initialBaseTitle = document.title || baseTitleFallback;
    }

    if (baseTitleRef.current === null) {
      baseTitleRef.current = initialBaseTitle;
    }

    prevTitleRef.current = document.title;

    const baseTitle = baseTitleRef.current;

    if (!title) {
      document.title = baseTitle;
      return () => {
        if (prevTitleRef.current !== null) {
          document.title = prevTitleRef.current;
        }
      };
    }

    document.title = `${title} | ${baseTitle}`;

    return () => {
      if (prevTitleRef.current !== null) {
        document.title = prevTitleRef.current;
      }
    };
  }, [baseTitleFallback, title]);
}
