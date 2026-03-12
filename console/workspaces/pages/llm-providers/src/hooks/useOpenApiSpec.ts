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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useState } from "react";

const specCache = new Map<string, string>();

export function useOpenApiSpec(
  url: string | undefined,
  fallbackText?: string,
): {
  text: string;
  setText: (value: string) => void;
  isLoading: boolean;
  error: Error | null;
} {
  const [text, setText] = useState(fallbackText ?? "");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    setText(fallbackText ?? "");
  }, [fallbackText]);

  useEffect(() => {
    if (!url || (fallbackText ?? "").trim()) return;

    const cached = specCache.get(url);
    if (cached) {
      setText(cached);
      return;
    }

    const controller = new AbortController();
    setIsLoading(true);
    setError(null);
    fetch(url, { signal: controller.signal })
      .then((r) => {
        if (!r.ok) {
          throw new Error(`Failed to fetch: ${r.status} ${r.statusText}`);
        }
        return r.text();
      })
      .then((t) => {
        specCache.set(url, t);
        setText(t);
      })
      .catch((err) => {
        if (err?.name !== "AbortError") {
          setError(err instanceof Error ? err : new Error(String(err)));
        }
      })
      .finally(() => setIsLoading(false));

    return () => controller.abort();
  }, [url, fallbackText]);

  const setTextStable = useCallback((value: string) => {
    setText(value);
  }, []);

  return { text, setText: setTextStable, isLoading, error };
}
