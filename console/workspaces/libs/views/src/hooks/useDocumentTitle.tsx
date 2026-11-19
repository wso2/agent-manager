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

import { createContext, useContext, useEffect, useRef, type ReactNode } from 'react';

const DEFAULT_BASE_TITLE = 'Agent Manager Console';

/**
 * Base (prefix) for every tab title, e.g. "<baseTitle> | Agents". `undefined`
 * means "not provided" — fall back to the page's initial `document.title`.
 */
const DocumentTitleContext = createContext<string | undefined>(undefined);

export interface DocumentTitleProviderProps {
  children: ReactNode;
  /** Suffix shown after every page title in the browser tab. */
  baseTitle?: string;
}

export function DocumentTitleProvider({ children, baseTitle }: DocumentTitleProviderProps) {
  return (
    <DocumentTitleContext.Provider value={baseTitle?.trim() || undefined}>
      {children}
    </DocumentTitleContext.Provider>
  );
}

let initialDocumentTitle: string | null = null;

/**
 * Sets the browser tab title to `"<base> | <title>"` and restores the previous
 * title on unmount. The base comes from {@link DocumentTitleProvider}, else the
 * document's initial `<title>`, else `baseTitleFallback`.
 */
export function useDocumentTitle(title?: string, baseTitleFallback = DEFAULT_BASE_TITLE) {
  const providedBaseTitle = useContext(DocumentTitleContext);
  const prevTitleRef = useRef<string | null>(null);

  useEffect(() => {
    if (typeof document === 'undefined') {
      return;
    }

    if (initialDocumentTitle === null) {
      initialDocumentTitle = document.title || baseTitleFallback;
    }

    const baseTitle = providedBaseTitle ?? initialDocumentTitle;

    prevTitleRef.current = document.title;
    document.title = title ? `${baseTitle} | ${title}` : baseTitle;

    return () => {
      if (prevTitleRef.current !== null) {
        document.title = prevTitleRef.current;
      }
    };
  }, [baseTitleFallback, providedBaseTitle, title]);
}
